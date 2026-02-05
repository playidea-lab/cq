"""C4 Daemon - Main orchestrator for C4 project management."""

import logging
from datetime import datetime
from pathlib import Path
from typing import Any

from ..constants import REPAIR_PREFIX, REPAIR_PREFIX_LEN
from ..discovery import (
    DesignStore,
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
    SubmitResponse,
    Task,
    TaskAssignment,
    TaskStatus,
    TaskType,
    ValidationResult,
)
from ..state_machine import StateMachine
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
from .checkpoint_ops import CheckpointOps
from .code_ops import CodeOps
from .design_ops import DesignOps
from .discovery_ops import DiscoveryOps
from .git_ops import GitOperations
from .pr_manager import PRManager
from .task_ops import TaskOps
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
        # Modular operation handlers (lazy initialization)
        self._task_ops: TaskOps | None = None
        self._code_ops: CodeOps | None = None
        self._discovery_ops: DiscoveryOps | None = None
        self._design_ops: DesignOps | None = None
        self._checkpoint_ops: CheckpointOps | None = None

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

        # Load config first to get project_id
        self._load_config()

        # Initialize state machine with project_id from config
        self.state_machine = StateMachine(self.c4_dir, store=self._get_default_store())
        self.state_machine.load_state(self.config.project_id)
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
        """Sync a verification requirement to config.yaml. Delegates to DiscoveryOps."""
        return self.discovery_ops._sync_verification_to_config(verification)

    def _apply_domain_default_verifications(self, domain: str) -> list[str]:
        """Apply default verifications for a domain. Delegates to DiscoveryOps."""
        return self.discovery_ops._apply_domain_default_verifications(domain)

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

    @property
    def task_ops(self) -> TaskOps:
        """Get task operations handler, creating if necessary."""
        if self._task_ops is None:
            self._task_ops = TaskOps(self)
        return self._task_ops

    @property
    def code_ops(self) -> CodeOps:
        """Get code operations handler, creating if necessary."""
        if self._code_ops is None:
            self._code_ops = CodeOps(self)
        return self._code_ops

    @property
    def discovery_ops(self) -> DiscoveryOps:
        """Get discovery operations handler, creating if necessary."""
        if self._discovery_ops is None:
            self._discovery_ops = DiscoveryOps(self)
        return self._discovery_ops

    @property
    def design_ops(self) -> DesignOps:
        """Get design operations handler, creating if necessary."""
        if self._design_ops is None:
            self._design_ops = DesignOps(self)
        return self._design_ops

    @property
    def checkpoint_ops(self) -> CheckpointOps:
        """Get checkpoint operations handler, creating if necessary."""
        if self._checkpoint_ops is None:
            self._checkpoint_ops = CheckpointOps(self)
        return self._checkpoint_ops

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

    def get_all_tasks(self) -> dict[str, Task]:
        """Get all tasks (public accessor for _tasks)."""
        return self._tasks

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
        from .git_ops import GitResult

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

        Returns:
            TaskAssignment with task details, or None if no tasks available

        Delegates to: TaskOps.get_task()
        """
        return self.task_ops.get_task(worker_id, model_filter)

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

        Args:
            task_id: ID of the completed task
            commit_sha: Git commit SHA of the work
            validation_results: List of validation result dicts
            worker_id: Worker ID submitting the task
            review_result: Review decision for review tasks
            review_comments: Comments for review tasks

        Returns:
            SubmitResponse with success status and next action

        Delegates to: TaskOps.submit()
        """
        return self.task_ops.submit(
            task_id, commit_sha, validation_results, worker_id, review_result, review_comments
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

        Returns:
            Dictionary with success status and task details

        Delegates to: TaskOps.add_todo()
        """
        return self.task_ops.add_todo(
            task_id, title, scope, dod, dependencies, domain, priority, model
        )

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
        """Record checkpoint decision. Delegates to CheckpointOps.checkpoint()."""
        return self.checkpoint_ops.checkpoint(
            checkpoint_id=checkpoint_id,
            decision=decision,
            notes=notes,
            required_changes=required_changes,
        )

    def c4_mark_blocked(
        self,
        task_id: str,
        worker_id: str,
        failure_signature: str,
        attempts: int,
        last_error: str = "",
    ) -> dict[str, Any]:
        """Mark a task as blocked after max retry attempts.

        Adds the task to repair queue for supervisor guidance.

        Args:
            task_id: ID of the blocked task
            worker_id: ID of the worker that was working on the task
            failure_signature: Error signature from validation failures
            attempts: Number of fix attempts made
            last_error: Last error message

        Returns:
            Dictionary with success status and message

        Delegates to: TaskOps.mark_blocked()
        """
        return self.task_ops.mark_blocked(
            task_id, worker_id, failure_signature, attempts, last_error
        )

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
        """Save feature specification. Delegates to DiscoveryOps.save_spec()."""
        return self.discovery_ops.save_spec(
            feature=feature,
            requirements=requirements,
            domain=domain,
            description=description,
        )

    def c4_list_specs(self) -> dict[str, Any]:
        """List all feature specifications. Delegates to DiscoveryOps.list_specs()."""
        return self.discovery_ops.list_specs()

    def c4_get_spec(self, feature: str) -> dict[str, Any]:
        """Get a specific feature specification. Delegates to DiscoveryOps.get_spec()."""
        return self.discovery_ops.get_spec(feature)

    def c4_add_verification(
        self,
        feature: str,
        verification_type: str,
        name: str,
        reason: str,
        priority: int = 2,
        config: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        """Add verification requirement. Delegates to DiscoveryOps.add_verification()."""
        return self.discovery_ops.add_verification(
            feature=feature,
            verification_type=verification_type,
            name=name,
            reason=reason,
            priority=priority,
            config=config,
        )

    def c4_get_feature_verifications(self, feature: str) -> dict[str, Any]:
        """Get verifications for a feature. Delegates to DiscoveryOps.get_feature_verifications()."""
        return self.discovery_ops.get_feature_verifications(feature)

    def c4_discovery_complete(self) -> dict[str, Any]:
        """Complete discovery phase. Delegates to DiscoveryOps.discovery_complete()."""
        return self.discovery_ops.discovery_complete()

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
        """Save design specification. Delegates to DesignOps.save_design()."""
        return self.design_ops.save_design(
            feature=feature,
            domain=domain,
            selected_option=selected_option,
            options=options,
            components=components,
            decisions=decisions,
            mermaid_diagram=mermaid_diagram,
            constraints=constraints,
            nfr=nfr,
            description=description,
        )

    def c4_get_design(self, feature: str) -> dict[str, Any]:
        """Get design specification. Delegates to DesignOps.get_design()."""
        return self.design_ops.get_design(feature)

    def c4_list_designs(self) -> dict[str, Any]:
        """List all designs. Delegates to DesignOps.list_designs()."""
        return self.design_ops.list_designs()

    def c4_design_complete(self) -> dict[str, Any]:
        """Complete design phase. Delegates to DesignOps.design_complete()."""
        return self.design_ops.design_complete()

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

    # =========================================================================
    # Symbol Operation Tools (delegates to CodeOps)
    # =========================================================================

    def c4_replace_symbol_body(
        self,
        name_path: str,
        file_path: str | None,
        new_body: str,
    ) -> dict[str, Any]:
        """Replace the body of a symbol. Delegates to CodeOps.replace_symbol_body()."""
        return self.code_ops.replace_symbol_body(
            name_path=name_path,
            file_path=file_path,
            new_body=new_body,
        )

    def c4_insert_before_symbol(
        self,
        name_path: str,
        file_path: str | None,
        content: str,
    ) -> dict[str, Any]:
        """Insert content before a symbol. Delegates to CodeOps.insert_before_symbol()."""
        return self.code_ops.insert_before_symbol(
            name_path=name_path,
            file_path=file_path,
            content=content,
        )

    def c4_insert_after_symbol(
        self,
        name_path: str,
        file_path: str | None,
        content: str,
    ) -> dict[str, Any]:
        """Insert content after a symbol. Delegates to CodeOps.insert_after_symbol()."""
        return self.code_ops.insert_after_symbol(
            name_path=name_path,
            file_path=file_path,
            content=content,
        )

    def c4_rename_symbol(
        self,
        name_path: str,
        file_path: str | None,
        new_name: str,
    ) -> dict[str, Any]:
        """Rename a symbol across the codebase. Delegates to CodeOps.rename_symbol()."""
        return self.code_ops.rename_symbol(
            name_path=name_path,
            file_path=file_path,
            new_name=new_name,
        )

    def c4_find_symbol(
        self,
        name_path_pattern: str,
        relative_path: str = "",
        include_body: bool = False,
        depth: int = 0,
    ) -> dict[str, Any]:
        """Find symbols matching the name path pattern.

        Delegates to CodeOps.find_symbol().
        """
        return self.code_ops.find_symbol(
            name_path_pattern=name_path_pattern,
            relative_path=relative_path,
            include_body=include_body,
            depth=depth,
        )

    def c4_get_symbols_overview(
        self,
        relative_path: str,
        depth: int = 0,
    ) -> dict[str, Any]:
        """Get an overview of symbols in a file.

        Delegates to CodeOps.get_symbols_overview().
        """
        return self.code_ops.get_symbols_overview(
            relative_path=relative_path,
            depth=depth,
        )

    def c4_find_referencing_symbols(
        self,
        name_path: str,
        file_path: str | None = None,
    ) -> dict[str, Any]:
        """Find all references to a symbol in the codebase.

        Delegates to CodeOps.find_referencing_symbols().
        """
        return self.code_ops.find_referencing_symbols(
            name_path=name_path,
            file_path=file_path,
        )

    # =========================================================================
    # File Operation Tools (delegates to CodeOps)
    # =========================================================================

    def c4_read_file(
        self,
        relative_path: str,
        start_line: int = 0,
        end_line: int | None = None,
    ) -> dict[str, Any]:
        """Read a file or portion of it. Delegates to CodeOps.read_file()."""
        return self.code_ops.read_file(
            relative_path=relative_path,
            start_line=start_line,
            end_line=end_line,
        )

    def c4_create_text_file(
        self,
        relative_path: str,
        content: str,
    ) -> dict[str, Any]:
        """Create or overwrite a text file. Delegates to CodeOps.create_text_file()."""
        return self.code_ops.create_text_file(
            relative_path=relative_path,
            content=content,
        )

    def c4_list_dir(
        self,
        relative_path: str = ".",
        recursive: bool = False,
    ) -> dict[str, Any]:
        """List files and directories. Delegates to CodeOps.list_dir()."""
        return self.code_ops.list_dir(
            relative_path=relative_path,
            recursive=recursive,
        )

    def c4_find_file(
        self,
        file_mask: str,
        relative_path: str = ".",
    ) -> dict[str, Any]:
        """Find files matching a pattern. Delegates to CodeOps.find_file()."""
        return self.code_ops.find_file(
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
        """Search for a regex pattern in files. Delegates to CodeOps.search_for_pattern()."""
        return self.code_ops.search_for_pattern(
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
        """Replace content in a file. Delegates to CodeOps.replace_content()."""
        return self.code_ops.replace_content(
            relative_path=relative_path,
            needle=needle,
            replacement=replacement,
            mode=mode,
            allow_multiple=allow_multiple,
        )

    def check_and_trigger_checkpoint(self) -> dict[str, Any] | None:
        """Check if checkpoint conditions are met and trigger if so.

        Delegates to CheckpointOps.check_and_trigger_checkpoint().
        """
        return self.checkpoint_ops.check_and_trigger_checkpoint()

    # =========================================================================
    # Supervisor Integration
    # =========================================================================

    def create_checkpoint_bundle(self, checkpoint_id: str | None = None) -> Path:
        """Create a bundle for supervisor review.

        Delegates to CheckpointOps.create_checkpoint_bundle().
        """
        return self.checkpoint_ops.create_checkpoint_bundle(checkpoint_id=checkpoint_id)

    def run_supervisor_review(
        self,
        bundle_dir: Path | None = None,
        use_mock: bool = False,
        mock_decision: str = "APPROVE",
        timeout: int = 300,
        max_retries: int = 3,
    ) -> dict[str, Any]:
        """Run supervisor review on a checkpoint bundle.

        Delegates to CheckpointOps.run_supervisor_review().
        """
        return self.checkpoint_ops.run_supervisor_review(
            bundle_dir=bundle_dir,
            use_mock=use_mock,
            mock_decision=mock_decision,
            timeout=timeout,
            max_retries=max_retries,
        )

    def process_supervisor_decision(self, response: Any) -> dict[str, Any]:
        """Process supervisor decision and update state accordingly.

        Delegates to CheckpointOps.process_supervisor_decision().
        """
        return self.checkpoint_ops.process_supervisor_decision(response)

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


