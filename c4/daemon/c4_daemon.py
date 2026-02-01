"""C4 Daemon - Main orchestrator for C4 project management."""

import logging
from datetime import datetime
from pathlib import Path
from typing import Any

from ..constants import MAX_REPAIR_DEPTH, REPAIR_PREFIX, REPAIR_PREFIX_LEN
from ..discovery import (
    DesignStore,
    Domain,
    EARSPattern,
    EARSRequirement,
    FeatureSpec,
    SpecStore,
    VerificationRequirement,
)
from ..models import (
    C4Config,
    C4State,
    CheckpointConfig,
    CheckpointQueueItem,
    CheckpointResponse,
    EventType,
    ProjectStatus,
    RepairQueueItem,
    SubmitResponse,
    Task,
    TaskAssignment,
    TaskStatus,
    TaskType,
    ValidationResult,
)
from ..notification import NotificationManager
from ..state_machine import StateMachine, StateTransitionError
from ..store import (
    LockStore,
    SQLiteStateStore,
    SQLiteTaskStore,
    StateStore,
    create_lock_store,
    create_state_store,
)
from ..supervisor._legacy.agent_router import AgentRouter
from ..supervisor.agent_graph import (
    AgentGraph,
    AgentGraphLoader,
    GraphRouter,
    RuleEngine,
    SkillMatcher,
    TaskContext,
)
from ..validation import ValidationRunner
from .git_ops import GitOperations
from .pr_manager import PRManager
from .workers import WorkerManager

logger = logging.getLogger(__name__)


def _use_graph_router() -> bool:
    """Check if GraphRouter should be used (feature flag)."""
    import os
    flag = os.environ.get("C4_USE_GRAPH_ROUTER", "true").lower()
    return flag in ("true", "1", "yes", "on")


def _get_workflow_guide(status: str) -> dict[str, str]:
    """Get workflow guide for current project status."""
    guides: dict[str, dict[str, str]] = {
        "INIT": {
            "phase": "init",
            "next": "discovery",
            "hint": (
                "Start planning: scan docs/*.md for requirements, "
                "detect project domain, collect EARS requirements, "
                "call c4_save_spec() for each feature, "
                "then c4_discovery_complete() when done"
            ),
        },
        "DISCOVERY": {
            "phase": "discovery",
            "next": "design",
            "hint": (
                "Continue collecting requirements using EARS patterns. "
                "Call c4_save_spec() for each feature, "
                "then c4_discovery_complete() to proceed to design"
            ),
        },
        "DESIGN": {
            "phase": "design",
            "next": "plan",
            "hint": (
                "Define architecture options for each feature. "
                "Call c4_save_design() with components and decisions, "
                "then c4_design_complete() to proceed to planning"
            ),
        },
        "PLAN": {
            "phase": "plan",
            "next": "execute",
            "hint": (
                "Tasks are ready. Call c4_start() to begin execution, "
                "then use c4_get_task(worker_id) in a loop to process tasks"
            ),
        },
        "EXECUTE": {
            "phase": "execute",
            "next": "worker_loop",
            "hint": (
                "Worker loop: call c4_get_task(worker_id) to get a task, "
                "implement it, run validations with c4_run_validation(), "
                "then c4_submit(task_id, commit_sha, validation_results)"
            ),
        },
        "CHECKPOINT": {
            "phase": "checkpoint",
            "next": "review",
            "hint": (
                "Supervisor review in progress. "
                "Wait for c4_ensure_supervisor() to complete the review, "
                "or call c4_checkpoint() to manually process"
            ),
        },
        "HALTED": {
            "phase": "halted",
            "next": "resume",
            "hint": (
                "Execution is paused. Call c4_start() to resume, "
                "or review repair_queue for blocked tasks"
            ),
        },
        "COMPLETE": {
            "phase": "complete",
            "next": "done",
            "hint": "Project is complete. All tasks have been processed.",
        },
    }
    return guides.get(status, {"phase": "unknown", "next": "unknown", "hint": "Unknown status"})



class C4Daemon:
    """C4 Daemon - manages project state and task orchestration"""

    def __init__(
        self,
        project_root: Path | None = None,
        state_store: StateStore | None = None,
    ):
        import os
        # Priority: explicit param > env var > cwd
        if project_root:
            self.root = project_root
        elif os.environ.get("C4_PROJECT_ROOT"):
            self.root = Path(os.environ["C4_PROJECT_ROOT"])
        else:
            self.root = Path.cwd()
        self.c4_dir = self.root / ".c4"
        self.state_machine: StateMachine | None = None
        self._config: C4Config | None = None
        self._tasks: dict[str, Task] = {}
        self._validation_runner: ValidationRunner | None = None
        self._worker_manager: WorkerManager | None = None
        self._state_store = state_store
        self._lock_store: "LockStore | None" = None
        self._task_store: SQLiteTaskStore | None = None
        self._spec_store: SpecStore | None = None
        self._agent_router: AgentRouter | None = None
        # GraphRouter components (used when C4_USE_GRAPH_ROUTER=true)
        self._graph_router: GraphRouter | None = None
        self._agent_graph: AgentGraph | None = None
        self._rule_engine: RuleEngine | None = None
        # LSP server components
        self._lsp_server: Any = None
        self._lsp_thread: Any = None
        self._lsp_port: int | None = None

    # =========================================================================
    # Initialization
    # =========================================================================

    def _get_default_store(self) -> StateStore:
        """Get the default state store.

        Uses factory to create store based on:
        1. Explicitly provided state_store
        2. config.yaml store settings
        3. C4_STORE_BACKEND environment variable
        4. Default: SQLite
        """
        if self._state_store is not None:
            return self._state_store

        # Get store config from config.yaml if available
        store_config = None
        if hasattr(self, "_config") and self._config is not None:
            store_config = self._config.store

        return create_state_store(self.c4_dir, store_config)

    def is_initialized(self) -> bool:
        """Check if C4 is initialized in this project"""
        # Check both SQLite (new) and JSON (legacy) for backward compatibility
        return (self.c4_dir / "c4.db").exists() or (self.c4_dir / "state.json").exists()

    def _ensure_git_initialized(self) -> bool:
        """
        Ensure git is initialized in the project directory.

        Returns:
            True if git was initialized, False if already existed
        """
        import subprocess

        git_dir = self.root / ".git"
        if git_dir.exists():
            return False

        # Initialize git
        logger.info(f"Initializing git in {self.root}")
        subprocess.run(
            ["git", "init"],
            cwd=self.root,
            capture_output=True,
            check=True,
        )

        # Create default .gitignore if not exists
        gitignore = self.root / ".gitignore"
        if not gitignore.exists():
            gitignore.write_text(
                """# C4
.c4/

# Python
__pycache__/
*.py[cod]
.venv/
.env

# IDE
.idea/
.vscode/
*.swp

# OS
.DS_Store
Thumbs.db
"""
            )
            logger.info("Created default .gitignore")

        return True

    def _ensure_gitignore_has_c4(self) -> bool:
        """Ensure .gitignore contains .c4/ entry.

        Returns:
            True if .gitignore was modified, False if already contained .c4/
        """
        gitignore = self.root / ".gitignore"
        c4_entry = ".c4/"

        if not gitignore.exists():
            # Create new .gitignore with .c4/
            gitignore.write_text(f"# C4 local state (auto-generated)\n{c4_entry}\n")
            logger.info("Created .gitignore with .c4/ entry")
            return True

        # Read existing content
        content = gitignore.read_text()

        # Check if .c4/ is already present (as line or with variations)
        lines = content.splitlines()
        for line in lines:
            stripped = line.strip()
            # Match .c4, .c4/, /.c4, /.c4/ (with or without comments)
            if stripped in (".c4", ".c4/", "/.c4", "/.c4/"):
                return False  # Already present

        # Append .c4/ entry
        if not content.endswith("\n"):
            content += "\n"
        content += f"\n# C4 local state\n{c4_entry}\n"
        gitignore.write_text(content)
        logger.info("Added .c4/ to existing .gitignore")
        return True

    def initialize(
        self,
        project_id: str | None = None,
        with_default_checkpoints: bool = True,
    ) -> C4State:
        """
        Initialize C4 in the project directory.

        Args:
            project_id: Project identifier (defaults to directory name)
            with_default_checkpoints: If True, add default checkpoints (CP-REVIEW, CP-FINAL)
        """
        if project_id is None:
            project_id = self.root.name

        # Initialize git if not exists
        self._ensure_git_initialized()

        # Create directories
        self.c4_dir.mkdir(parents=True, exist_ok=True)
        (self.c4_dir / "locks").mkdir(exist_ok=True)
        (self.c4_dir / "events").mkdir(exist_ok=True)
        (self.c4_dir / "bundles").mkdir(exist_ok=True)
        (self.c4_dir / "workers").mkdir(exist_ok=True)
        (self.c4_dir / "runs").mkdir(exist_ok=True)
        (self.c4_dir / "runs" / "tests").mkdir(exist_ok=True)
        (self.c4_dir / "runs" / "logs").mkdir(exist_ok=True)

        # Create docs directory
        (self.root / "docs").mkdir(exist_ok=True)

        # Initialize state machine with SQLite store (default)
        self.state_machine = StateMachine(self.c4_dir, store=self._get_default_store())
        state = self.state_machine.initialize_state(project_id)

        # Create config with optional default checkpoints
        checkpoints = []
        if with_default_checkpoints:
            from c4.models.checkpoint import DEFAULT_CHECKPOINTS

            checkpoints = list(DEFAULT_CHECKPOINTS)  # Copy to avoid mutation

        self._config = C4Config(project_id=project_id, checkpoints=checkpoints)
        self._save_config()

        # Transition to DISCOVERY
        self.state_machine.transition("c4_init")

        # Add .c4/ to .gitignore
        self._ensure_gitignore_has_c4()

        return state

    def load(self) -> None:
        """Load existing C4 project"""
        if not self.is_initialized():
            raise FileNotFoundError(f"C4 not initialized in {self.root}")

        # Migrate from state.json to SQLite if needed
        self._migrate_to_sqlite_if_needed()

        self.state_machine = StateMachine(self.c4_dir, store=self._get_default_store())
        self.state_machine.load_state()
        self._load_config()
        self._load_tasks()

    def _migrate_to_sqlite_if_needed(self) -> None:
        """Migrate from state.json to SQLite if needed"""
        state_json = self.c4_dir / "state.json"
        db_path = self.c4_dir / "c4.db"

        # Only migrate if state.json exists and c4.db doesn't
        if state_json.exists() and not db_path.exists():
            import json

            # 1. Read JSON state
            data = json.loads(state_json.read_text())
            state = C4State.model_validate(data)

            # 2. Save to SQLite
            store = SQLiteStateStore(db_path)
            store.save(state)

            # 3. Backup original (don't delete)
            backup_path = self.c4_dir / "state.json.bak"
            state_json.rename(backup_path)

    # =========================================================================
    # Config Management
    # =========================================================================

    def _load_config(self) -> None:
        """Load config from config.yaml"""
        config_file = self.c4_dir / "config.yaml"
        if config_file.exists():
            import yaml

            data = yaml.safe_load(config_file.read_text())
            self._config = C4Config.model_validate(data)
        else:
            # Create default config
            self._config = C4Config(project_id=self.root.name)
            self._save_config()

    def _save_config(self) -> None:
        """Save config to config.yaml"""
        if self._config is None:
            return
        import yaml

        config_file = self.c4_dir / "config.yaml"
        config_file.write_text(yaml.dump(self._config.model_dump(), default_flow_style=False))

    def _sync_verification_to_config(self, verification: "VerificationRequirement") -> None:
        """Sync a verification requirement to config.yaml.

        This ensures verifications collected during discovery are available
        for runtime verification during checkpoint review.
        """
        from c4.models.config import VerificationItem

        # Check if already exists (by name)
        existing_names = {item.name for item in self.config.verifications.items}
        if verification.name in existing_names:
            return  # Already synced

        # Add to config
        item = VerificationItem(
            type=verification.type,
            name=verification.name,
            config=verification.config,
            enabled=verification.enabled,
        )
        self.config.verifications.items.append(item)

        # Enable verifications if not already
        if not self.config.verifications.enabled:
            self.config.verifications.enabled = True

        self._save_config()

    def _apply_domain_default_verifications(self, domain: str) -> list[str]:
        """Apply default verifications for a domain.

        Returns list of verification names that were added.
        """
        from c4.models.config import VerificationItem
        from c4.supervisor.verifier import DOMAIN_DEFAULT_VERIFICATIONS

        defaults = DOMAIN_DEFAULT_VERIFICATIONS.get(domain, [])
        if not defaults:
            return []

        added = []
        existing_names = {item.name for item in self.config.verifications.items}

        for default in defaults:
            if default["name"] in existing_names:
                continue

            item = VerificationItem(
                type=default["type"],
                name=default["name"],
                config=default.get("config", {}),
                enabled=True,
            )
            self.config.verifications.items.append(item)
            added.append(default["name"])

        if added:
            if not self.config.verifications.enabled:
                self.config.verifications.enabled = True
            self._save_config()

        return added

    @property
    def config(self) -> C4Config:
        if self._config is None:
            self._load_config()
        return self._config  # type: ignore

    @property
    def agent_router(self) -> AgentRouter:
        """Get legacy agent router (deprecated).

        .. deprecated::
            Use graph_router instead. This property will be removed in a future version.
            Set C4_USE_GRAPH_ROUTER=false to use this legacy router.
        """
        import warnings
        warnings.warn(
            "agent_router is deprecated. Use graph_router with C4_USE_GRAPH_ROUTER=true (default).",
            DeprecationWarning,
            stacklevel=2,
        )
        if self._agent_router is None:
            agent_config = self.config.agents if self._config else None
            self._agent_router = AgentRouter(config=agent_config)
        return self._agent_router

    @property
    def graph_router(self) -> GraphRouter:
        """Get GraphRouter with skill matching and rule engine.

        The GraphRouter provides advanced routing features:
        - Skill-based agent selection
        - Rule engine with overrides and chain extensions
        - Dynamic chain building based on task keywords
        - Graph-based handoff relationships

        This is the default router when C4_USE_GRAPH_ROUTER=true (default).
        """
        if self._graph_router is None:
            # Load graph and rule engine from YAML definitions
            # Use EXAMPLES_DIR for personas/domains/rules, SKILLS_DIR for skills
            from c4.supervisor.agent_graph import EXAMPLES_DIR, SKILLS_DIR
            from c4.supervisor.agent_graph.graph import AgentGraph
            from c4.supervisor.agent_graph.rule_engine import RuleEngine

            # Load personas, domains, rules from EXAMPLES_DIR
            examples_loader = AgentGraphLoader(base_dir=EXAMPLES_DIR)

            # Load skills from SKILLS_DIR (new domain-specific skills)
            skills_loader = AgentGraphLoader(base_dir=SKILLS_DIR.parent)

            # Build graph manually
            self._agent_graph = AgentGraph()
            self._rule_engine = RuleEngine()

            # Add skills from SKILLS_DIR (14 domain-specific skills)
            for skill_def in skills_loader.load_skills(recursive=True):
                self._agent_graph.add_skill(skill_def)

            # Add agents, domains, rules from EXAMPLES_DIR
            for agent_def in examples_loader.load_agents():
                self._agent_graph.add_agent(agent_def)

            for domain_def in examples_loader.load_domains():
                self._agent_graph.add_domain(domain_def)

            for rule_def in examples_loader.load_rules():
                self._rule_engine.add_rules(rule_def)

            # Create skill matcher
            skill_matcher = SkillMatcher(self._agent_graph)

            # Create router with all components
            self._graph_router = GraphRouter(
                graph=self._agent_graph,
                skill_matcher=skill_matcher,
                rule_engine=self._rule_engine,
            )
        return self._graph_router

    @property
    def validation_runner(self) -> ValidationRunner:
        """Get validation runner, creating if necessary"""
        if self._validation_runner is None:
            self._validation_runner = ValidationRunner(self.root, self.config)
        return self._validation_runner

    @property
    def lock_store(self) -> LockStore:
        """Get lock store for atomic lock operations.

        Uses factory to create store based on config or environment.
        """
        if self._lock_store is None:
            store_config = None
            if hasattr(self, "_config") and self._config is not None:
                store_config = self._config.store
            self._lock_store = create_lock_store(self.c4_dir, config=store_config)
        return self._lock_store

    @property
    def worker_manager(self) -> WorkerManager:
        """Get worker manager, creating if necessary"""
        if self._worker_manager is None:
            if self.state_machine is None:
                raise RuntimeError("C4 not loaded")
            self._worker_manager = WorkerManager(self.state_machine, self.config)
        return self._worker_manager

    def _touch_worker(self, worker_id: str | None) -> None:
        """
        Update worker's last_seen timestamp (implicit heartbeat).

        This should be called on every MCP tool call that includes a worker_id
        to keep the worker marked as active. Prevents stale worker recovery
        from reclaiming tasks from active workers.
        """
        if worker_id and self._worker_manager is not None:
            try:
                self._worker_manager.heartbeat(worker_id)
            except Exception as e:
                # Don't fail tool calls if heartbeat fails, but log the error
                logger.warning(f"Worker heartbeat failed for {worker_id}: {e}")

    def _get_effective_domain(self, task: Task) -> str | None:
        """
        Get effective domain for a task (Phase 4: Agent routing).

        Priority:
        1. Task-specific domain (task.domain)
        2. Project config domain (self.config.domain)
        3. None (will use 'unknown' in agent routing)

        Returns:
            Domain string or None
        """
        # Task-specific domain takes priority
        if task.domain:
            return task.domain

        # Fall back to project config domain
        if self._config and self._config.domain:
            return self._config.domain

        return None

    def _get_agent_routing(self, task: Task) -> dict[str, Any]:
        """
        Get agent routing information for a task (Phase 4).

        Uses GraphRouter (default) or legacy AgentRouter based on
        C4_USE_GRAPH_ROUTER environment variable.

        When using GraphRouter:
        - Skill-based matching from task title/description
        - Rule engine overrides based on task_type and keywords
        - Dynamic chain building with required roles

        Returns:
            Dict with agent routing fields for TaskAssignment
        """
        domain = self._get_effective_domain(task)

        if _use_graph_router():
            # Use new GraphRouter with skill matching and rules
            task_context = TaskContext(
                title=task.title,
                description=task.dod,
                task_type=task.task_type if hasattr(task, "task_type") else None,
            )
            agent_config = self.graph_router.get_recommended_agent(domain, task=task_context)
        else:
            # Legacy mode: use static AgentRouter
            import warnings
            warnings.warn(
                "Using legacy AgentRouter. Set C4_USE_GRAPH_ROUTER=true for advanced routing.",
                DeprecationWarning,
                stacklevel=2,
            )
            agent_config = self._get_legacy_agent_config(domain)

        return {
            "recommended_agent": agent_config.primary,
            "agent_chain": agent_config.chain,
            "domain": domain,
            "handoff_instructions": getattr(agent_config, "handoff_instructions", "") or "",
        }

    def _get_legacy_agent_config(self, domain: str | None) -> Any:
        """Get agent config from legacy AgentRouter (no deprecation warning).

        Internal method to avoid double deprecation warnings.
        """
        if self._agent_router is None:
            agent_config = self.config.agents if self._config else None
            self._agent_router = AgentRouter(config=agent_config)
        return self._agent_router.get_recommended_agent(domain)

    def _get_original_worker_for_repair(self, task_id: str) -> str | None:
        """Get the original worker who blocked a repair task.

        Args:
            task_id: The repair task ID (e.g., "REPAIR-T-001")

        Returns:
            The worker_id of the original worker, or None if not found
        """
        if not task_id.startswith(REPAIR_PREFIX):
            return None

        # Extract original task ID by removing REPAIR- prefix
        original_task_id = task_id[REPAIR_PREFIX_LEN:]

        # Look up in repair_queue
        if self.state_machine is None:
            return None

        state = self.state_machine.state
        for item in state.repair_queue:
            if item.task_id == original_task_id:
                return item.worker_id

        return None

    # Note: SupervisorLoop has been removed in favor of unified task queue.
    # CP-XXX tasks handle checkpoint processing, RPR-XXX tasks handle repairs.

    def _sync_merged_tasks(self) -> int:
        """Sync tasks whose branches have been merged to main.

        Checks all pending/in_progress tasks and marks them as done
        if their c4/w-{task_id} branch has been merged to main.

        Returns:
            Number of tasks synced
        """
        if self.state_machine is None:
            return 0

        state = self.state_machine.state
        synced = 0

        # Get merged branches
        git_ops = GitOperations(self.root)
        merged_branches = git_ops.get_merged_task_branches()
        if not merged_branches:
            return 0

        # Check pending tasks
        for task_id in list(state.queue.pending):
            branch = f"c4/w-{task_id}"
            if branch in merged_branches:
                task = self.get_task(task_id)
                if task:
                    logger.info(f"Syncing merged task {task_id} (branch {branch} merged)")
                    state.queue.pending.remove(task_id)
                    state.queue.done.append(task_id)
                    task.status = TaskStatus.DONE
                    task.assigned_to = None
                    self._save_task(task)
                    synced += 1

        # Check in_progress tasks
        for task_id in list(state.queue.in_progress.keys()):
            branch = f"c4/w-{task_id}"
            if branch in merged_branches:
                task = self.get_task(task_id)
                if task:
                    logger.info(f"Syncing merged task {task_id} (branch {branch} merged)")
                    del state.queue.in_progress[task_id]
                    state.queue.done.append(task_id)
                    task.status = TaskStatus.DONE
                    task.assigned_to = None
                    self._save_task(task)
                    synced += 1

        if synced > 0:
            self.state_machine.save_state()
            logger.info(f"Synced {synced} merged tasks")

        return synced

    def c4_ensure_supervisor(self, force_restart: bool = False) -> dict[str, Any]:
        """
        Deprecated: SupervisorLoop has been removed.

        Checkpoint and repair processing is now handled via unified task queue:
        - CP-XXX tasks for checkpoint processing
        - RPR-XXX tasks for repair processing

        This method is kept for backward compatibility but is a no-op.
        """
        return {
            "success": True,
            "action": "deprecated",
            "message": "SupervisorLoop removed. Use unified task queue (CP-XXX, RPR-XXX).",
        }

    def _ensure_supervisor_running(self) -> None:
        """Internal helper to ensure supervisor is running.

        Calls c4_ensure_supervisor() for backward compatibility.
        This is a no-op since SupervisorLoop has been removed.
        """
        self.c4_ensure_supervisor()

    # =========================================================================
    # Task Management
    # =========================================================================

    @property
    def task_store(self) -> SQLiteTaskStore:
        """Get or create the task store"""
        if self._task_store is None:
            self._task_store = SQLiteTaskStore(self.c4_dir / "c4.db")
        return self._task_store

    @property
    def spec_store(self) -> SpecStore:
        """Get or create the spec store"""
        if self._spec_store is None:
            self._spec_store = SpecStore(self.c4_dir)
        return self._spec_store

    def _load_tasks(self) -> None:
        """Load tasks from SQLite (with migration from tasks.json if needed)"""
        project_id = self.state_machine.state.project_id

        # Migrate from tasks.json if SQLite is empty but tasks.json exists
        tasks_file = self.c4_dir / "tasks.json"
        if not self.task_store.exists(project_id) and tasks_file.exists():
            count = self.task_store.migrate_from_json(project_id, tasks_file)
            if count > 0:
                # Backup original tasks.json
                backup_path = self.c4_dir / "tasks.json.bak"
                if not backup_path.exists():
                    tasks_file.rename(backup_path)

        # Load from SQLite
        self._tasks = self.task_store.load_all(project_id)

    def _save_tasks(self) -> None:
        """Save all tasks to SQLite"""
        if self.state_machine is None:
            return
        project_id = self.state_machine.state.project_id
        self.task_store.save_all(project_id, self._tasks)

    def _save_task(self, task: Task) -> None:
        """Save a single task to SQLite (more efficient than save_all)"""
        if self.state_machine is None:
            return
        project_id = self.state_machine.state.project_id
        self.task_store.save(project_id, task)
        # Also update in-memory cache
        self._tasks[task.id] = task

    def get_task(self, task_id: str) -> Task | None:
        """Get a task by ID (from cache, falls back to SQLite)"""
        # Try cache first
        if task_id in self._tasks:
            return self._tasks[task_id]
        # Fall back to SQLite
        if self.state_machine is None:
            return None
        project_id = self.state_machine.state.project_id
        task = self.task_store.get(project_id, task_id)
        if task:
            self._tasks[task_id] = task  # Update cache
        return task

    def add_task(self, task: Task) -> None:
        """Add a task to the registry"""
        self._tasks[task.id] = task
        if task.id not in self.state_machine.state.queue.pending:
            self.state_machine.state.queue.pending.append(task.id)
        self._save_task(task)  # Use single-task save
        self.state_machine.save_state()

    # =========================================================================
    # MCP Tool Implementations
    # =========================================================================

    def _sync_state_consistency(self) -> dict[str, list[str]]:
        """Synchronize c4_state with c4_tasks to fix any inconsistencies.

        c4_state.queue is the source of truth. This method:
        1. Detects tasks in c4_state.in_progress that are done in c4_tasks
        2. Detects tasks in c4_state.in_progress that don't exist
        3. Fixes worker states for completed/missing tasks

        Returns:
            Dict with lists of fixed task IDs by category
        """
        if self.state_machine is None:
            return {"fixed": [], "errors": []}

        fixed_tasks: list[str] = []
        error_tasks: list[str] = []
        project_id = self.state_machine.state.project_id

        try:
            store = self.state_machine.store
            with store.atomic_modify(project_id) as state:
                # Check in_progress tasks
                tasks_to_remove = []
                for task_id, worker_id in list(state.queue.in_progress.items()):
                    task = self.get_task(task_id)

                    # Case 1: Task doesn't exist
                    if task is None:
                        tasks_to_remove.append((task_id, worker_id, "not_found"))
                        continue

                    # Case 2: Task is already done in c4_tasks but still in in_progress
                    if task.status == TaskStatus.DONE:
                        tasks_to_remove.append((task_id, worker_id, "already_done"))
                        continue

                # Apply fixes
                for task_id, worker_id, reason in tasks_to_remove:
                    # Remove from in_progress
                    del state.queue.in_progress[task_id]

                    # Add to done if task exists and was already done
                    if reason == "already_done" and task_id not in state.queue.done:
                        state.queue.done.append(task_id)

                    # Reset worker state
                    if worker_id and worker_id in state.workers:
                        worker = state.workers[worker_id]
                        if worker.task_id == task_id:
                            worker.state = "idle"
                            worker.task_id = None
                            worker.scope = None

                    fixed_tasks.append(task_id)
                    logger.warning(
                        f"State sync: Fixed task {task_id} ({reason}), "
                        f"was assigned to {worker_id}"
                    )

                # Update cached state
                self.state_machine._state = state

        except Exception as e:
            logger.error(f"State sync failed: {e}")
            error_tasks.append(str(e))

        return {"fixed": fixed_tasks, "errors": error_tasks}

    def c4_status(self) -> dict[str, Any]:
        """Get current C4 project status"""
        if self.state_machine is None:
            return {
                "success": False,
                "error": "C4 not initialized",
                "initialized": False,
                "workflow": _get_workflow_guide("INIT"),
            }

        # Plan file sync: Check for changes from plan file
        self._sync_from_plan_file()

        # Auto-sync state consistency on status check
        sync_result = self._sync_state_consistency()
        if sync_result["fixed"]:
            logger.info(f"Auto-fixed {len(sync_result['fixed'])} inconsistent tasks")

        # Re-load state and tasks to get latest (multi-worker sync)
        self.state_machine.load_state()
        self._load_tasks()  # Refresh task cache for consistency
        state = self.state_machine.state
        status_value = state.status.value

        # Get queue stats directly from tasks table (authoritative source)
        # This avoids stale data in c4_state.queue
        queue_stats = self.task_store.get_queue_stats(state.project_id)

        return {
            "initialized": True,
            "project_id": state.project_id,
            "status": status_value,
            "execution_mode": state.execution_mode.value if state.execution_mode else None,
            "checkpoint": {
                "current": state.checkpoint.current,
                "state": state.checkpoint.state,
            },
            "queue": {
                "pending": queue_stats["pending_count"],
                "in_progress": queue_stats["in_progress_count"],
                "done": queue_stats["done_count"],
                "pending_ids": queue_stats["pending_ids"],
                "in_progress_map": queue_stats["in_progress_map"],
                # Economic mode: model info for pending tasks
                "pending_tasks": [
                    {"id": tid, "model": self.get_task(tid).model if self.get_task(tid) else "opus"}
                    for tid in queue_stats["pending_ids"]
                ],
            },
            "workers": {
                wid: {"state": w.state, "task_id": w.task_id}
                for wid, w in state.workers.items()
            },
            "metrics": state.metrics.model_dump(),
            # Async queues for automation
            "checkpoint_queue": [
                {"checkpoint_id": item.checkpoint_id, "triggered_at": item.triggered_at}
                for item in state.checkpoint_queue
            ],
            "repair_queue": [
                {"task_id": item.task_id, "attempts": item.attempts}
                for item in state.repair_queue
            ],
            # Note: SupervisorLoop removed - CP/RPR tasks processed via unified queue
            "supervisor": {
                "mode": "unified_queue",
                "checkpoint_queue_size": len(state.checkpoint_queue),
                "repair_queue_size": len(state.repair_queue),
            },
            # Parallelism analysis for smart worker spawning
            "parallelism": self._calculate_optimal_workers(),
            # Workflow guide for all MCP clients (Claude Code, Codex, Gemini, etc.)
            "workflow": _get_workflow_guide(status_value),
        }

    def c4_worktree_status(self, worker_id: str | None = None) -> dict[str, Any]:
        """Get worktree status for all workers or a specific worker.

        Args:
            worker_id: If provided, return info for specific worker.
                       If None, return list of all worktrees.

        Returns:
            Dict with worktree information:
            - If worker_id is None: list of all worktrees with basic info
            - If worker_id is provided: detailed info for that worktree
        """
        from c4.daemon.worktree_manager import WorktreeManager

        git_ops = GitOperations(self.root)
        if not git_ops.is_git_repo():
            return {
                "success": False,
                "error": "Not a git repository",
            }

        if not self.config.worktree.enabled:
            return {
                "success": False,
                "error": "Worktree feature is disabled in config",
                "hint": "Set worktree.enabled=true in .c4/config.yaml",
            }

        manager = WorktreeManager(self.root)

        if worker_id is None:
            # Return list of all worktrees
            worktrees = manager.list_worktrees()
            worker_ids = manager.get_all_worker_ids()

            return {
                "success": True,
                "worktrees_dir": str(manager.worktrees_dir),
                "worker_count": len(worker_ids),
                "worker_ids": worker_ids,
                "all_worktrees": worktrees,
            }
        else:
            # Return detailed info for specific worker
            info = manager.get_worktree_info(worker_id)

            return {
                "success": True,
                "worker_id": info.worker_id,
                "exists": info.exists,
                "path": str(info.path),
                "branch": info.branch,
                "head": info.head,
                "has_changes": info.has_changes,
            }

    def c4_worktree_cleanup(self, keep_active: bool = True) -> dict[str, Any]:
        """Clean up worktrees, optionally keeping active workers.

        Args:
            keep_active: If True, keep worktrees for workers with in_progress tasks.
                        If False, remove all worktrees.

        Returns:
            Dict with cleanup results including deleted count.
        """
        from c4.daemon.worktree_manager import WorktreeManager

        git_ops = GitOperations(self.root)
        if not git_ops.is_git_repo():
            return {
                "success": False,
                "error": "Not a git repository",
            }

        if not self.config.worktree.enabled:
            return {
                "success": False,
                "error": "Worktree feature is disabled in config",
                "hint": "Set worktree.enabled=true in .c4/config.yaml",
            }

        manager = WorktreeManager(self.root)

        # Get current worker IDs with worktrees
        all_worker_ids = manager.get_all_worker_ids()
        if not all_worker_ids:
            return {
                "success": True,
                "deleted_count": 0,
                "kept_count": 0,
                "message": "No worktrees to clean up",
            }

        # Determine which workers to keep
        keep_workers: list[str] = []
        if keep_active:
            # Keep workers with in_progress tasks
            if self.state_machine:
                self.state_machine.load_state()
                state = self.state_machine.state
                # Get workers that have assigned tasks
                keep_workers = list(state.queue.in_progress.values())

        # Perform cleanup
        result = manager.cleanup(keep_workers=keep_workers if keep_workers else None)

        if not result.success:
            return {
                "success": False,
                "error": result.message,
            }

        # Calculate deleted count
        remaining_workers = manager.get_all_worker_ids()
        deleted_count = len(all_worker_ids) - len(remaining_workers)

        return {
            "success": True,
            "deleted_count": deleted_count,
            "kept_count": len(remaining_workers),
            "kept_workers": remaining_workers,
            "message": result.message,
        }

    def _create_task_branch_from_work(
        self, git_ops: GitOperations, task_branch: str, work_branch: str
    ) -> Any:
        """Create task branch from work branch.

        Ensures task branches are created from the C4 work branch,
        not from arbitrary git HEAD states.

        Args:
            git_ops: GitOperations instance
            task_branch: Name of the task branch (e.g., 'c4/w-T-001-0')
            work_branch: Name of the work branch (e.g., 'c4/my-project')

        Returns:
            GitResult with branch creation status
        """
        from .daemon import GitResult

        # Check if task branch already exists
        check_result = git_ops._run_git("branch", "--list", task_branch)
        if check_result.stdout.strip():
            # Branch exists, checkout to it
            result = git_ops._run_git("checkout", task_branch)
            if result.returncode != 0:
                return GitResult(False, f"Checkout failed: {result.stderr}")
            return GitResult(True, f"Switched to existing task branch {task_branch}")

        # Checkout to work branch first
        checkout_work = git_ops._run_git("checkout", work_branch)
        if checkout_work.returncode != 0:
            # Try to create work branch if it doesn't exist
            create_work = git_ops._run_git("checkout", "-b", work_branch)
            if create_work.returncode != 0:
                return GitResult(
                    False, f"Cannot checkout/create work branch: {checkout_work.stderr}"
                )

        # Create task branch from work branch
        result = git_ops._run_git("checkout", "-b", task_branch)
        if result.returncode != 0:
            return GitResult(False, f"Task branch creation failed: {result.stderr}")

        return GitResult(True, f"Created task branch {task_branch} from {work_branch}")

    def c4_get_task(
        self, worker_id: str, model_filter: str | None = None
    ) -> TaskAssignment | None:
        """Request next task assignment for a worker.

        Args:
            worker_id: Unique worker identifier
            model_filter: Only return tasks with this model (sonnet, opus, haiku).
                If None, returns any available task (default behavior).
        """
        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        # Auto-ensure supervisor loop is running for AI review
        self._ensure_supervisor_running()

        # Implicit heartbeat - keep worker marked as active
        self._touch_worker(worker_id)

        # Re-load state to get latest (prevent race conditions with other workers)
        self.state_machine.load_state()
        # Also refresh task cache from SQLite (fixes stale cache after direct DB edits)
        self._load_tasks()
        state = self.state_machine.state

        # Sync tasks whose branches have been merged (fixes Git-C4 state sync)
        self._sync_merged_tasks()

        # Clean up expired scope locks (prevents stale locks from blocking task assignment)
        self.lock_store.cleanup_expired(state.project_id)

        # Ensure we're in EXECUTE state
        if state.status != ProjectStatus.EXECUTE:
            return None

        # Register worker if not exists
        if not self.worker_manager.is_registered(worker_id):
            self.worker_manager.register(worker_id)

        # Check if worker already has an in_progress task (resume after crash/restart)
        for task_id, assigned_worker in list(state.queue.in_progress.items()):  # list() to allow mutation
            if assigned_worker == worker_id:
                task = self.get_task(task_id)
                if task:
                    # Verify scope lock is still valid for this worker
                    if task.scope:
                        # Check SQLite lock store (authoritative) for lock status
                        lock_owner = self.lock_store.get_lock_owner(
                            state.project_id, task.scope
                        )
                        if lock_owner is None or lock_owner != worker_id:
                            # Lock expired or taken by another worker - cannot resume
                            # Move task back to pending for reassignment by OTHER workers
                            del state.queue.in_progress[task_id]
                            state.queue.pending.insert(0, task_id)  # Add to front of queue
                            task.status = TaskStatus.PENDING
                            task.assigned_to = None
                            self._save_task(task)
                            # Also release SQLite lock if we owned it
                            self.lock_store.release_scope_lock(state.project_id, task.scope)
                            self.state_machine.save_state()
                            # Return None - let another worker pick up the task
                            # (don't try to re-acquire as same worker)
                            return None
                    else:
                        # For scope=None tasks, verify task state consistency
                        if task.assigned_to != worker_id or task.status != TaskStatus.IN_PROGRESS:
                            # Task state inconsistent with queue - reset for reassignment
                            del state.queue.in_progress[task_id]
                            state.queue.pending.insert(0, task_id)
                            task.status = TaskStatus.PENDING
                            task.assigned_to = None
                            self._save_task(task)
                            self.state_machine.save_state()
                            continue

                    # Re-sync worker state
                    self.worker_manager.set_busy(
                        worker_id, task_id, task.scope, task.branch
                    )

                    # Refresh lock TTL for resumed work (with result check)
                    if task.scope:
                        if not self.lock_store.refresh_scope_lock(
                            state.project_id, task.scope, worker_id, self.config.scope_lock_ttl_sec
                        ):
                            # Lock refresh failed - task may have been taken
                            # Move back to pending for safe reassignment
                            del state.queue.in_progress[task_id]
                            state.queue.pending.insert(0, task_id)
                            task.status = TaskStatus.PENDING
                            task.assigned_to = None
                            self._save_task(task)
                            self.state_machine.save_state()
                            continue

                    # Get agent routing info (Phase 4)
                    agent_routing = self._get_agent_routing(task)

                    # Check for existing worktree for resumed task
                    git_ops = GitOperations(self.root)
                    worktree_path: str | None = None
                    if git_ops.is_git_repo() and self.config.worktree.enabled:
                        wt_path = git_ops.get_worktree_path(worker_id)
                        if wt_path.exists():
                            worktree_path = str(wt_path)
                        else:
                            # Try to create worktree for resumed task
                            worktree_result = git_ops.create_worktree(
                                worker_id=worker_id,
                                branch=task.branch or "",
                            )
                            if worktree_result.success:
                                worktree_path = str(wt_path)

                    return TaskAssignment(
                        task_id=task_id,
                        title=task.title,
                        scope=task.scope,
                        dod=task.dod,
                        validations=task.validations,
                        branch=task.branch or "",
                        worktree_path=worktree_path,
                        model=task.model,
                        **agent_routing,
                    )

        # Find available task from pending (sorted by priority, highest first)
        project_id = state.project_id
        ttl = self.config.scope_lock_ttl_sec

        # Get all pending tasks and sort by priority (descending)
        pending_tasks = []
        for task_id in state.queue.pending:
            task = self.get_task(task_id)
            if task:
                pending_tasks.append(task)
        pending_tasks.sort(key=lambda t: t.priority, reverse=True)

        for task in pending_tasks:
            task_id = task.id

            # Economic mode: filter by model if specified
            if model_filter and task.model != model_filter:
                continue

            # Check dependencies first (non-locking check)
            deps_met = all(
                dep_id in state.queue.done for dep_id in task.dependencies
            )
            if not deps_met:
                continue

            # Peer Review: exclude original worker from repair tasks
            # This ensures repairs are reviewed by a different worker
            original_worker = self._get_original_worker_for_repair(task_id)
            if original_worker and original_worker == worker_id:
                continue

            # Try to acquire scope lock ATOMICALLY using SQLite
            # This prevents race conditions between workers
            if task.scope:
                lock_acquired = self.lock_store.acquire_scope_lock(
                    project_id, task.scope, worker_id, ttl
                )
                if not lock_acquired:
                    # Another worker has the lock - skip this task
                    continue

            # Lock acquired (or no scope) - now safe to assign
            # Use atomic_modify to prevent race conditions with c4_submit
            store = self.state_machine.store

            # Determine branch: Review tasks use parent's branch
            if task.type == TaskType.REVIEW and task.parent_id:
                parent_task = self.get_task(task.parent_id)
                if parent_task and parent_task.branch:
                    task_branch = parent_task.branch
                    is_review_using_parent_branch = True
                else:
                    # Fallback: compute parent branch name from parent_id
                    task_branch = f"{self.config.work_branch_prefix}{task.parent_id}"
                    is_review_using_parent_branch = True
            else:
                task_branch = f"{self.config.work_branch_prefix}{task_id}"
                is_review_using_parent_branch = False

            assigned = False

            with store.atomic_modify(project_id) as state:
                # Double-check task is still pending
                if task_id in state.queue.pending:
                    # Assign task (ATOMIC)
                    state.queue.pending.remove(task_id)
                    state.queue.in_progress[task_id] = worker_id

                    # Ensure worker exists in state (fix race condition)
                    if worker_id not in state.workers:
                        from c4.models import WorkerInfo

                        now = datetime.now()
                        state.workers[worker_id] = WorkerInfo(
                            worker_id=worker_id,
                            state="busy",
                            task_id=task_id,
                            scope=task.scope,
                            branch=task_branch,
                            joined_at=now,
                            last_seen=now,
                        )
                    else:
                        # Update existing worker state
                        state.workers[worker_id].state = "busy"
                        state.workers[worker_id].task_id = task_id
                        state.workers[worker_id].scope = task.scope
                        state.workers[worker_id].branch = task_branch
                        state.workers[worker_id].last_seen = datetime.now()

                    assigned = True

                # Update cached state
                self.state_machine._state = state

            if not assigned:
                # Task was assigned by another worker - release our lock
                if task.scope:
                    self.lock_store.release_scope_lock(project_id, task.scope)
                continue

            # Update task in SQLite (outside atomic block but still atomic per-task)
            task.status = TaskStatus.IN_PROGRESS
            task.assigned_to = worker_id
            task.branch = task_branch
            self._save_task(task)

            # Create worktree for isolated multi-worker support
            # Each worker gets their own working directory to avoid conflicts
            git_ops = GitOperations(self.root)
            worktree_path: str | None = None

            if git_ops.is_git_repo() and self.config.worktree.enabled:
                work_branch = self.config.get_work_branch()

                if not is_review_using_parent_branch:
                    # Create worktree with new branch from work_branch
                    worktree_result = git_ops.create_worktree(
                        worker_id=worker_id,
                        branch=task_branch,
                        base_branch=work_branch,
                    )
                    if worktree_result.success:
                        wt_path = git_ops.get_worktree_path(worker_id)
                        worktree_path = str(wt_path)
                        logger.info(
                            f"Created worktree for {worker_id} at {worktree_path}"
                        )
                    else:
                        # Fallback: create branch only (legacy behavior)
                        logger.warning(
                            f"Worktree creation failed for {worker_id}: "
                            f"{worktree_result.message}. Using branch only."
                        )
                        branch_result = self._create_task_branch_from_work(
                            git_ops, task_branch, work_branch
                        )
                        if not branch_result.success:
                            logger.warning(
                                f"Failed to create task branch {task_branch}: "
                                f"{branch_result.message}"
                            )
                else:
                    # Review task: reuse worker's existing worktree if exists
                    wt_path = git_ops.get_worktree_path(worker_id)
                    if wt_path.exists():
                        worktree_path = str(wt_path)
                        logger.info(
                            f"Review task {task_id} using worktree {worktree_path}"
                        )
                    else:
                        logger.info(
                            f"Review task {task_id} using parent branch "
                            f"{task_branch} (no worktree)"
                        )

            # Emit event
            self.state_machine.emit_event(
                EventType.TASK_ASSIGNED,
                "c4d",
                {
                    "task_id": task_id,
                    "worker_id": worker_id,
                    "scope": task.scope,
                    "worktree_path": worktree_path,
                },
            )

            # Get agent routing info (Phase 4)
            agent_routing = self._get_agent_routing(task)

            return TaskAssignment(
                task_id=task_id,
                title=task.title,
                scope=task.scope,
                dod=task.dod,
                validations=task.validations,
                branch=task_branch,
                worktree_path=worktree_path,
                model=task.model,
                **agent_routing,
            )

        return None

    def c4_submit(
        self,
        task_id: str,
        commit_sha: str,
        validation_results: list[dict],
        worker_id: str | None = None,
        review_result: str | None = None,
        review_comments: str | None = None,
    ) -> SubmitResponse:
        """Report task completion with validation results.

        For review tasks (TaskType.REVIEW), additional parameters:
        - review_result: "APPROVE" or "REQUEST_CHANGES"
        - review_comments: Comments for REQUEST_CHANGES (becomes DoD for next version)
        """
        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        # Auto-ensure supervisor loop is running for AI review
        self._ensure_supervisor_running()

        # Implicit heartbeat - keep worker marked as active
        self._touch_worker(worker_id)

        # Parse and validate results first (doesn't need state lock)
        results = [ValidationResult.model_validate(r) for r in validation_results]
        all_passed = all(r.status == "pass" for r in results)
        if not all_passed:
            return SubmitResponse(
                success=False,
                next_action="fix_failures",
                message="Some validations failed",
            )

        # Get task info (from tasks.json, not state)
        task = self.get_task(task_id)
        project_id = self.state_machine.state.project_id

        # Use atomic_modify for thread-safe state update
        # This prevents race conditions when multiple workers submit concurrently
        store = self.state_machine.store

        # Atomic state modification (all stores support atomic_modify)
        error_response: SubmitResponse | None = None
        actual_worker_id: str | None = None

        with store.atomic_modify(project_id) as state:
            # Validate task exists and is in progress
            if task_id not in state.queue.in_progress:
                if task_id in state.queue.done:
                    error_response = SubmitResponse(
                        success=False,
                        next_action="get_next_task",
                        message=f"Task {task_id} already completed by another worker",
                    )
                else:
                    error_response = SubmitResponse(
                        success=False,
                        next_action="fix_failures",
                        message=f"Task {task_id} is not in progress",
                    )
                # Still need to exit context properly - state will be saved as-is
                if error_response:
                    # Update cached state and return early
                    self.state_machine._state = state
                    # We can't return from inside context, so we'll check after

            if error_response is None:
                # Validate worker assignment
                assigned_worker = state.queue.in_progress.get(task_id)
                if worker_id and assigned_worker != worker_id:
                    error_response = SubmitResponse(
                        success=False,
                        next_action="get_next_task",
                        message=f"Task {task_id} is assigned to {assigned_worker}, not {worker_id}",
                    )

            if error_response is None:
                # All validations passed - proceed with state modification
                actual_worker_id = state.queue.in_progress[task_id]

                # Move to done (ATOMIC - this is the critical section)
                del state.queue.in_progress[task_id]
                state.queue.done.append(task_id)

                # Update worker state (ensure worker exists first)
                if actual_worker_id in state.workers:
                    state.workers[actual_worker_id].state = "idle"
                    state.workers[actual_worker_id].task_id = None
                    state.workers[actual_worker_id].scope = None
                    state.workers[actual_worker_id].last_seen = datetime.now()
                else:
                    # Worker not in state (shouldn't happen but handle gracefully)
                    from c4.models import WorkerInfo

                    state.workers[actual_worker_id] = WorkerInfo(
                        worker_id=actual_worker_id,
                        state="idle",
                        joined_at=datetime.now(),
                        last_seen=datetime.now(),
                    )

                # Update metrics
                state.metrics.tasks_completed += 1

                # Update last validation
                state.last_validation = {r.name: r.status for r in results}

            # Update cached state in state_machine
            self.state_machine._state = state

        # Handle error responses after atomic block
        if error_response:
            return error_response

        # Non-atomic operations (safe to do outside the lock)
        # Update task status and commit info in SQLite
        # Although status is derived from c4_state.queue (single source of truth),
        # we sync the task file status for consistency with c4_get_task checks
        self.task_store.update_status(
            project_id,
            task_id,
            status="done",
            commit_sha=commit_sha,
        )

        # Plan file sync: Update task checkbox to done
        self._update_plan_task_status(task_id, "done")

        # Invalidate task cache to ensure get_task returns fresh data
        if task_id in self._tasks:
            del self._tasks[task_id]

        # Release scope lock
        if task and task.scope:
            self.lock_store.release_scope_lock(project_id, task.scope)

        # Emit event
        self.state_machine.emit_event(
            EventType.WORKER_SUBMITTED,
            actual_worker_id,
            {
                "task_id": task_id,
                "commit_sha": commit_sha,
                "validations": [r.model_dump() for r in results],
            },
        )

        # Send notification for task completion
        task_title = task.title if task else task_id
        NotificationManager.notify(
            title="C4 Task Complete",
            message=f"{task_id}: {task_title}",
            urgency="normal",
        )

        # GitHub Auto-Commit: Trigger on task completion
        if (
            self.config.github.enabled
            and self.config.github.auto_commit
            and task
            and task.type == TaskType.IMPLEMENTATION
        ):
            self._trigger_auto_commit(task_id, task.title, actual_worker_id)

        # Review-as-Task: Generate review task for implementation tasks
        if self.config.review_as_task and task and task.type == TaskType.IMPLEMENTATION:
            self._generate_review_task(task, actual_worker_id)

        # Review-as-Task: Handle review task completion
        if self.config.review_as_task and task and task.type == TaskType.REVIEW:
            review_response = self._handle_review_completion(
                task, review_result, review_comments, actual_worker_id
            )
            if review_response:
                return review_response

        # Checkpoint-as-Task: Handle checkpoint task completion
        if self.config.checkpoint_as_task and task and task.type == TaskType.CHECKPOINT:
            cp_response = self._handle_checkpoint_completion(
                task, review_result, review_comments, actual_worker_id
            )
            if cp_response:
                return cp_response

        # Auto-cleanup worktree if enabled
        if self.config.worktree.enabled and self.config.worktree.auto_cleanup:
            if actual_worker_id:
                git_ops = GitOperations(self.root)
                if git_ops.is_git_repo():
                    cleanup_result = git_ops.remove_worktree(actual_worker_id)
                    if cleanup_result.success:
                        logger.info(
                            f"Auto-cleaned worktree for {actual_worker_id} after task {task_id}"
                        )
                    else:
                        logger.warning(
                            f"Failed to cleanup worktree for {actual_worker_id}: "
                            f"{cleanup_result.message}"
                        )

        # Get fresh state reference for final checks
        state = self.state_machine.state

        # Check if checkpoint reached
        cp_id = self.state_machine.check_gate_conditions(self.config)
        if cp_id:
            self._add_to_checkpoint_queue(cp_id, results)
            self.state_machine.enter_checkpoint(cp_id)
            return SubmitResponse(
                success=True,
                next_action="await_checkpoint",
                message=f"Checkpoint {cp_id} queued for AI review (automatic)",
            )

        # Check if all done
        if not state.queue.pending and not state.queue.in_progress:
            return SubmitResponse(
                success=True,
                next_action="complete",
                message="All tasks completed",
            )

        return SubmitResponse(
            success=True,
            next_action="get_next_task",
            message="Task completed successfully",
        )

    def _c4_submit_legacy(
        self,
        task_id: str,
        commit_sha: str,
        results: list[ValidationResult],
        worker_id: str | None,
        task: Task | None,
    ) -> SubmitResponse:
        """Legacy submit for non-SQLite stores (backward compatibility)"""
        self.state_machine.load_state()
        state = self.state_machine.state

        if task_id not in state.queue.in_progress:
            if task_id in state.queue.done:
                return SubmitResponse(
                    success=False,
                    next_action="get_next_task",
                    message=f"Task {task_id} already completed",
                )
            return SubmitResponse(
                success=False,
                next_action="fix_failures",
                message=f"Task {task_id} is not in progress",
            )

        assigned_worker = state.queue.in_progress.get(task_id)
        if worker_id and assigned_worker != worker_id:
            return SubmitResponse(
                success=False,
                next_action="get_next_task",
                message=f"Task {task_id} is assigned to {assigned_worker}",
            )

        actual_worker_id = state.queue.in_progress[task_id]
        del state.queue.in_progress[task_id]
        state.queue.done.append(task_id)

        if task:
            task.status = TaskStatus.DONE
            task.commit_sha = commit_sha
            self._save_task(task)

        if task and task.scope:
            self.lock_store.release_scope_lock(state.project_id, task.scope)

        self.worker_manager.set_idle(actual_worker_id)
        state.metrics.tasks_completed += 1
        state.last_validation = {r.name: r.status for r in results}

        self.state_machine.emit_event(
            EventType.WORKER_SUBMITTED,
            actual_worker_id,
            {
                "task_id": task_id,
                "commit_sha": commit_sha,
                "validations": [r.model_dump() for r in results],
            },
        )
        self.state_machine.save_state()

        # GitHub Auto-Commit: Trigger on task completion (legacy)
        if (
            self.config.github.enabled
            and self.config.github.auto_commit
            and task
            and task.type == TaskType.IMPLEMENTATION
        ):
            self._trigger_auto_commit(task_id, task.title, actual_worker_id)

        cp_id = self.state_machine.check_gate_conditions(self.config)
        if cp_id:
            self._add_to_checkpoint_queue(cp_id, results)
            self.state_machine.enter_checkpoint(cp_id)
            return SubmitResponse(
                success=True,
                next_action="await_checkpoint",
                message=f"Checkpoint {cp_id} queued for AI review (automatic)",
            )

        if not state.queue.pending and not state.queue.in_progress:
            return SubmitResponse(
                success=True,
                next_action="complete",
                message="All tasks completed",
            )

        return SubmitResponse(
            success=True,
            next_action="get_next_task",
            message="Task completed successfully",
        )

    def _add_to_checkpoint_queue(
        self, checkpoint_id: str, validation_results: list[ValidationResult]
    ) -> None:
        """Add checkpoint to queue for async supervisor processing"""
        if self.state_machine is None:
            return

        state = self.state_machine.state

        # Check if already in queue (avoid duplicates)
        if any(item.checkpoint_id == checkpoint_id for item in state.checkpoint_queue):
            return

        item = CheckpointQueueItem(
            checkpoint_id=checkpoint_id,
            triggered_at=datetime.now().isoformat(),
            tasks_completed=list(state.queue.done),
            validation_results=validation_results,
        )
        state.checkpoint_queue.append(item)
        self.state_machine.save_state()

    def c4_add_todo(
        self,
        task_id: str,
        title: str,
        scope: str | None,
        dod: str,
        dependencies: list[str] | None = None,
        domain: str | None = None,
        priority: int = 0,
        model: str = "opus",
    ) -> dict[str, Any]:
        """Add a new task with optional dependencies.

        Supports versioned task IDs for Review-as-Task workflow:
        - T-001 -> T-001-0 (auto-append version 0)
        - T-001-0 -> T-001-0 (keep as-is)
        - R-001-0 -> R-001-0 (review tasks)

        Args:
            task_id: Unique task identifier (e.g., "T-001" or "T-001-0")
            title: Task title
            scope: File/directory scope for lock (e.g., "src/auth/")
            dod: Definition of Done
            dependencies: List of task IDs that must complete first
            domain: Domain for agent routing (e.g., "web-frontend")
            priority: Higher priority tasks are assigned first (default: 0)
            model: Claude model for this task (sonnet, opus, haiku). Default: opus
        """
        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        # Parse and normalize task ID for Review-as-Task
        normalized_id, base_id, version, task_type = self._parse_task_id(task_id)

        # Normalize dependency IDs as well (fixes T-001 vs T-001-0 mismatch bug)
        normalized_deps: list[str] = []
        if dependencies:
            for dep_id in dependencies:
                norm_dep_id, _, _, _ = self._parse_task_id(dep_id)
                normalized_deps.append(norm_dep_id)

        task = Task(
            id=normalized_id,
            title=title,
            scope=scope,
            dod=dod,
            dependencies=normalized_deps,
            domain=domain,
            priority=priority,
            model=model,
            # Review-as-Task fields
            type=task_type,
            base_id=base_id,
            version=version,
        )
        self.add_task(task)

        # Plan file sync: Regenerate plan file with new task
        self._sync_to_plan_file()

        # Validate DoD quality (DDD-CLEANCODE requirement)
        warnings = self._validate_dod_quality(dod)

        result: dict[str, Any] = {
            "success": True,
            "task_id": normalized_id,
            "dependencies": task.dependencies,
            "model": task.model,
        }

        if warnings:
            result["warnings"] = warnings
            result["hint"] = (
                "Use Worker Packet format for better task specification. "
                "See docs/PLAN-DDD-CLEANCODE-guide.md"
            )

        return result

    def _validate_dod_quality(self, dod: str) -> list[str]:
        """Validate DoD quality according to DDD-CLEANCODE requirements.

        Returns:
            List of warning messages (empty if DoD is well-formed)
        """
        from c4.utils.dod_parser import parse_dod, validate_dod_requirements

        warnings: list[str] = []

        # Basic checks
        if len(dod.strip()) < 20:
            warnings.append("DoD too short (< 20 chars)")

        if "- [ ]" not in dod and "- [x]" not in dod:
            warnings.append("DoD missing checklist format (use '- [ ] item')")

        # Parse and validate
        items = parse_dod(dod)
        if items:
            errors = validate_dod_requirements(items)
            warnings.extend(errors)
        else:
            warnings.append("DoD could not be parsed into checklist items")

        # Check for test requirements
        dod_lower = dod.lower()
        if "test" not in dod_lower and "테스트" not in dod_lower:
            warnings.append("DoD missing test requirements")

        return warnings

    def _parse_task_id(self, task_id: str) -> tuple[str, str, int, TaskType]:
        """Parse task ID and extract base_id, version, and type.

        Supports various ID patterns:
        - T-001 -> T-001-0, base_id="001", version=0, IMPLEMENTATION
        - T-001-0 -> T-001-0, base_id="001", version=0, IMPLEMENTATION
        - T-001-2 -> T-001-2, base_id="001", version=2, IMPLEMENTATION
        - R-001-0 -> R-001-0, base_id="001", version=0, REVIEW
        - T-SBX-001-0 -> T-SBX-001-0, base_id="SBX-001", version=0, IMPLEMENTATION
        - T-SBX-001-0-0 -> T-SBX-001-0-0, base_id="SBX-001-0", version=0, IMPLEMENTATION

        Returns:
            Tuple of (normalized_id, base_id, version, task_type)
        """
        import re

        # Determine task type from prefix
        if task_id.startswith("R-"):
            task_type = TaskType.REVIEW
            prefix = "R-"
        elif task_id.startswith("CP-"):
            task_type = TaskType.CHECKPOINT
            prefix = "CP-"
        else:
            task_type = TaskType.IMPLEMENTATION
            prefix = "T-"

        # Remove prefix for parsing
        without_prefix = task_id[len(prefix):]

        # Pattern 1: Simple numeric "001-0" or "001-2"
        match = re.match(r"^(\d+)-(\d+)$", without_prefix)
        if match:
            base_id = match.group(1)
            version = int(match.group(2))
            return task_id, base_id, version, task_type

        # Pattern 2: Simple numeric "001" (no version)
        match = re.match(r"^(\d+)$", without_prefix)
        if match:
            base_id = match.group(1)
            version = 0
            normalized_id = f"{prefix}{base_id}-{version}"
            return normalized_id, base_id, version, task_type

        # Pattern 3: Complex ID ending with version "-N" (e.g., "SBX-001-0", "DEP-001-0-0")
        # Check if last segment is a single digit (version)
        match = re.match(r"^(.+)-(\d)$", without_prefix)
        if match:
            base_id = match.group(1)
            version = int(match.group(2))
            return task_id, base_id, version, task_type

        # Pattern 4: Complex ID without version (e.g., "SBX-001")
        # Append -0 for version
        base_id = without_prefix
        version = 0
        normalized_id = f"{prefix}{base_id}-{version}"
        return normalized_id, base_id, version, task_type

    def _trigger_auto_commit(
        self, task_id: str, title: str, worker_id: str | None
    ) -> None:
        """Trigger GitHub auto-commit for completed task.

        Args:
            task_id: Task identifier
            title: Task title for commit message
            worker_id: Worker who completed the task
        """
        try:
            from c4.integrations import GitHubAutomation

            automation = GitHubAutomation(
                config=self.config.github,
                repo_path=self.root,
            )

            body = f"Task completed by worker: {worker_id}" if worker_id else None
            result = automation.auto_commit(
                task_id=task_id,
                title=title,
                body=body,
            )

            if result.success:
                logger.info(
                    f"Auto-commit for {task_id}: {result.commit_sha} "
                    f"({result.files_changed} files)"
                )
            else:
                logger.warning(f"Auto-commit failed for {task_id}: {result.message}")

        except Exception as e:
            # Don't block workflow on auto-commit errors
            logger.error(f"Auto-commit error for {task_id}: {e}")

    def _get_completed_tasks(self) -> list[dict[str, str]]:
        """Get list of completed implementation tasks for PR body.

        Returns:
            List of dicts with 'id' and 'title' for each completed task
        """
        state = self.state_machine.load()
        tasks_completed = []

        for task_id in state.queue.done:
            task = self.get_task(task_id)
            if task and task.type == TaskType.IMPLEMENTATION:
                tasks_completed.append({
                    "id": task_id,
                    "title": task.title,
                })

        return tasks_completed

    def _generate_review_task(self, task: Task, worker_id: str | None) -> None:
        """Generate a review task for a completed implementation task.

        Creates R-{base_id}-{version} with lower priority to encourage
        peer review (or delayed self-review for solo workers).

        Args:
            task: The completed implementation task
            worker_id: The worker who completed the task
        """
        if not task.base_id:
            # Legacy task without base_id, skip review generation
            logger.warning(f"Task {task.id} has no base_id, skipping review generation")
            return

        review_task_id = f"R-{task.base_id}-{task.version}"
        review_priority = max(0, task.priority - self.config.review_priority_offset)

        review_task = Task(
            id=review_task_id,
            title=f"Review: {task.title}",
            scope=task.scope,
            dod=(
                f"Review implementation of {task.id}. "
                "Check code quality, correctness, and alignment with DoD. "
                "Submit with APPROVE (no comments) or REQUEST_CHANGES (with comments)."
            ),
            dependencies=[],  # Review doesn't depend on other tasks
            domain=task.domain,
            priority=review_priority,
            task_type="review",  # Enable code-reviewer agent routing
            # Review-as-Task fields
            type=TaskType.REVIEW,
            base_id=task.base_id,
            version=task.version,
            parent_id=task.id,
            completed_by=worker_id,
        )

        try:
            self.add_task(review_task)
            logger.info(
                f"Generated review task {review_task_id} for {task.id} "
                f"(priority={review_priority}, completed_by={worker_id})"
            )
        except Exception as e:
            logger.error(f"Failed to generate review task for {task.id}: {e}")

    def _handle_review_completion(
        self,
        task: Task,
        review_result: str | None,
        review_comments: str | None,
        worker_id: str | None,
    ) -> SubmitResponse | None:
        """Handle review task completion with APPROVE or REQUEST_CHANGES.

        Args:
            task: The review task being completed
            review_result: "APPROVE" or "REQUEST_CHANGES"
            review_comments: Comments for REQUEST_CHANGES (becomes new DoD)
            worker_id: The worker completing the review

        Returns:
            SubmitResponse if there's an error or special handling needed,
            None to continue with normal completion flow
        """
        if not task.base_id or not task.parent_id:
            logger.warning(f"Review task {task.id} missing base_id or parent_id")
            return None

        # Determine result based on comments if not explicitly provided
        if review_result is None:
            if review_comments:
                review_result = "REQUEST_CHANGES"
            else:
                review_result = "APPROVE"

        if review_result == "APPROVE":
            # Mark parent implementation task as truly done
            # (it's already in done queue, just log the approval)
            logger.info(
                f"Review {task.id} APPROVED by {worker_id}. "
                f"Parent task {task.parent_id} confirmed complete."
            )

            # Update review_decision on the task
            self.task_store.update_review_decision(
                self.state_machine.state.project_id,
                task.id,
                "APPROVE",
            )

            # Check if all reviews in this phase are approved -> create CP task
            if self.config.checkpoint_as_task:
                self._check_and_create_checkpoint_task(task)

            return None  # Continue with normal completion

        elif review_result == "REQUEST_CHANGES":
            if not review_comments:
                return SubmitResponse(
                    success=False,
                    next_action="fix_failures",
                    message="REQUEST_CHANGES requires review_comments",
                )

            # Check max_revision limit
            next_version = task.version + 1
            if next_version > self.config.max_revision:
                # Mark as BLOCKED
                logger.warning(
                    f"Task {task.base_id} exceeded max_revision ({self.config.max_revision}). "
                    "Marking as BLOCKED."
                )
                # Add to repair queue for escalation
                from c4.models import RepairQueueItem

                repair_item = RepairQueueItem(
                    task_id=f"T-{task.base_id}-{task.version}",
                    worker_id=worker_id or "unknown",
                    failure_signature=f"max_revision_exceeded:{self.config.max_revision}",
                    last_error=f"Exceeded maximum revision count ({self.config.max_revision})",
                    attempts=next_version,
                    blocked_at=datetime.now().isoformat(),
                )
                if self.state_machine:
                    state = self.state_machine.state
                    state.repair_queue.append(repair_item)
                    self.state_machine.save_state()

                return SubmitResponse(
                    success=True,
                    next_action="escalate",
                    message=(
                        f"Task {task.base_id} exceeded max_revision limit. "
                        "Escalated to repair queue."
                    ),
                )

            # Create next version implementation task
            new_task_id = f"T-{task.base_id}-{next_version}"

            # Get original task's info for priority and scope
            parent_task = self.get_task(task.parent_id)
            parent_priority = parent_task.priority if parent_task else task.priority
            parent_scope = parent_task.scope if parent_task else task.scope

            new_task = Task(
                id=new_task_id,
                title=parent_task.title if parent_task else f"Fix: {task.base_id}",
                scope=parent_scope,
                dod=review_comments,  # Review comments become new DoD
                dependencies=[],
                domain=task.domain,
                priority=parent_priority,  # Same priority as original
                # Review-as-Task fields
                type=TaskType.IMPLEMENTATION,
                base_id=task.base_id,
                version=next_version,
                parent_id=task.id,  # Points to the review that requested changes
                review_comments=review_comments,
            )

            try:
                self.add_task(new_task)
                logger.info(
                    f"Created revision task {new_task_id} from review {task.id}. "
                    f"Version {next_version}/{self.config.max_revision}"
                )
            except Exception as e:
                logger.error(f"Failed to create revision task {new_task_id}: {e}")
                return SubmitResponse(
                    success=False,
                    next_action="fix_failures",
                    message=f"Failed to create revision task: {e}",
                )

            return None  # Continue with normal completion

        else:
            return SubmitResponse(
                success=False,
                next_action="fix_failures",
                message=(
                    f"Invalid review_result: {review_result}. "
                    "Use 'APPROVE' or 'REQUEST_CHANGES'"
                ),
            )

    def _check_and_create_checkpoint_task(self, completed_review: Task) -> Task | None:
        """Check if all reviews in a checkpoint are approved and create CP task.

        This is called after a review task is approved. It checks if all tasks
        in the checkpoint config have their reviews approved, and if so,
        creates a checkpoint task (CP-XXX).

        Args:
            completed_review: The review task that was just approved

        Returns:
            The created checkpoint task, or None if not all reviews are approved
        """
        if self.state_machine is None:
            return None

        # Find the checkpoint config that includes this task's parent
        parent_task_id = completed_review.parent_id
        if not parent_task_id:
            return None

        # Find which checkpoint this task belongs to
        matching_checkpoint: CheckpointConfig | None = None
        for cp_config in self.config.checkpoints:
            # Skip already passed checkpoints
            if cp_config.id in self.state_machine.state.passed_checkpoints:
                continue

            # If required_tasks is empty, this checkpoint applies to ALL tasks
            if not cp_config.required_tasks:
                matching_checkpoint = cp_config
                break

            # Check if parent task (T-XXX-N) or base ID matches required_tasks
            base_id = completed_review.base_id
            for required in cp_config.required_tasks:
                # Match by full ID (T-001-0) or base ID pattern (T-001)
                if required == parent_task_id or required.rstrip("-0123456789") == f"T-{base_id}".rstrip("-0123456789"):
                    matching_checkpoint = cp_config
                    break
            if matching_checkpoint:
                break

        if not matching_checkpoint:
            # No checkpoint config includes this task
            logger.debug(f"No checkpoint config found for task {parent_task_id}")
            return None

        # Check if CP task already exists
        cp_task_id = f"CP-{matching_checkpoint.id}"
        if self._task_exists(cp_task_id):
            logger.debug(f"Checkpoint task {cp_task_id} already exists")
            return None

        # Check if this checkpoint was already passed
        if matching_checkpoint.id in self.state_machine.state.passed_checkpoints:
            logger.debug(f"Checkpoint {matching_checkpoint.id} already passed")
            return None

        # Check if all required tasks have their latest review approved
        all_approved = True
        required_impl_tasks: list[str] = []
        review_task_ids: list[str] = []
        state = self.state_machine.state

        # If required_tasks is empty, check ALL implementation tasks in done queue
        if not matching_checkpoint.required_tasks:
            # Get all done implementation tasks
            impl_tasks = self._get_all_done_impl_tasks()
            if not impl_tasks:
                logger.debug("No completed implementation tasks found")
                return None

            for impl_task in impl_tasks:
                required_impl_tasks.append(impl_task.id)

                # Find the latest review for this impl task
                latest_review = self._get_latest_review_for_impl(impl_task.base_id)

                if latest_review is None:
                    # No review yet - not all approved
                    all_approved = False
                    break

                # Check if review is done and approved
                if latest_review.id not in state.queue.done:
                    all_approved = False
                    break

                if latest_review.review_decision != "APPROVE":
                    all_approved = False
                    break

                review_task_ids.append(latest_review.id)
        else:
            # Check specific required tasks
            for required_task_pattern in matching_checkpoint.required_tasks:
                # Find all implementation tasks matching this pattern
                impl_tasks = self._find_tasks_by_pattern(required_task_pattern, TaskType.IMPLEMENTATION)

                for impl_task in impl_tasks:
                    required_impl_tasks.append(impl_task.id)

                    # Find the latest review for this impl task
                    latest_review = self._get_latest_review_for_impl(impl_task.base_id)

                    if latest_review is None:
                        all_approved = False
                        break

                    # Check if review is done and approved
                    if latest_review.id not in state.queue.done:
                        all_approved = False
                        break

                    if latest_review.review_decision != "APPROVE":
                        all_approved = False
                        break

                    review_task_ids.append(latest_review.id)

                if not all_approved:
                    break

        if not all_approved:
            logger.debug(
                f"Not all reviews approved for checkpoint {matching_checkpoint.id}"
            )
            return None

        # All reviews approved! Create checkpoint task
        logger.info(
            f"All reviews approved for checkpoint {matching_checkpoint.id}. "
            f"Creating checkpoint task {cp_task_id}"
        )

        # Build DoD from checkpoint config and design verifications
        checkpoint_dod = self._build_checkpoint_dod(matching_checkpoint)

        # Calculate priority (lower than reviews)
        base_priority = min(
            (self.get_task(t).priority for t in required_impl_tasks if self.get_task(t)),
            default=0,
        )
        cp_priority = base_priority - self.config.checkpoint_priority_offset

        cp_task = Task(
            id=cp_task_id,
            title=f"Checkpoint: {matching_checkpoint.description or matching_checkpoint.id}",
            dod=checkpoint_dod,
            dependencies=review_task_ids,  # Depends on all reviews
            priority=cp_priority,
            type=TaskType.CHECKPOINT,
            phase_id=matching_checkpoint.id,
            required_tasks=required_impl_tasks,
        )

        try:
            self.add_task(cp_task)
            logger.info(
                f"Created checkpoint task {cp_task_id} "
                f"(required_tasks={required_impl_tasks}, priority={cp_priority})"
            )
            return cp_task
        except Exception as e:
            logger.error(f"Failed to create checkpoint task {cp_task_id}: {e}")
            return None

    def _find_tasks_by_pattern(
        self, pattern: str, task_type: TaskType | None = None
    ) -> list[Task]:
        """Find tasks matching a pattern.

        Patterns:
        - "T-001-0": Exact match
        - "T-001": Match any version (T-001-0, T-001-1, etc.)

        Args:
            pattern: Task ID or base ID pattern
            task_type: Optional filter by task type

        Returns:
            List of matching tasks
        """
        import re

        tasks = self.get_all_tasks()
        matching = []

        for task in tasks.values():
            if task_type and task.type != task_type:
                continue

            # Exact match
            if task.id == pattern:
                matching.append(task)
                continue

            # Base ID pattern match (e.g., "T-001" matches "T-001-0", "T-001-1")
            if task.base_id and pattern == f"T-{task.base_id}":
                matching.append(task)
                continue

            # Regex pattern for more complex matching
            if re.match(f"^{pattern.replace('-', r'-')}(-\\d+)?$", task.id):
                matching.append(task)

        return matching

    def _get_all_done_impl_tasks(self) -> list[Task]:
        """Get all completed implementation tasks.

        Returns:
            List of implementation tasks that are in the done queue
        """
        if self.state_machine is None:
            return []

        done_ids = set(self.state_machine.state.queue.done)
        tasks = self.get_all_tasks()
        impl_tasks = []

        for task in tasks.values():
            if task.type != TaskType.IMPLEMENTATION:
                continue
            if task.id not in done_ids:
                continue
            impl_tasks.append(task)

        return impl_tasks

    def _calculate_optimal_workers(self) -> dict[str, Any]:
        """Calculate optimal number of workers based on dependency graph.

        Analyzes the pending task queue to determine:
        1. ready_now: Tasks with all dependencies satisfied
        2. max_parallelism: Maximum width of the dependency DAG
        3. model_distribution: Tasks grouped by model type

        Returns:
            Dict with parallelism analysis:
            - recommended: Recommended number of workers
            - ready_now: Number of tasks ready to run
            - max_parallelism: Max theoretical parallelism
            - by_model: Dict[model, count] of ready tasks per model
            - reason: Explanation of the recommendation
        """
        if self.state_machine is None:
            return {
                "recommended": 1,
                "ready_now": 0,
                "max_parallelism": 1,
                "by_model": {},
                "reason": "C4 not initialized",
            }

        state = self.state_machine.state
        tasks = self.get_all_tasks()

        # Get pending task IDs from queue
        pending_ids = set(state.queue.pending)
        done_ids = set(state.queue.done)

        if not pending_ids:
            return {
                "recommended": 1,
                "ready_now": 0,
                "max_parallelism": 0,
                "by_model": {},
                "reason": "No pending tasks",
            }

        # Find tasks ready to run (all dependencies satisfied)
        ready_tasks: list[Task] = []
        blocked_tasks: list[Task] = []

        for task_id in pending_ids:
            task = tasks.get(task_id)
            if task is None:
                continue

            # Check if all dependencies are done
            deps_satisfied = all(dep in done_ids for dep in (task.dependencies or []))

            if deps_satisfied:
                ready_tasks.append(task)
            else:
                blocked_tasks.append(task)

        # Group ready tasks by model
        by_model: dict[str, int] = {}
        for task in ready_tasks:
            model = task.model or "opus"
            by_model[model] = by_model.get(model, 0) + 1

        ready_count = len(ready_tasks)

        # Calculate max parallelism by analyzing dependency DAG
        # Use level-order (BFS) to find max width at any level
        max_parallelism = self._calculate_dag_max_width(tasks, pending_ids, done_ids)

        # Recommended = min(ready_now, max_parallelism, MAX_WORKERS)
        MAX_WORKERS = 7  # Claude Code subagent limit
        recommended = min(ready_count, max_parallelism, MAX_WORKERS)

        # At least 1 if there are pending tasks
        if pending_ids and recommended == 0:
            recommended = 1

        # Build reason explanation
        if ready_count == 0:
            reason = "All pending tasks have unmet dependencies"
        elif ready_count <= 2:
            reason = f"{ready_count} tasks ready, minimal parallelism"
        elif ready_count == recommended:
            reason = f"All {ready_count} ready tasks can run in parallel"
        else:
            reason = f"{ready_count} tasks ready, capped at {recommended} workers"

        return {
            "recommended": recommended,
            "ready_now": ready_count,
            "max_parallelism": max_parallelism,
            "by_model": by_model,
            "pending_total": len(pending_ids),
            "blocked_count": len(blocked_tasks),
            "reason": reason,
        }

    def _calculate_dag_max_width(
        self,
        tasks: dict[str, Task],
        pending_ids: set[str],
        done_ids: set[str],
    ) -> int:
        """Calculate maximum width (parallelism) of the task dependency DAG.

        Uses topological level assignment to find max concurrent tasks.

        Args:
            tasks: All tasks
            pending_ids: Set of pending task IDs
            done_ids: Set of done task IDs

        Returns:
            Maximum number of tasks that can run in parallel
        """
        if not pending_ids:
            return 0

        # Build dependency graph for pending tasks only
        # level[task_id] = earliest level this task can start
        levels: dict[str, int] = {}

        # Tasks with no pending dependencies start at level 0
        def get_level(task_id: str, visited: set[str]) -> int:
            if task_id in levels:
                return levels[task_id]

            if task_id in visited:
                # Cycle detected, treat as level 0
                return 0

            visited.add(task_id)

            task = tasks.get(task_id)
            if task is None:
                levels[task_id] = 0
                return 0

            # Find max level of dependencies (only pending ones matter)
            max_dep_level = -1
            for dep_id in task.dependencies or []:
                if dep_id in done_ids:
                    # Already done, doesn't affect level
                    continue
                if dep_id in pending_ids:
                    dep_level = get_level(dep_id, visited)
                    max_dep_level = max(max_dep_level, dep_level)

            # This task's level is one after its latest dependency
            my_level = max_dep_level + 1
            levels[task_id] = my_level
            return my_level

        # Calculate levels for all pending tasks
        for task_id in pending_ids:
            get_level(task_id, set())

        if not levels:
            return 1

        # Count tasks at each level
        level_counts: dict[int, int] = {}
        for level in levels.values():
            level_counts[level] = level_counts.get(level, 0) + 1

        # Return max width
        return max(level_counts.values()) if level_counts else 1

    def _get_latest_review_for_impl(self, base_id: str) -> Task | None:
        """Get the latest review task for an implementation task base ID.

        Args:
            base_id: Base task ID (e.g., "001")

        Returns:
            The latest version review task (R-001-N with highest N), or None
        """
        tasks = self.get_all_tasks()
        latest_review: Task | None = None
        latest_version = -1

        for task in tasks.values():
            if task.type != TaskType.REVIEW:
                continue
            if task.base_id != base_id:
                continue
            if task.version > latest_version:
                latest_version = task.version
                latest_review = task

        return latest_review

    def _build_checkpoint_dod(self, cp_config: CheckpointConfig) -> str:
        """Build DoD for checkpoint task from config and design verifications.

        Args:
            cp_config: Checkpoint configuration

        Returns:
            Markdown DoD string
        """
        dod = f"""## Checkpoint: {cp_config.description or cp_config.id}

### 1. Build & Test
- [ ] Full build succeeds
- [ ] All unit tests pass
- [ ] Lint errors: 0

### 2. Integration Verification
- [ ] E2E tests pass (if configured)
- [ ] Component interfaces work correctly

### 3. Required Validations
"""
        for validation in cp_config.required_validations:
            dod += f"- [ ] {validation}: pass\n"

        # Add design verifications if available
        if self.config.verifications.enabled and self.config.verifications.items:
            dod += "\n### 4. Runtime Verifications\n"
            for item in self.config.verifications.items:
                if item.enabled:
                    dod += f"- [ ] [{item.type}] {item.name}\n"

        dod += """
### Decision
- **APPROVE**: All verifications pass, phase complete
- **REQUEST_CHANGES**: Specific tasks need fixes (list task IDs)
- **REPLAN**: Major issues require replanning
"""
        return dod

    def _task_exists(self, task_id: str) -> bool:
        """Check if a task with given ID exists."""
        return task_id in self._tasks or task_id in self.get_all_tasks()

    def _handle_checkpoint_completion(
        self,
        task: Task,
        review_result: str | None,
        review_comments: str | None,
        worker_id: str | None,
    ) -> SubmitResponse | None:
        """Handle checkpoint task completion with APPROVE, REQUEST_CHANGES, or REPLAN.

        Args:
            task: The checkpoint task being completed
            review_result: "APPROVE", "REQUEST_CHANGES", or "REPLAN"
            review_comments: Comments (required for REQUEST_CHANGES/REPLAN)
            worker_id: The worker completing the checkpoint

        Returns:
            SubmitResponse if there's special handling needed,
            None to continue with normal completion flow
        """
        if not task.phase_id:
            logger.warning(f"Checkpoint task {task.id} missing phase_id")
            return None

        # Determine result based on comments if not explicitly provided
        if review_result is None:
            if review_comments:
                review_result = "REQUEST_CHANGES"
            else:
                review_result = "APPROVE"

        # Update review_decision on the task
        self.task_store.update_review_decision(
            self.state_machine.state.project_id,
            task.id,
            review_result,
        )

        if review_result == "APPROVE":
            logger.info(
                f"Checkpoint {task.id} APPROVED by {worker_id}. "
                f"Phase {task.phase_id} complete."
            )

            # Mark checkpoint as passed
            if self.state_machine:
                self.state_machine.state.passed_checkpoints.append(task.phase_id)
                self.state_machine.save_state()

            # Perform completion action (merge/pr) if this was the final checkpoint
            # Check if all checkpoints are now passed
            all_passed = all(
                cp.id in self.state_machine.state.passed_checkpoints
                for cp in self.config.checkpoints
            )
            if all_passed:
                self._perform_completion_action()

            return None  # Continue with normal completion

        elif review_result == "REQUEST_CHANGES":
            if not review_comments:
                return SubmitResponse(
                    success=False,
                    next_action="fix_failures",
                    message="REQUEST_CHANGES requires review_comments with task IDs to fix",
                )

            # Parse problem task IDs from comments
            # Expected format: "T-001, T-003: <description of issues>"
            problem_task_ids = self._parse_problem_task_ids(review_comments)

            if not problem_task_ids:
                logger.warning(
                    f"No task IDs found in checkpoint comments: {review_comments}"
                )
                # Create a generic fix task
                problem_task_ids = task.required_tasks[:1] if task.required_tasks else []

            created_tasks = []
            for problem_task_id in problem_task_ids:
                original_task = self.get_task(problem_task_id)
                if not original_task:
                    logger.warning(f"Problem task {problem_task_id} not found")
                    continue

                # Create next version of the problem task
                next_version = original_task.version + 1
                if next_version > self.config.max_revision:
                    logger.warning(
                        f"Task {original_task.base_id} exceeded max_revision. "
                        "Adding to repair queue."
                    )
                    # Add to repair queue
                    from c4.models import RepairQueueItem

                    repair_item = RepairQueueItem(
                        task_id=problem_task_id,
                        worker_id=worker_id or "unknown",
                        failure_signature=f"checkpoint_request_changes:{task.id}",
                        last_error=review_comments,
                        attempts=next_version,
                        blocked_at=datetime.now().isoformat(),
                    )
                    if self.state_machine:
                        self.state_machine.state.repair_queue.append(repair_item)
                        self.state_machine.save_state()
                    continue

                new_task_id = f"T-{original_task.base_id}-{next_version}"
                new_task = Task(
                    id=new_task_id,
                    title=original_task.title,
                    scope=original_task.scope,
                    dod=review_comments,  # Checkpoint comments become new DoD
                    dependencies=[],
                    domain=original_task.domain,
                    priority=original_task.priority,
                    type=TaskType.IMPLEMENTATION,
                    base_id=original_task.base_id,
                    version=next_version,
                    parent_id=task.id,  # Points to checkpoint task
                    phase_id=task.phase_id,
                    review_comments=review_comments,
                )

                try:
                    self.add_task(new_task)
                    created_tasks.append(new_task_id)
                    logger.info(
                        f"Created fix task {new_task_id} from checkpoint {task.id}"
                    )
                except Exception as e:
                    logger.error(f"Failed to create fix task {new_task_id}: {e}")

            if created_tasks:
                return SubmitResponse(
                    success=True,
                    next_action="get_next_task",
                    message=f"Checkpoint requested changes. Created fix tasks: {created_tasks}",
                )
            return None

        elif review_result == "REPLAN":
            logger.warning(
                f"Checkpoint {task.id} requires REPLAN. "
                f"Adding to repair queue for supervisor guidance."
            )

            # Add to repair queue for escalation
            from c4.models import RepairQueueItem

            repair_item = RepairQueueItem(
                task_id=task.id,
                worker_id=worker_id or "unknown",
                failure_signature=f"checkpoint_replan:{task.phase_id}",
                last_error=review_comments or "Checkpoint requires replanning",
                attempts=1,
                blocked_at=datetime.now().isoformat(),
            )
            if self.state_machine:
                self.state_machine.state.repair_queue.append(repair_item)
                self.state_machine.save_state()

            return SubmitResponse(
                success=True,
                next_action="escalate",
                message=f"Checkpoint {task.id} requires replanning. Escalated to repair queue.",
            )

        else:
            return SubmitResponse(
                success=False,
                next_action="fix_failures",
                message=(
                    f"Invalid review_result: {review_result}. "
                    "Use 'APPROVE', 'REQUEST_CHANGES', or 'REPLAN'"
                ),
            )

    def _parse_problem_task_ids(self, comments: str) -> list[str]:
        """Parse task IDs from checkpoint comments.

        Expected formats:
        - "T-001, T-003: description"
        - "Fix T-001-0 and T-003-0"
        - "Issues with T-001"

        Args:
            comments: Review comments

        Returns:
            List of task IDs found
        """
        import re

        # Match T-XXX-N or T-XXX patterns
        pattern = r"T-\d+(?:-\d+)?"
        matches = re.findall(pattern, comments, re.IGNORECASE)
        return list(set(matches))  # Dedupe

    def _perform_completion_action(self) -> dict[str, Any] | None:
        """Perform completion action when plan is finished.

        Based on config.completion_action (or worktree.completion_action in worktree mode):
        - 'merge': Squash-merge work branch into default_branch
        - 'pr': Create pull request (requires GitHub auth or gh CLI)
        - 'manual': Do nothing, user handles

        Returns:
            Result dict with action taken and status, or None if manual
        """
        # Use worktree.completion_action if worktree is enabled, else top-level
        if self.config.worktree.enabled:
            completion_action = self.config.worktree.completion_action
        else:
            completion_action = self.config.completion_action
        work_branch = self.config.get_work_branch()
        default_branch = self.config.default_branch

        if completion_action == "manual":
            logger.info(
                f"Completion action: manual. "
                f"Merge {work_branch} to {default_branch} when ready."
            )
            return None

        git_ops = GitOperations(self.root)
        if not git_ops.is_git_repo():
            logger.warning("Not a git repository, skipping completion action")
            return {"action": completion_action, "status": "skipped", "reason": "not a git repo"}

        if completion_action == "merge":
            # Squash-merge work branch into default branch
            merge_result = git_ops.merge_branch_to_target(
                source_branch=work_branch,
                target_branch=default_branch,
                squash=True,  # Squash all commits into one
            )
            if merge_result.success:
                # Delete work branch after successful merge
                git_ops._run_git("branch", "-D", work_branch)
                return {
                    "action": "merge",
                    "status": "success",
                    "message": f"Merged {work_branch} into {default_branch}",
                }
            else:
                return {
                    "action": "merge",
                    "status": "failed",
                    "message": merge_result.message,
                }

        elif completion_action == "pr":
            # Check worktree mode - only create PR if base_branch != main
            if self.config.worktree.enabled:
                base_branch = self.config.worktree.base_branch
                if base_branch == "main" or base_branch == default_branch:
                    logger.info(
                        f"Worktree base_branch is '{base_branch}', no PR needed"
                    )
                    return {
                        "action": "pr",
                        "status": "skipped",
                        "message": f"base_branch '{base_branch}' is same as target, no PR needed",
                    }
                # In worktree mode, PR from base_branch to default_branch
                source_branch = base_branch
            else:
                # Non-worktree mode: PR from work_branch to default_branch
                source_branch = work_branch

            # Use PRManager (gh CLI based) for PR creation
            pr_manager = PRManager(self.root)

            if not pr_manager.is_gh_available():
                logger.warning("gh CLI not available, skipping PR creation")
                return {
                    "action": "pr",
                    "status": "skipped",
                    "message": "gh CLI not installed. Install from https://cli.github.com/",
                }

            try:
                # Push source branch to remote first
                push_result = git_ops._run_git("push", "-u", "origin", source_branch)
                if push_result.returncode != 0:
                    return {
                        "action": "pr",
                        "status": "failed",
                        "message": f"Failed to push: {push_result.stderr}",
                    }

                # Get completed tasks for PR body
                tasks_completed = self._get_completed_tasks()
                pr_body = pr_manager.get_completed_tasks_summary(
                    tasks_completed, include_dod=True
                )

                # Create PR using PRManager
                pr_result = pr_manager.create_or_update_pr(
                    branch=source_branch,
                    title=f"[C4] {self.config.project_id}",
                    body=pr_body,
                    base_branch=default_branch,
                )

                # Record result in state (best effort)
                if pr_result.success:
                    try:
                        if self.state_machine and self.state_machine.state:
                            state = self.state_machine.state
                            state.completion_result = {
                                "action": "pr",
                                "status": "success",
                                "pr_url": pr_result.pr_url,
                                "pr_number": pr_result.pr_number,
                                "timestamp": datetime.utcnow().isoformat(),
                            }
                            self._get_default_store().save(self.config.project_id, state)
                    except Exception as e:
                        logger.warning(f"Failed to record PR result in state: {e}")

                    return {
                        "action": "pr",
                        "status": "success",
                        "pr_url": pr_result.pr_url,
                        "pr_number": pr_result.pr_number,
                    }
                else:
                    return {
                        "action": "pr",
                        "status": "failed",
                        "message": pr_result.message,
                    }

            except Exception as e:
                logger.error(f"PR creation error: {e}")
                return {
                    "action": "pr",
                    "status": "failed",
                    "message": str(e),
                }

        return None

    def _merge_completed_task_branches(
        self, state: "C4State"
    ) -> list[dict[str, str]]:
        """Merge completed task branches into work branch.

        Called on checkpoint APPROVE to consolidate approved work.

        Args:
            state: Current C4 state

        Returns:
            List of merge results with task_id and status
        """

        results = []
        git_ops = GitOperations(self.root)

        if not git_ops.is_git_repo():
            return results

        work_branch = self.config.get_work_branch()

        # Get completed tasks from done queue
        for task_id in state.queue.done:
            task = self.get_task(task_id)
            if not task or not task.branch:
                continue

            # Skip if already merged (no branch exists)
            check_result = git_ops._run_git("branch", "--list", task.branch)
            if not check_result.stdout.strip():
                continue

            # Merge task branch into work branch
            merge_result = git_ops.merge_branch_to_target(
                source_branch=task.branch,
                target_branch=work_branch,
                squash=False,  # Keep history for now
            )

            if merge_result.success:
                results.append({"task_id": task_id, "status": "merged"})
                # Optionally delete the task branch after merge
                git_ops._run_git("branch", "-d", task.branch)
            else:
                results.append(
                    {"task_id": task_id, "status": f"failed: {merge_result.message}"}
                )
                logger.warning(
                    f"Failed to merge {task.branch}: {merge_result.message}"
                )

        return results

    def c4_checkpoint(
        self,
        checkpoint_id: str,
        decision: str,
        notes: str,
        required_changes: list[str] | None = None,
    ) -> CheckpointResponse:
        """Record supervisor checkpoint decision"""
        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        state = self.state_machine.state

        # Validate we're in CHECKPOINT state
        if state.status != ProjectStatus.CHECKPOINT:
            return CheckpointResponse(
                success=False,
                message=f"Not in CHECKPOINT state (current: {state.status.value})",
            )

        # Emit decision event
        self.state_machine.emit_event(
            EventType.SUPERVISOR_DECISION,
            "supervisor",
            {
                "checkpoint_id": checkpoint_id,
                "decision": decision,
                "notes": notes,
                "required_changes": required_changes,
            },
        )

        # Process decision
        try:
            if decision == "APPROVE":
                # Add to passed checkpoints to prevent re-triggering
                if checkpoint_id not in state.passed_checkpoints:
                    state.passed_checkpoints.append(checkpoint_id)

                # Merge completed task branches into work branch
                # Branch strategy: task branches → work branch on checkpoint APPROVE
                merge_results = self._merge_completed_task_branches(state)
                if merge_results:
                    logger.info(
                        f"Checkpoint {checkpoint_id}: merged {len(merge_results)} branches"
                    )

                # Check if this is the final checkpoint
                is_final = not state.queue.pending
                if is_final:
                    self.state_machine.transition("approve_final")
                    # Perform completion action (merge, pr, or manual)
                    completion_result = self._perform_completion_action()
                    if completion_result:
                        logger.info(f"Plan completed: {completion_result}")
                else:
                    self.state_machine.transition("approve")
                state.metrics.checkpoints_passed += 1

            elif decision == "REQUEST_CHANGES":
                # Mark checkpoint as passed to prevent re-triggering
                # (supervisor has reviewed; RC tasks are the follow-up)
                if checkpoint_id not in state.passed_checkpoints:
                    state.passed_checkpoints.append(checkpoint_id)

                # Add required changes as tasks
                if required_changes:
                    for i, change in enumerate(required_changes):
                        task_id = f"RC-{checkpoint_id}-{i+1:02d}"
                        self.c4_add_todo(
                            task_id=task_id,
                            title=change,
                            scope=None,
                            dod=change,
                        )
                self.state_machine.transition("request_changes")

            elif decision == "REPLAN":
                self.state_machine.transition("replan")

            else:
                return CheckpointResponse(
                    success=False,
                    message=f"Invalid decision: {decision}",
                )

            # Clear checkpoint state
            state.checkpoint.current = None
            state.checkpoint.state = "pending"

            # Send notification for checkpoint decision
            urgency = "normal" if decision == "APPROVE" else "critical"
            NotificationManager.notify(
                title="C4 Checkpoint Decision",
                message=f"{checkpoint_id}: {decision}",
                urgency=urgency,
            )

            return CheckpointResponse(success=True)

        except StateTransitionError as e:
            return CheckpointResponse(success=False, message=str(e))

    def c4_mark_blocked(
        self,
        task_id: str,
        worker_id: str,
        failure_signature: str,
        attempts: int,
        last_error: str = "",
    ) -> dict[str, Any]:
        """
        Mark a task as blocked after max retry attempts.
        Adds the task to repair queue for supervisor guidance.

        Args:
            task_id: ID of the blocked task
            worker_id: ID of the worker that was working on the task
            failure_signature: Error signature from validation failures
            attempts: Number of fix attempts made
            last_error: Last error message

        Returns:
            Dictionary with success status and message
        """
        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        # Implicit heartbeat - keep worker marked as active
        self._touch_worker(worker_id)

        state = self.state_machine.state

        # Prevent infinite REPAIR nesting (max 2 levels: REPAIR-REPAIR-{task})
        # Use prefix-based check to avoid false positives like "MY-REPAIR-FEATURE"
        repair_depth = 0
        temp_id = task_id
        while temp_id.startswith(REPAIR_PREFIX):
            repair_depth += 1
            temp_id = temp_id[REPAIR_PREFIX_LEN:]

        if repair_depth >= MAX_REPAIR_DEPTH:
            return {
                "success": False,
                "error": f"Max repair nesting exceeded ({repair_depth} >= {MAX_REPAIR_DEPTH})",
                "message": f"Task {task_id} has already been repaired {repair_depth} times. Manual intervention required.",
                "task_id": task_id,
            }

        # Validate task is actually in progress before marking blocked
        if task_id not in state.queue.in_progress:
            return {
                "success": False,
                "error": f"Task {task_id} is not in progress",
                "message": "Cannot mark a task as blocked if it's not currently in progress",
            }

        # Verify worker ownership - only the assigned worker can mark task as blocked
        assigned_worker = state.queue.in_progress.get(task_id)
        if assigned_worker != worker_id:
            return {
                "success": False,
                "error": f"Task {task_id} is assigned to {assigned_worker}, not {worker_id}",
                "message": "Cannot mark a task as blocked if you are not the assigned worker",
            }

        # Move task from in_progress (will be picked up after repair)
        del state.queue.in_progress[task_id]

        # Get task and release scope lock
        task = self.get_task(task_id)
        if task and task.scope:
            self.lock_store.release_scope_lock(state.project_id, task.scope)

        # Update worker state
        if self.worker_manager.is_registered(worker_id):
            self.worker_manager.set_idle(worker_id)

        # Add to repair queue
        item = RepairQueueItem(
            task_id=task_id,
            worker_id=worker_id,
            failure_signature=failure_signature,
            attempts=attempts,
            blocked_at=datetime.now().isoformat(),
            last_error=last_error,
        )
        state.repair_queue.append(item)

        # Emit event
        self.state_machine.emit_event(
            EventType.WORKER_SUBMITTED,  # Reuse event type for blocked
            worker_id,
            {
                "task_id": task_id,
                "blocked": True,
                "failure_signature": failure_signature,
                "attempts": attempts,
            },
        )

        self.state_machine.save_state()

        return {
            "success": True,
            "message": f"Task {task_id} marked as blocked and added to repair queue",
            "repair_queue_size": len(state.repair_queue),
        }

    def c4_start(self) -> dict[str, Any]:
        """
        Transition from PLAN/HALTED to EXECUTE state.
        This starts the worker loop execution.

        Also ensures the C4 work branch exists (created from default_branch if needed).
        Branch strategy: main → c4/{project_id} → c4/w-T-XXX
        """
        if self.state_machine is None:
            return {"success": False, "error": "C4 not initialized"}

        state = self.state_machine.state
        current_status = state.status.value if state.status else "unknown"

        # Check if transition is valid
        if not self.state_machine.can_transition("c4_run"):
            return {
                "success": False,
                "error": f"Cannot start from current state: {current_status}",
                "current_status": current_status,
                "hint": "Must be in PLAN or HALTED state to start execution",
            }

        # Ensure C4 work branch exists (Convention over Configuration)
        # Branch strategy: main → c4/{project_id} → task branches
        git_ops = GitOperations(self.root)
        work_branch = self.config.get_work_branch()
        default_branch = self.config.default_branch

        branch_message = None
        if git_ops.is_git_repo():
            branch_result = git_ops.ensure_work_branch(work_branch, default_branch)
            if not branch_result.success:
                return {
                    "success": False,
                    "error": f"Failed to setup work branch: {branch_result.message}",
                    "current_status": current_status,
                    "hint": f"Ensure '{default_branch}' branch exists and is clean",
                }
            branch_message = branch_result.message
        else:
            logger.info("Not a git repository, skipping work branch setup")

        # Perform the transition
        self.state_machine.transition("c4_run")
        new_state = self.state_machine.state

        # Note: SupervisorLoop removed - CP/RPR tasks processed via unified queue

        return {
            "success": True,
            "message": f"Transitioned from {current_status} to EXECUTE",
            "status": new_state.status.value,
            "pending_tasks": len(new_state.queue.pending),
            "work_branch": work_branch,
            "branch_message": branch_message,
        }

    def c4_run_validation(
        self,
        names: list[str] | None = None,
        fail_fast: bool = True,
        timeout: int = 300,
    ) -> dict[str, Any]:
        """
        Run validation commands (lint, test, etc.)

        Args:
            names: Specific validations to run. If None, runs all required.
            fail_fast: Stop on first failure
            timeout: Timeout per validation in seconds

        Returns:
            Dictionary with validation results
        """
        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        try:
            if names:
                runs = self.validation_runner.run_validations(
                    names, fail_fast=fail_fast, timeout=timeout
                )
            else:
                runs = self.validation_runner.run_all_required(
                    timeout_per_validation=timeout
                )

            # Convert to results
            results = []
            all_passed = True
            for run in runs:
                result = run.to_result()
                results.append(result.model_dump())
                if not run.passed:
                    all_passed = False

            # Update state with last validation results
            self.state_machine.state.last_validation = {
                r["name"]: r["status"] for r in results
            }
            self.state_machine.save_state()

            # Emit event
            self.state_machine.emit_event(
                EventType.VALIDATION_RUN,
                "worker",
                {
                    "validations": [r["name"] for r in results],
                    "all_passed": all_passed,
                    "results": results,
                },
            )

            return {
                "success": all_passed,
                "results": results,
                "summary": {
                    "total": len(results),
                    "passed": sum(1 for r in results if r["status"] == "pass"),
                    "failed": sum(1 for r in results if r["status"] == "fail"),
                },
            }

        except ValueError as e:
            return {"success": False, "error": str(e), "results": []}
        except Exception as e:
            return {"success": False, "error": str(e), "results": []}

    # =========================================================================
    # Discovery & Specification Management
    # =========================================================================

    def c4_save_spec(
        self,
        feature: str,
        requirements: list[dict],
        domain: str,
        description: str | None = None,
    ) -> dict[str, Any]:
        """
        Save feature specification to .c4/specs/.

        Args:
            feature: Feature name (e.g., "user-auth")
            requirements: List of EARS requirements [{id, pattern, text}]
            domain: Domain name (e.g., "web-frontend")
            description: Optional feature description

        Returns:
            Dictionary with save result
        """
        try:
            # Parse domain
            domain_enum = Domain(domain)

            # Create feature spec
            spec = FeatureSpec(
                feature=feature,
                domain=domain_enum,
                description=description,
            )

            # Add requirements
            for req_data in requirements:
                req = EARSRequirement(
                    id=req_data["id"],
                    pattern=EARSPattern(req_data.get("pattern", "ubiquitous")),
                    text=req_data["text"],
                    domain=domain_enum,
                )
                spec.requirements.append(req)

            # Save to store
            spec_file = self.spec_store.save(spec)

            return {
                "success": True,
                "feature": feature,
                "domain": domain,
                "requirements_count": len(spec.requirements),
                "file_path": str(spec_file),
            }

        except ValueError as e:
            return {"success": False, "error": f"Invalid domain: {e}"}
        except Exception as e:
            return {"success": False, "error": str(e)}

    def c4_list_specs(self) -> dict[str, Any]:
        """
        List all feature specifications.

        Returns:
            Dictionary with feature list
        """
        try:
            features = self.spec_store.list_features()

            # Get summary for each feature
            specs_summary = []
            for feature in features:
                spec = self.spec_store.load(feature)
                if spec:
                    specs_summary.append({
                        "feature": spec.feature,
                        "domain": spec.domain.value,
                        "requirements_count": len(spec.requirements),
                        "description": spec.description,
                    })

            return {
                "success": True,
                "count": len(features),
                "features": specs_summary,
            }

        except Exception as e:
            return {"success": False, "error": str(e), "features": []}

    def c4_get_spec(self, feature: str) -> dict[str, Any]:
        """
        Get a specific feature specification.

        Args:
            feature: Feature name

        Returns:
            Dictionary with spec details
        """
        try:
            spec = self.spec_store.load(feature)
            if spec is None:
                return {"success": False, "error": f"Feature '{feature}' not found"}

            return {
                "success": True,
                "feature": spec.feature,
                "domain": spec.domain.value,
                "description": spec.description,
                "requirements": [
                    {
                        "id": req.id,
                        "pattern": req.pattern.value,
                        "text": req.text,
                    }
                    for req in spec.requirements
                ],
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    def c4_add_verification(
        self,
        feature: str,
        verification_type: str,
        name: str,
        reason: str,
        priority: int = 2,
        config: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        """
        Add a verification requirement to a feature spec from conversation context.

        Use this when the user requests specific verification or when conversation
        context suggests verification needs (e.g., "성능 검증 필요해", "보안 테스트 해줘").

        Args:
            feature: Feature name (must exist)
            verification_type: One of: http, browser, cli, metrics, visual, dryrun
            name: Human-readable name for the verification
            reason: Why this verification is needed (from conversation context)
            priority: 1=critical, 2=normal, 3=optional (default: 2)
            config: Verification-specific configuration

        Returns:
            Dictionary with success status and verification details

        Example:
            c4_add_verification(
                feature="user-auth",
                verification_type="http",
                name="Login API Response Time",
                reason="User requested performance verification for login",
                priority=1,
                config={"url": "/api/login", "max_response_time": 500}
            )
        """
        # Validate verification type
        valid_types = ["http", "browser", "cli", "metrics", "visual", "dryrun"]
        if verification_type not in valid_types:
            return {
                "success": False,
                "error": f"Invalid verification type: {verification_type}",
                "valid_types": valid_types,
            }

        # Load existing spec
        spec = self.spec_store.load(feature)
        if spec is None:
            return {
                "success": False,
                "error": f"Feature '{feature}' not found",
                "hint": "Create the feature with c4_save_spec first",
            }

        try:
            # Add verification requirement
            verification = spec.add_verification(
                verification_type=verification_type,
                name=name,
                reason=reason,
                priority=priority,
                **(config or {}),
            )

            # Save updated spec
            self.spec_store.save(spec)

            # Also sync to config.yaml for runtime verification
            self._sync_verification_to_config(verification)

            return {
                "success": True,
                "feature": feature,
                "verification": {
                    "type": verification.type,
                    "name": verification.name,
                    "reason": verification.reason,
                    "priority": verification.priority,
                    "config": verification.config,
                },
                "total_verifications": len(spec.verification_requirements),
                "config_synced": True,
                "message": f"Added {verification_type} verification: {name} (synced to config.yaml)",
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    def c4_get_feature_verifications(self, feature: str) -> dict[str, Any]:
        """
        Get all verification requirements for a feature.

        Args:
            feature: Feature name

        Returns:
            Dictionary with verification requirements
        """
        spec = self.spec_store.load(feature)
        if spec is None:
            return {
                "success": False,
                "error": f"Feature '{feature}' not found",
            }

        return {
            "success": True,
            "feature": feature,
            "verifications": [
                {
                    "type": v.type,
                    "name": v.name,
                    "reason": v.reason,
                    "priority": v.priority,
                    "config": v.config,
                    "enabled": v.enabled,
                }
                for v in spec.verification_requirements
            ],
            "config_format": spec.get_verifications_for_config(),
        }

    def c4_discovery_complete(self) -> dict[str, Any]:
        """
        Mark discovery phase as complete, transition to DESIGN.

        Returns:
            Dictionary with transition result
        """
        if self.state_machine is None:
            return {"success": False, "error": "C4 not initialized"}

        state = self.state_machine.state
        current_status = state.status.value

        # Verify we're in DISCOVERY state
        if state.status != ProjectStatus.DISCOVERY:
            return {
                "success": False,
                "error": f"Not in DISCOVERY state (current: {current_status})",
                "hint": "c4_discovery_complete can only be called from DISCOVERY state",
            }

        # Check if any specs have been created
        specs = self.spec_store.list_features()
        if not specs:
            return {
                "success": False,
                "error": "No specifications found",
                "hint": "Create at least one specification with c4_save_spec before completing discovery",
            }

        try:
            # Collect unique domains from all specs and apply default verifications
            domains_found: set[str] = set()
            default_verifications_added: list[str] = []

            for spec_name in specs:
                spec = self.spec_store.load(spec_name)
                if spec and spec.domain:
                    domain_value = spec.domain.value if hasattr(spec.domain, "value") else str(spec.domain)
                    domains_found.add(domain_value)

            # Apply domain defaults for each unique domain
            for domain in domains_found:
                added = self._apply_domain_default_verifications(domain)
                default_verifications_added.extend(added)

            # Also set domain in config if single domain project
            if len(domains_found) == 1:
                self._config.domain = list(domains_found)[0]
                self._save_config()

            # Transition to DESIGN
            self.state_machine.transition("discovery_complete")

            result = {
                "success": True,
                "previous_status": current_status,
                "new_status": self.state_machine.state.status.value,
                "specs_count": len(specs),
                "domains": list(domains_found),
                "message": "Discovery phase complete. Ready for design review.",
            }

            if default_verifications_added:
                result["default_verifications_added"] = default_verifications_added
                result["message"] += f" Added {len(default_verifications_added)} domain default verification(s)."

            return result

        except StateTransitionError as e:
            return {"success": False, "error": str(e)}

    # =========================================================================
    # Design Phase Tools
    # =========================================================================

    @property
    def design_store(self) -> "DesignStore":
        """Get or create design store instance."""
        if not hasattr(self, "_design_store"):
            from c4.discovery.design import DesignStore

            self._design_store = DesignStore(self.c4_dir / "specs")
        return self._design_store

    def c4_save_design(
        self,
        feature: str,
        domain: str,
        selected_option: str | None = None,
        options: list[dict] | None = None,
        components: list[dict] | None = None,
        decisions: list[dict] | None = None,
        mermaid_diagram: str | None = None,
        constraints: list[str] | None = None,
        nfr: dict[str, str] | None = None,
        description: str | None = None,
    ) -> dict[str, Any]:
        """
        Save design specification for a feature.

        Args:
            feature: Feature name (must match an existing spec)
            domain: Domain name (e.g., "web-frontend")
            selected_option: ID of the selected architecture option
            options: List of architecture options [{id, name, description, complexity, pros, cons, recommended}]
            components: List of components [{name, type, description, responsibilities, dependencies}]
            decisions: List of design decisions [{id, question, decision, rationale, alternatives_considered}]
            mermaid_diagram: Mermaid diagram string
            constraints: List of technical constraints
            nfr: Non-functional requirements dict

        Returns:
            Dictionary with save result
        """
        try:
            from c4.discovery.design import (
                ArchitectureOption,
                ComponentDesign,
                DesignSpec,
            )
            from c4.discovery.models import Domain as DomainEnum

            # Parse domain
            domain_enum = DomainEnum(domain)

            # Create design spec
            spec = DesignSpec(
                feature=feature,
                domain=domain_enum,
                description=description,
                selected_option=selected_option,
                mermaid_diagram=mermaid_diagram,
                constraints=constraints or [],
                nfr=nfr or {},
            )

            # Add architecture options
            if options:
                for opt_data in options:
                    opt = ArchitectureOption(
                        id=opt_data.get("id", f"option-{len(spec.architecture_options)+1}"),
                        name=opt_data.get("name", "Unnamed Option"),
                        description=opt_data.get("description", ""),
                        complexity=opt_data.get("complexity", "medium"),
                        pros=opt_data.get("pros", []),
                        cons=opt_data.get("cons", []),
                        recommended=opt_data.get("recommended", False),
                    )
                    spec.add_option(opt)

            # Add components
            if components:
                for comp_data in components:
                    comp = ComponentDesign(
                        name=comp_data.get("name", ""),
                        type=comp_data.get("type", "component"),
                        description=comp_data.get("description", ""),
                        responsibilities=comp_data.get("responsibilities", []),
                        dependencies=comp_data.get("dependencies", []),
                        interfaces=comp_data.get("interfaces", []),
                    )
                    spec.add_component(comp)

            # Add decisions
            if decisions:
                for dec_data in decisions:
                    spec.add_decision(
                        id=dec_data.get("id", f"DEC-{len(spec.decisions)+1}"),
                        question=dec_data.get("question", ""),
                        decision=dec_data.get("decision", ""),
                        rationale=dec_data.get("rationale", ""),
                        alternatives=dec_data.get("alternatives_considered", []),
                    )

            # Save design
            yaml_path, md_path = self.design_store.save(spec)

            return {
                "success": True,
                "feature": feature,
                "domain": domain,
                "yaml_path": str(yaml_path),
                "md_path": str(md_path),
                "options_count": len(spec.architecture_options),
                "components_count": len(spec.components),
                "decisions_count": len(spec.decisions),
            }

        except ValueError as e:
            return {"success": False, "error": f"Invalid domain: {e}"}
        except Exception as e:
            return {"success": False, "error": str(e)}

    def c4_get_design(self, feature: str) -> dict[str, Any]:
        """
        Get design specification for a feature.

        Args:
            feature: Feature name to retrieve

        Returns:
            Dictionary with design details or error
        """
        try:
            spec = self.design_store.load(feature)

            if spec is None:
                return {
                    "success": False,
                    "error": f"Design not found for feature: {feature}",
                    "hint": "Create a design with c4_save_design first",
                }

            return {
                "success": True,
                "feature": spec.feature,
                "domain": spec.domain.value,
                "description": spec.description,
                "selected_option": spec.selected_option,
                "options": [
                    {
                        "id": opt.id,
                        "name": opt.name,
                        "description": opt.description,
                        "complexity": opt.complexity,
                        "pros": opt.pros,
                        "cons": opt.cons,
                        "recommended": opt.recommended,
                    }
                    for opt in spec.architecture_options
                ],
                "components": [
                    {
                        "name": comp.name,
                        "type": comp.type,
                        "description": comp.description,
                        "responsibilities": comp.responsibilities,
                        "dependencies": comp.dependencies,
                    }
                    for comp in spec.components
                ],
                "decisions": [
                    {
                        "id": dec.id,
                        "question": dec.question,
                        "decision": dec.decision,
                        "rationale": dec.rationale,
                    }
                    for dec in spec.decisions
                ],
                "mermaid_diagram": spec.mermaid_diagram,
                "constraints": spec.constraints,
                "nfr": spec.nfr,
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    def c4_list_designs(self) -> dict[str, Any]:
        """
        List all features with design specifications.

        Returns:
            Dictionary with list of features
        """
        try:
            features = self.design_store.list_features_with_design()
            designs = []

            for feature_name in features:
                spec = self.design_store.load(feature_name)
                if spec:
                    designs.append({
                        "feature": spec.feature,
                        "domain": spec.domain.value,
                        "selected_option": spec.selected_option,
                        "has_diagram": spec.mermaid_diagram is not None,
                        "components_count": len(spec.components),
                        "decisions_count": len(spec.decisions),
                    })

            return {
                "success": True,
                "count": len(designs),
                "designs": designs,
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    def c4_design_complete(self) -> dict[str, Any]:
        """
        Mark design phase as complete, transition to PLAN.

        Returns:
            Dictionary with transition result
        """
        if self.state_machine is None:
            return {"success": False, "error": "C4 not initialized"}

        state = self.state_machine.state
        current_status = state.status.value

        # Verify we're in DESIGN state
        if state.status != ProjectStatus.DESIGN:
            return {
                "success": False,
                "error": f"Not in DESIGN state (current: {current_status})",
                "hint": "c4_design_complete can only be called from DESIGN state",
            }

        # Check if any designs have been created
        designs = self.design_store.list_features_with_design()
        if not designs:
            return {
                "success": False,
                "error": "No design specifications found",
                "hint": "Create at least one design with c4_save_design before completing design phase",
            }

        # Check if designs have selected options
        incomplete_designs = []
        for feature_name in designs:
            spec = self.design_store.load(feature_name)
            if spec and spec.architecture_options and not spec.selected_option:
                incomplete_designs.append(feature_name)

        if incomplete_designs:
            return {
                "success": False,
                "error": f"Designs without selected option: {incomplete_designs}",
                "hint": "Select an architecture option for each design before completing",
            }

        try:
            # Transition to PLAN
            self.state_machine.transition("design_approved")

            return {
                "success": True,
                "previous_status": current_status,
                "new_status": self.state_machine.state.status.value,
                "designs_count": len(designs),
                "message": "Design phase complete. Ready for planning.",
            }

        except StateTransitionError as e:
            return {"success": False, "error": str(e)}

    def c4_test_agent_routing(
        self,
        domain: str | None = None,
        task_type: str | None = None,
    ) -> dict[str, Any]:
        """
        Test agent routing configuration (for debugging/verification).

        Args:
            domain: Domain to test. If not provided, shows all domains.
            task_type: Task type to test. Shows override if exists.

        Returns:
            Agent routing information

        Examples:
            c4_test_agent_routing()  # All domains and overrides
            c4_test_agent_routing(domain="web-frontend")  # Specific domain
            c4_test_agent_routing(domain="ml-dl", task_type="debug")  # With override
        """
        if domain:
            config = self.agent_router.get_recommended_agent(domain)
            result: dict[str, Any] = {
                "domain": domain,
                "primary_agent": config.primary,
                "agent_chain": config.chain,
                "handoff_instructions": config.handoff_instructions,
            }

            if task_type:
                override = self.agent_router.get_agent_for_task_type(task_type, domain)
                result["task_type"] = task_type
                result["overridden_agent"] = override
                result["is_override"] = override != config.primary

            return result

        # Return all domains and overrides summary
        return {
            "total_domains": len(self.agent_router.get_all_domains()),
            "domains": self.agent_router.get_all_domains(),
            "domain_configs": {
                d: {"primary": c.primary, "chain_length": len(c.chain)}
                for d, c in self.agent_router.merged_chains.items()
            },
            "total_task_overrides": len(self.agent_router.merged_overrides),
            "task_type_overrides": self.agent_router.merged_overrides,
            "custom_config_loaded": self._config is not None
            and self._config.agents is not None,
        }

    def c4_query_agent_graph(
        self,
        query_type: str = "overview",
        filter_by: str | None = None,
        filter_value: str | None = None,
        output_format: str = "json",
        from_agent: str | None = None,
        to_agent: str | None = None,
    ) -> dict[str, Any]:
        """
        Query the agent graph for agents, skills, domains, paths, and chains.

        This tool provides flexible querying of the agent routing graph,
        replacing and extending c4_test_agent_routing.

        Args:
            query_type: Type of query:
                - "overview": Summary of all nodes and edges
                - "agents": List all agents (optionally filter by skill/domain)
                - "skills": List all skills
                - "domains": List all domains
                - "path": Find path between two agents
                - "chain": Build chain from an agent
            filter_by: Filter type ("skill", "domain", "agent")
            filter_value: Value to filter by
            output_format: "json" or "mermaid"
            from_agent: Source agent for path query
            to_agent: Target agent for path query

        Returns:
            Query results in specified format

        Examples:
            c4_query_agent_graph(query_type="overview")
            c4_query_agent_graph(query_type="agents", filter_by="skill",
                                 filter_value="python-coding")
            c4_query_agent_graph(query_type="path", from_agent="backend-dev",
                                 to_agent="code-reviewer")
            c4_query_agent_graph(query_type="chain", from_agent="architect")
            c4_query_agent_graph(query_type="domains", output_format="mermaid")
        """
        from c4.supervisor.agent_graph.visualizer import (
            highlight_path,
            to_mermaid,
        )

        # Use the daemon's graph router (with loaded skills)
        router = self.graph_router
        graph = self._agent_graph

        result: dict[str, Any] = {"query_type": query_type}

        if query_type == "overview":
            # Summary of the agent graph
            result["agents"] = sorted(router.get_all_domains())
            result["total_domains"] = len(router.get_all_domains())
            result["domains"] = router.get_all_domains()
            result["legacy_fallback"] = router.use_legacy_fallback

            # Include domain configs
            result["domain_configs"] = {}
            for domain in router.get_all_domains():
                config = router.get_recommended_agent(domain)
                result["domain_configs"][domain] = {
                    "primary": config.primary,
                    "chain": config.chain,
                }

        elif query_type == "agents":
            # List agents, optionally filtered
            agents_data: list[dict[str, Any]] = []

            # Get all unique agents from domain chains
            all_agents: set[str] = set()
            for domain in router.get_all_domains():
                config = router.get_recommended_agent(domain)
                all_agents.update(config.chain)

            # Apply filter
            for agent_id in sorted(all_agents):
                if filter_by == "domain" and filter_value:
                    config = router.get_recommended_agent(filter_value)
                    if agent_id not in config.chain:
                        continue

                agents_data.append({"id": agent_id})

            result["agents"] = agents_data
            result["total"] = len(agents_data)

        elif query_type == "skills":
            # List skills from graph (if available)
            skills = graph.skills if graph else []
            result["skills"] = sorted(skills)
            result["total"] = len(skills)

        elif query_type == "domains":
            # List domains with details
            domains_data: list[dict[str, Any]] = []
            for domain in sorted(router.get_all_domains()):
                config = router.get_recommended_agent(domain)
                domains_data.append({
                    "id": domain,
                    "primary": config.primary,
                    "chain_length": len(config.chain),
                    "description": config.description,
                })
            result["domains"] = domains_data
            result["total"] = len(domains_data)

        elif query_type == "path":
            # Find path between two agents
            if not from_agent or not to_agent:
                result["error"] = "path query requires from_agent and to_agent"
            else:
                path = graph.get_path(from_agent, to_agent) if graph else None
                if path:
                    result["path"] = path
                    result["length"] = len(path)
                else:
                    result["path"] = None
                    result["message"] = f"No path found from {from_agent} to {to_agent}"

        elif query_type == "chain":
            # Build chain from an agent
            if not from_agent:
                result["error"] = "chain query requires from_agent"
            else:
                # Try graph first, then fallback to legacy router
                chain = graph.build_chain(from_agent) if graph else []
                if not chain:
                    # Use legacy fallback if graph has no chain
                    chain = router.get_chain_for_domain(filter_value) if filter_value else [from_agent]
                result["chain"] = chain
                result["length"] = len(chain)

        else:
            result["error"] = f"Unknown query_type: {query_type}"
            result["valid_types"] = ["overview", "agents", "skills", "domains",
                                     "path", "chain"]

        # Convert to Mermaid if requested
        if output_format == "mermaid" and "error" not in result:
            try:
                if query_type == "path" and result.get("path"):
                    # Highlight path in Mermaid
                    result["mermaid"] = highlight_path(
                        graph, from_agent, to_agent
                    ) if graph else ""
                else:
                    # Full graph as Mermaid
                    result["mermaid"] = to_mermaid(graph) if graph else ""
            except Exception as e:
                result["mermaid_error"] = str(e)

        return result

    # =========================================================================
    # Symbol Editing MCP Tools
    # =========================================================================

    def _get_symbol_by_name_path(
        self,
        name_path: str,
        file_path: str | None = None,
    ) -> tuple[Any | None, str | None, str | None]:
        """Find a symbol by name path.

        Args:
            name_path: Symbol name or qualified name (e.g., "MyClass" or "MyClass.method")
            file_path: Optional file path to restrict search

        Returns:
            Tuple of (symbol, file_path, error_message)
        """
        from c4.docs.analyzer import CodeAnalyzer

        try:
            analyzer = CodeAnalyzer()

            if file_path:
                abs_file_path = Path(file_path)
                if not abs_file_path.is_absolute():
                    abs_file_path = self.root / file_path
                if not abs_file_path.exists():
                    return None, None, f"File not found: {file_path}"
                analyzer.add_file(abs_file_path)
                search_path = str(abs_file_path)
            else:
                analyzer.add_directory(
                    self.root,
                    recursive=True,
                    exclude_patterns=[
                        "**/node_modules/**",
                        "**/__pycache__/**",
                        "**/.git/**",
                        "**/venv/**",
                        "**/.venv/**",
                        "**/.c4/**",
                        "**/.claude/**",
                    ],
                )
                search_path = None

            # Parse name_path to get symbol name and parent
            parts = name_path.split(".")
            symbol_name = parts[-1]

            # Find the symbol
            symbols = analyzer.find_symbol(
                symbol_name, file_path=search_path, exact_match=True
            )

            if not symbols:
                return None, None, f"Symbol not found: {name_path}"

            # If qualified name, filter by parent
            if len(parts) > 1:
                parent_name = ".".join(parts[:-1])
                matching = [
                    s for s in symbols
                    if s.parent == parent_name or s.qualified_name == name_path
                ]
                if not matching:
                    return None, None, f"Symbol with parent '{parent_name}' not found"
                symbols = matching

            # Return the first match
            symbol = symbols[0]

            # Find which file contains this symbol
            symbol_file = symbol.location.file_path

            return symbol, symbol_file, None

        except Exception as e:
            return None, None, str(e)

    def c4_replace_symbol_body(
        self,
        name_path: str,
        file_path: str | None,
        new_body: str,
    ) -> dict[str, Any]:
        """Replace the body of a symbol (function, class, method).

        Args:
            name_path: Symbol name or qualified name (e.g., "MyClass.method")
            file_path: File containing the symbol (optional for single-file search)
            new_body: New source code for the symbol body

        Returns:
            Dict with success status and details about the edit
        """
        symbol, symbol_file, error = self._get_symbol_by_name_path(name_path, file_path)
        if error:
            return {"success": False, "error": error}

        try:
            # Read the file
            file_path_obj = Path(symbol_file)
            content = file_path_obj.read_text(encoding="utf-8")
            lines = content.splitlines(keepends=True)

            # Get symbol location (1-indexed in Location)
            start_line = symbol.location.start_line - 1  # Convert to 0-indexed
            end_line = symbol.location.end_line - 1

            # Preserve leading indentation from original
            original_first_line = lines[start_line] if start_line < len(lines) else ""
            indent = len(original_first_line) - len(original_first_line.lstrip())
            indent_str = original_first_line[:indent]

            # Ensure new_body lines have proper indentation
            new_lines = new_body.splitlines(keepends=True)
            if new_lines and not new_lines[-1].endswith("\n"):
                new_lines[-1] += "\n"

            # Apply indentation to new body (except first line if it already has it)
            indented_lines = []
            for i, line in enumerate(new_lines):
                if i == 0 or not line.strip():
                    indented_lines.append(line)
                else:
                    indented_lines.append(indent_str + line.lstrip())

            # Replace the lines
            new_content_lines = (
                lines[:start_line] + indented_lines + lines[end_line + 1:]
            )
            new_content = "".join(new_content_lines)

            # Write back
            file_path_obj.write_text(new_content, encoding="utf-8")

            return {
                "success": True,
                "file_path": symbol_file,
                "symbol": name_path,
                "start_line": start_line + 1,
                "end_line": end_line + 1,
                "lines_replaced": end_line - start_line + 1,
                "new_lines": len(indented_lines),
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    def c4_insert_before_symbol(
        self,
        name_path: str,
        file_path: str | None,
        content: str,
    ) -> dict[str, Any]:
        """Insert content before a symbol.

        Args:
            name_path: Symbol name or qualified name
            file_path: File containing the symbol
            content: Content to insert before the symbol

        Returns:
            Dict with success status and details
        """
        symbol, symbol_file, error = self._get_symbol_by_name_path(name_path, file_path)
        if error:
            return {"success": False, "error": error}

        try:
            file_path_obj = Path(symbol_file)
            file_content = file_path_obj.read_text(encoding="utf-8")
            lines = file_content.splitlines(keepends=True)

            # Get insertion point (line before the symbol)
            insert_line = symbol.location.start_line - 1  # 0-indexed

            # Ensure content ends with newline
            if content and not content.endswith("\n"):
                content += "\n"

            # Insert the content
            content_lines = content.splitlines(keepends=True)
            new_lines = lines[:insert_line] + content_lines + lines[insert_line:]
            new_content = "".join(new_lines)

            # Write back
            file_path_obj.write_text(new_content, encoding="utf-8")

            return {
                "success": True,
                "file_path": symbol_file,
                "symbol": name_path,
                "inserted_at_line": insert_line + 1,
                "lines_inserted": len(content_lines),
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    def c4_insert_after_symbol(
        self,
        name_path: str,
        file_path: str | None,
        content: str,
    ) -> dict[str, Any]:
        """Insert content after a symbol.

        Args:
            name_path: Symbol name or qualified name
            file_path: File containing the symbol
            content: Content to insert after the symbol

        Returns:
            Dict with success status and details
        """
        symbol, symbol_file, error = self._get_symbol_by_name_path(name_path, file_path)
        if error:
            return {"success": False, "error": error}

        try:
            file_path_obj = Path(symbol_file)
            file_content = file_path_obj.read_text(encoding="utf-8")
            lines = file_content.splitlines(keepends=True)

            # Get insertion point (line after the symbol ends)
            # end_line is 1-indexed, so this gives us the 0-indexed line after
            insert_line = symbol.location.end_line

            # Ensure content starts with newline for separation and ends with newline
            if content and not content.startswith("\n"):
                content = "\n" + content
            if content and not content.endswith("\n"):
                content += "\n"

            # Insert the content
            content_lines = content.splitlines(keepends=True)
            new_lines = lines[:insert_line] + content_lines + lines[insert_line:]
            new_content = "".join(new_lines)

            # Write back
            file_path_obj.write_text(new_content, encoding="utf-8")

            return {
                "success": True,
                "file_path": symbol_file,
                "symbol": name_path,
                "inserted_at_line": insert_line + 1,
                "lines_inserted": len(content_lines),
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    def c4_rename_symbol(
        self,
        name_path: str,
        file_path: str | None,
        new_name: str,
    ) -> dict[str, Any]:
        """Rename a symbol across the entire codebase.

        This finds all references to the symbol and renames them.

        Args:
            name_path: Current symbol name or qualified name
            file_path: File containing the symbol definition (optional)
            new_name: New name for the symbol

        Returns:
            Dict with success status and list of files modified
        """
        import re

        from c4.docs.analyzer import CodeAnalyzer

        try:
            # First, find the symbol definition
            symbol, symbol_file, error = self._get_symbol_by_name_path(
                name_path, file_path
            )
            if error:
                return {"success": False, "error": error}

            # Get the simple name (last part of qualified name)
            old_name = name_path.split(".")[-1]

            # Validate new name
            if not new_name.isidentifier():
                return {"success": False, "error": f"Invalid identifier: {new_name}"}

            # Create analyzer for finding references
            analyzer = CodeAnalyzer()
            analyzer.add_directory(
                self.root,
                recursive=True,
                exclude_patterns=[
                    "**/node_modules/**",
                    "**/__pycache__/**",
                    "**/.git/**",
                    "**/venv/**",
                    "**/.venv/**",
                    "**/.c4/**",
                    "**/.claude/**",
                ],
            )

            # Find all references
            references = analyzer.find_references(old_name)

            # Group references by file
            refs_by_file: dict[str, list] = {}
            for ref in references:
                fp = ref.location.file_path
                if fp not in refs_by_file:
                    refs_by_file[fp] = []
                refs_by_file[fp].append(ref)

            # Also include the definition file
            if symbol_file not in refs_by_file:
                refs_by_file[symbol_file] = []

            # Perform replacements file by file
            files_modified = []
            total_replacements = 0

            for fp in refs_by_file:
                try:
                    file_path_obj = Path(fp)
                    file_content = file_path_obj.read_text(encoding="utf-8")

                    # Use word boundary replacement to avoid partial matches
                    pattern = r"\b" + re.escape(old_name) + r"\b"
                    new_content, count = re.subn(pattern, new_name, file_content)

                    if count > 0:
                        file_path_obj.write_text(new_content, encoding="utf-8")
                        files_modified.append({
                            "file_path": fp,
                            "replacements": count,
                        })
                        total_replacements += count

                except Exception:
                    # Log but continue with other files
                    pass

            return {
                "success": True,
                "old_name": old_name,
                "new_name": new_name,
                "files_modified": files_modified,
                "total_files": len(files_modified),
                "total_replacements": total_replacements,
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    def c4_find_symbol(
        self,
        name_path_pattern: str,
        relative_path: str = "",
        include_body: bool = False,
        depth: int = 0,
    ) -> dict[str, Any]:
        """Find symbols matching the name path pattern.

        Name path patterns:
        - Simple name: "method_name" - matches any symbol with that name
        - Relative path: "ClassName/method_name" - matches method in class
        - Absolute path: "/ClassName/method_name" - exact match from root

        Args:
            name_path_pattern: Pattern to match (e.g., "MyClass/my_method")
            relative_path: Restrict search to this file or directory
            include_body: Whether to include symbol body in results
            depth: Depth of children to include (0 = symbol only)

        Returns:
            Dict with list of matching symbols
        """
        # Warn if relative_path is not provided (workspace-wide search is slow/unreliable)
        if not relative_path:
            return {
                "success": False,
                "error": (
                    "relative_path is required for reliable symbol search. "
                    "Workspace-wide search is disabled due to timeout issues. "
                    "Please provide a file or directory path to limit the search scope."
                ),
                "hint": "Use relative_path parameter, e.g., relative_path='c4/lsp/provider.py'",
            }

        try:
            from c4.lsp.unified_provider import find_symbol_unified

            symbols = find_symbol_unified(
                name_path_pattern=name_path_pattern,
                relative_path=relative_path,
                include_body=include_body,
                project_path=str(self.root),
                timeout=30,
            )

            return {
                "success": True,
                "pattern": name_path_pattern,
                "relative_path": relative_path,
                "symbols": symbols,
                "count": len(symbols),
            }

        except ImportError:
            return {
                "success": False,
                "error": "LSP providers not available. Install with: uv add multilspy jedi",
            }
        except Exception as e:
            return {"success": False, "error": str(e)}

    def c4_get_symbols_overview(
        self,
        relative_path: str,
        depth: int = 0,
    ) -> dict[str, Any]:
        """Get an overview of symbols in a file.

        This should be the first tool to call when you want to understand a new file,
        unless you already know what you are looking for.

        Args:
            relative_path: Path to the file (relative to project root)
            depth: Depth of children to include (0 = top-level only)

        Returns:
            Dictionary with symbols grouped by kind
        """
        try:
            from c4.lsp.unified_provider import get_symbols_overview_unified

            result = get_symbols_overview_unified(
                relative_path=relative_path,
                depth=depth,
                project_path=str(self.root),
                timeout=30,
            )

            if "error" in result:
                return {"success": False, "error": result["error"]}

            return {
                "success": True,
                **result,
            }

        except ImportError:
            return {
                "success": False,
                "error": "LSP providers not available. Install with: uv add multilspy jedi",
            }
        except Exception as e:
            return {"success": False, "error": str(e)}

    # =========================================================================
    # File Operation Tools
    # =========================================================================

    def _get_file_tools(self):
        """Get or create FileTools instance."""
        if not hasattr(self, "_file_tools"):
            from c4.lsp.file_tools import FileTools
            self._file_tools = FileTools(self.root)
        return self._file_tools

    def c4_read_file(
        self,
        relative_path: str,
        start_line: int = 0,
        end_line: int | None = None,
    ) -> dict[str, Any]:
        """Read a file or portion of it.

        Args:
            relative_path: Path relative to project root
            start_line: 0-based index of first line to read
            end_line: 0-based index of last line (inclusive), None for end

        Returns:
            Dictionary with content, total_lines, start_line, end_line
        """
        return self._get_file_tools().read_file(
            relative_path=relative_path,
            start_line=start_line,
            end_line=end_line,
        )

    def c4_create_text_file(
        self,
        relative_path: str,
        content: str,
    ) -> dict[str, Any]:
        """Create or overwrite a text file.

        Args:
            relative_path: Path relative to project root
            content: Content to write

        Returns:
            Dictionary with success status and message
        """
        return self._get_file_tools().create_text_file(
            relative_path=relative_path,
            content=content,
        )

    def c4_list_dir(
        self,
        relative_path: str = ".",
        recursive: bool = False,
    ) -> dict[str, Any]:
        """List files and directories.

        Args:
            relative_path: Path relative to project root
            recursive: Whether to scan subdirectories

        Returns:
            Dictionary with directories and files lists
        """
        return self._get_file_tools().list_dir(
            relative_path=relative_path,
            recursive=recursive,
        )

    def c4_find_file(
        self,
        file_mask: str,
        relative_path: str = ".",
    ) -> dict[str, Any]:
        """Find files matching a pattern.

        Args:
            file_mask: Filename or glob pattern
            relative_path: Directory to search in

        Returns:
            Dictionary with matches list
        """
        return self._get_file_tools().find_file(
            file_mask=file_mask,
            relative_path=relative_path,
        )

    def c4_search_for_pattern(
        self,
        pattern: str,
        relative_path: str = ".",
        glob_pattern: str | None = None,
        context_lines: int = 0,
    ) -> dict[str, Any]:
        """Search for a regex pattern in files.

        Args:
            pattern: Regular expression pattern
            relative_path: Directory or file to search in
            glob_pattern: Optional glob to filter files
            context_lines: Number of context lines before/after match

        Returns:
            Dictionary with matches list
        """
        return self._get_file_tools().search_for_pattern(
            pattern=pattern,
            relative_path=relative_path,
            glob_pattern=glob_pattern,
            context_lines=context_lines,
        )

    def c4_replace_content(
        self,
        relative_path: str,
        needle: str,
        replacement: str,
        mode: str = "literal",
        allow_multiple: bool = False,
    ) -> dict[str, Any]:
        """Replace content in a file.

        Args:
            relative_path: Path relative to project root
            needle: String or regex pattern to search for
            replacement: Replacement string
            mode: 'literal' for exact match, 'regex' for regex
            allow_multiple: Whether to allow multiple replacements

        Returns:
            Dictionary with success status and replacements_made count
        """
        return self._get_file_tools().replace_content(
            relative_path=relative_path,
            needle=needle,
            replacement=replacement,
            mode=mode,
            allow_multiple=allow_multiple,
        )

    def check_and_trigger_checkpoint(self) -> dict[str, Any] | None:
        """
        Check if checkpoint conditions are met and trigger if so.

        Returns:
            Checkpoint info if triggered, None otherwise
        """
        if self.state_machine is None:
            return None

        state = self.state_machine.state

        # Only check in EXECUTE state
        if state.status != ProjectStatus.EXECUTE:
            return None

        # Check gate conditions
        cp_id = self.state_machine.check_gate_conditions(self.config)
        if cp_id:
            # Enter checkpoint state
            self.state_machine.enter_checkpoint(cp_id)

            return {
                "checkpoint_id": cp_id,
                "triggered": True,
                "message": f"Checkpoint {cp_id} conditions met, entering CHECKPOINT state",
            }

        return None

    # =========================================================================
    # Supervisor Integration
    # =========================================================================

    def create_checkpoint_bundle(self, checkpoint_id: str | None = None) -> Path:
        """
        Create a bundle for supervisor review.

        Args:
            checkpoint_id: Checkpoint ID (uses current if not specified)

        Returns:
            Path to the created bundle directory
        """
        from .bundle import BundleCreator

        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        state = self.state_machine.state

        # Use current checkpoint if not specified
        if checkpoint_id is None:
            checkpoint_id = state.checkpoint.current
        if checkpoint_id is None:
            raise ValueError("No checkpoint ID specified or active")

        # Get completed tasks
        tasks_completed = list(state.queue.done)

        # Get last validation results
        validation_results = []
        if state.last_validation:
            for name, status in state.last_validation.items():
                validation_results.append({"name": name, "status": status})

        # Create bundle
        bundle_creator = BundleCreator(self.root, self.c4_dir)
        bundle_dir = bundle_creator.create_bundle(
            checkpoint_id=checkpoint_id,
            tasks_completed=tasks_completed,
            validation_results=validation_results,
        )

        return bundle_dir

    def run_supervisor_review(
        self,
        bundle_dir: Path | None = None,
        use_mock: bool = False,
        mock_decision: str = "APPROVE",
        timeout: int = 300,
        max_retries: int = 3,
    ) -> dict[str, Any]:
        """
        Run supervisor review on a checkpoint bundle.

        Args:
            bundle_dir: Path to bundle (creates new if None)
            use_mock: Use mock supervisor instead of real Claude CLI
            mock_decision: Decision for mock supervisor
            timeout: Timeout for real supervisor
            max_retries: Max retries for real supervisor

        Returns:
            Dictionary with supervisor decision and processing result
        """
        from .models import SupervisorDecision
        from .supervisor import Supervisor, SupervisorError

        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        state = self.state_machine.state

        # Ensure we're in CHECKPOINT state
        if state.status != ProjectStatus.CHECKPOINT:
            return {
                "success": False,
                "error": f"Not in CHECKPOINT state (current: {state.status.value})",
            }

        # Create bundle if not provided
        if bundle_dir is None:
            bundle_dir = self.create_checkpoint_bundle()

        # Run supervisor
        supervisor = Supervisor(self.root, prompts_dir=self.root / "prompts")

        try:
            if use_mock:
                decision_enum = SupervisorDecision(mock_decision)
                response = supervisor.run_supervisor_mock(
                    bundle_dir,
                    mock_decision=decision_enum,
                    mock_notes=f"Mock {mock_decision} decision",
                    mock_changes=["Mock change 1"] if mock_decision == "REQUEST_CHANGES" else None,
                )
            else:
                response = supervisor.run_supervisor(
                    bundle_dir,
                    timeout=timeout,
                    max_retries=max_retries,
                )

            # Process the decision
            return self.process_supervisor_decision(response)

        except SupervisorError as e:
            return {"success": False, "error": str(e)}

    def process_supervisor_decision(self, response: Any) -> dict[str, Any]:
        """
        Process supervisor decision and update state accordingly.

        Args:
            response: SupervisorResponse from supervisor

        Returns:
            Dictionary with processing result
        """
        if self.state_machine is None:
            raise RuntimeError("C4 not initialized")

        # Apply decision via c4_checkpoint
        checkpoint_response = self.c4_checkpoint(
            checkpoint_id=response.checkpoint_id,
            decision=response.decision.value,
            notes=response.notes,
            required_changes=response.required_changes if response.required_changes else None,
        )

        return {
            "success": checkpoint_response.success,
            "decision": response.decision.value,
            "checkpoint_id": response.checkpoint_id,
            "notes": response.notes,
            "required_changes": response.required_changes,
            "new_status": self.state_machine.state.status.value,
            "message": checkpoint_response.message,
        }

    def trigger_lsp_reindex(self, files: list[str]) -> None:
        """Trigger LSP symbol cache invalidation for changed files.

        Called by SupervisorLoop when git commit events are processed.
        This method invalidates the symbol cache for the specified files,
        ensuring that subsequent symbol lookups return fresh data.

        Args:
            files: List of relative file paths that were changed.
                   Only Python files (.py) are processed for symbol indexing.

        Note:
            Currently, symbol resolution via Jedi is stateless and always
            reads the latest file content. This method serves as a hook
            for future caching implementations and logs the reindex trigger.
        """
        python_files = [f for f in files if f.endswith(".py")]
        if not python_files:
            return

        logger.debug(
            f"LSP reindex triggered for {len(python_files)} files: "
            f"{', '.join(python_files[:5])}{'...' if len(python_files) > 5 else ''}"
        )

        # Jedi-based symbol resolution is stateless (reads file on each call)
        # No explicit cache invalidation needed currently.
        # This method serves as a hook for future caching implementations.

        # If we had a symbol cache, we would invalidate it here:
        # for file in python_files:
        #     self._symbol_cache.invalidate(file)

    # =========================================================================
    # LSP Server Control
    # =========================================================================

    def c4_lsp_start(self, port: int = 2088) -> dict[str, Any]:
        """Start the C4 LSP server in a background thread.

        Args:
            port: Port to run the LSP server on (default: 2088)

        Returns:
            Dictionary with success status and server info
        """
        import threading

        # Check if already running
        if self._lsp_thread is not None and self._lsp_thread.is_alive():
            return {
                "success": False,
                "error": f"LSP server already running on port {self._lsp_port}",
            }

        try:
            from c4.lsp.server import PYGLS_AVAILABLE, C4LSPServer

            if not PYGLS_AVAILABLE:
                return {
                    "success": False,
                    "error": "pygls is required for LSP support. Install with: uv add pygls",
                }

            # Create server instance
            self._lsp_server = C4LSPServer()
            self._lsp_server.set_workspace_root(self.root)
            self._lsp_port = port

            # Start server in background thread
            def run_server():
                try:
                    self._lsp_server.start_tcp("localhost", port)
                except Exception as e:
                    logger.error(f"LSP server error: {e}")

            self._lsp_thread = threading.Thread(
                target=run_server,
                name="c4-lsp-server",
                daemon=True,
            )
            self._lsp_thread.start()

            logger.info(f"LSP server started on port {port}")
            return {
                "success": True,
                "port": port,
                "message": f"LSP server started on localhost:{port}",
            }

        except ImportError as e:
            return {
                "success": False,
                "error": f"Failed to import LSP server: {e}",
            }
        except Exception as e:
            logger.error(f"Failed to start LSP server: {e}")
            return {
                "success": False,
                "error": str(e),
            }

    def c4_lsp_stop(self) -> dict[str, Any]:
        """Stop the running C4 LSP server.

        Returns:
            Dictionary with success status
        """
        if self._lsp_thread is None or not self._lsp_thread.is_alive():
            return {
                "success": False,
                "error": "LSP server is not running",
            }

        try:
            # Signal server to stop
            if self._lsp_server is not None:
                self._lsp_server.stop()

            # Wait for thread to finish (with timeout)
            self._lsp_thread.join(timeout=5.0)

            port = self._lsp_port
            self._lsp_server = None
            self._lsp_thread = None
            self._lsp_port = None

            logger.info("LSP server stopped")
            return {
                "success": True,
                "message": f"LSP server stopped (was on port {port})",
            }

        except Exception as e:
            logger.error(f"Failed to stop LSP server: {e}")
            return {
                "success": False,
                "error": str(e),
            }

    def c4_lsp_status(self) -> dict[str, Any]:
        """Get the status of the C4 LSP server.

        Returns:
            Dictionary with server status information
        """
        if self._lsp_thread is None:
            return {
                "running": False,
                "status": "not_started",
                "message": "LSP server not started",
            }

        if not self._lsp_thread.is_alive():
            # Thread existed but died
            self._lsp_server = None
            self._lsp_thread = None
            old_port = self._lsp_port
            self._lsp_port = None
            return {
                "running": False,
                "status": "stopped",
                "message": f"LSP server stopped (was on port {old_port})",
            }

        # Server is running - gather stats
        features = [
            "textDocument/hover",
            "textDocument/definition",
            "textDocument/references",
            "textDocument/documentSymbol",
            "workspace/symbol",
            "textDocument/completion",
        ]

        indexed_files = 0
        total_symbols = 0

        # Try to get stats from server's analyzer if available
        if self._lsp_server is not None:
            try:
                analyzer = getattr(self._lsp_server, "analyzer", None)
                if analyzer:
                    file_contents = getattr(analyzer, "_file_contents", {})
                    indexed_files = len(file_contents)
                    all_symbols = analyzer.get_all_symbols() if hasattr(analyzer, "get_all_symbols") else []
                    total_symbols = len(all_symbols)
            except Exception:
                pass  # Stats unavailable, use defaults

        return {
            "running": True,
            "status": "running",
            "port": self._lsp_port,
            "message": f"LSP server running on localhost:{self._lsp_port}",
            "features": features,
            "indexed_files": indexed_files,
            "total_symbols": total_symbols,
        }

    # =========================================================================
    # Plan File Sync Helpers
    # =========================================================================

    def _sync_to_plan_file(self) -> None:
        """Sync C4 tasks to plan file (C4 → Plan).

        Regenerates the plan file with current task state.
        Only runs if plan_sync is enabled in config.
        """
        if not self._is_plan_sync_enabled():
            return

        try:
            from pathlib import Path

            from .plan_sync import PlanFileSync

            plan_dir = None
            if self.config.plan_sync.plan_dir:
                plan_dir = Path(self.config.plan_sync.plan_dir)

            sync = PlanFileSync(plan_dir=plan_dir)
            project_id = self.state_machine.state.project_id

            # Get all tasks
            tasks = list(self.task_store.load_all(project_id).values())

            # Generate plan file
            sync.generate_plan_file(project_id, tasks)
            logger.debug(f"Plan file synced for project {project_id}")
        except Exception as e:
            logger.warning(f"Failed to sync plan file: {e}")

    def _sync_from_plan_file(self) -> None:
        """Sync changes from plan file to C4 (Plan → C4).

        Checks plan file for status changes and new tasks.
        Only runs if plan_sync is enabled in config.
        """
        if not self._is_plan_sync_enabled():
            return

        try:
            from pathlib import Path

            from .plan_sync import PlanFileSync

            plan_dir = None
            if self.config.plan_sync.plan_dir:
                plan_dir = Path(self.config.plan_sync.plan_dir)

            sync = PlanFileSync(plan_dir=plan_dir)
            project_id = self.state_machine.state.project_id

            if not sync.has_plan_file(project_id):
                return

            # Get current C4 tasks
            c4_tasks = self.task_store.load_all(project_id)

            # Detect changes
            changes = sync.sync_from_plan_file(project_id, c4_tasks)

            # Apply status updates (plan file → C4)
            for update in changes["status_updates"]:
                task_id = update["task_id"]
                new_status = update["new_status"]
                if new_status == "done":
                    # Mark task as done in C4 state
                    self._mark_task_done_from_plan(task_id)
                    logger.info(f"Plan sync: marked {task_id} as done")

            # Apply plan file updates (C4 → plan file)
            for plan_update in changes["plan_updates"]:
                sync.update_task_status(
                    project_id,
                    plan_update["task_id"],
                    plan_update["new_status"],
                )

            # Note: new_tasks from plan file are logged but not auto-added
            # This requires manual c4_add_todo to maintain task ID consistency
            if changes["new_tasks"]:
                logger.info(
                    f"Plan sync: {len(changes['new_tasks'])} new tasks detected "
                    "in plan file (use c4_add_todo to add them)"
                )
        except Exception as e:
            logger.warning(f"Failed to sync from plan file: {e}")

    def _update_plan_task_status(self, task_id: str, status: str) -> None:
        """Update a single task's status in plan file.

        Args:
            task_id: The task ID to update
            status: New status ("done" or "pending")
        """
        if not self._is_plan_sync_enabled():
            return

        if not self.config.plan_sync.auto_update_status:
            return

        try:
            from pathlib import Path

            from .plan_sync import PlanFileSync

            plan_dir = None
            if self.config.plan_sync.plan_dir:
                plan_dir = Path(self.config.plan_sync.plan_dir)

            sync = PlanFileSync(plan_dir=plan_dir)
            project_id = self.state_machine.state.project_id

            sync.update_task_status(project_id, task_id, status)
            logger.debug(f"Plan file: updated {task_id} to {status}")
        except Exception as e:
            logger.warning(f"Failed to update plan task status: {e}")

    def _is_plan_sync_enabled(self) -> bool:
        """Check if plan sync is enabled in config."""
        if self.state_machine is None:
            return False
        if not hasattr(self.config, "plan_sync"):
            return False
        return self.config.plan_sync.enabled

    def _mark_task_done_from_plan(self, task_id: str) -> None:
        """Mark a task as done based on plan file change.

        This is a simplified version - doesn't run validations.
        Used for syncing from plan file edits.
        """
        project_id = self.state_machine.state.project_id
        state = self.state_machine.state

        # Move from pending/in_progress to done
        if task_id in state.queue.pending:
            state.queue.pending.remove(task_id)
            state.queue.done.append(task_id)
        elif task_id in state.queue.in_progress:
            del state.queue.in_progress[task_id]
            state.queue.done.append(task_id)

        # Update task store
        self.task_store.update_status(project_id, task_id, status="done")

        # Save state
        self.state_machine.save_state()

    # =========================================================================
    # Commit Analysis (Claude Code ↔ C4 Sync)
    # =========================================================================

    def notify_commit(
        self, commit_sha: str, min_confidence: float = 0.7
    ) -> dict[str, Any]:
        """Analyze a commit and update matching tasks.

        This enables work done by Claude Code outside C4 to be reflected
        in C4's task tracking. Called via post-commit hook or manually.

        Flow:
        1. Analyze commit → match to C4 tasks
        2. Update matched tasks to done
        3. PlanFileSync propagates to Claude Code plan file

        Args:
            commit_sha: The commit SHA to analyze
            min_confidence: Minimum confidence for auto-update (default 0.7)

        Returns:
            Dictionary with:
                - success: Whether the operation succeeded
                - matches: List of matched tasks
                - updated: List of tasks that were updated
        """
        from .commit_analyzer import CommitAnalyzer

        try:
            if self.state_machine is None:
                return {"success": False, "error": "C4 not initialized"}

            project_id = self.state_machine.state.project_id
            analyzer = CommitAnalyzer(self.root)

            # Get current tasks
            c4_tasks = self.task_store.load_all(project_id)

            # Analyze commit
            matches = analyzer.analyze_and_suggest(
                commit_sha, c4_tasks, min_confidence
            )

            if not matches:
                logger.debug(f"No task matches found for commit {commit_sha}")
                return {
                    "success": True,
                    "matches": [],
                    "updated": [],
                    "commit": commit_sha,
                }

            # Update matched tasks
            updated = []
            for match in matches:
                task = c4_tasks.get(match.task_id)
                if task and task.status.value != "done":
                    self._mark_task_done_from_plan(match.task_id)
                    updated.append({
                        "task_id": match.task_id,
                        "confidence": match.confidence,
                        "reason": match.reason,
                    })
                    logger.info(
                        f"Commit {commit_sha[:8]}: marked {match.task_id} as done "
                        f"(confidence: {match.confidence:.0%}, reason: {match.reason})"
                    )

            # Sync to plan file if enabled
            self._sync_to_plan_file()

            return {
                "success": True,
                "commit": commit_sha,
                "matches": [
                    {
                        "task_id": m.task_id,
                        "confidence": m.confidence,
                        "reason": m.reason,
                    }
                    for m in matches
                ],
                "updated": updated,
            }

        except Exception as e:
            logger.error(f"Failed to analyze commit: {e}")
            return {"success": False, "error": str(e)}

    def analyze_recent_commits(
        self, since_sha: str | None = None, min_confidence: float = 0.7
    ) -> dict[str, Any]:
        """Analyze commits since a given SHA and update tasks.

        Useful for catching up after working without C4.

        Args:
            since_sha: Start commit (exclusive). If None, analyzes last commit.
            min_confidence: Minimum confidence for auto-update

        Returns:
            Dictionary with analysis results per commit
        """
        from .commit_analyzer import CommitAnalyzer

        try:
            if self.state_machine is None:
                return {"success": False, "error": "C4 not initialized"}

            analyzer = CommitAnalyzer(self.root)
            commits = analyzer.get_commits_since(since_sha)

            results = []
            for commit_sha in commits:
                result = self.notify_commit(commit_sha, min_confidence)
                results.append(result)

            total_updated = sum(len(r.get("updated", [])) for r in results)
            return {
                "success": True,
                "commits_analyzed": len(commits),
                "tasks_updated": total_updated,
                "results": results,
            }

        except Exception as e:
            logger.error(f"Failed to analyze recent commits: {e}")
            return {"success": False, "error": str(e)}


# =============================================================================
# MCP Server Setup
# =============================================================================


