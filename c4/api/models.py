"""Pydantic models for C4 API request/response schemas."""

from datetime import datetime
from enum import Enum
from typing import Any

from pydantic import BaseModel, Field

# ============================================================================
# Common Models
# ============================================================================


class ValidationStatus(str, Enum):
    """Status of a validation result."""

    PASS = "pass"
    FAIL = "fail"


class ValidationResult(BaseModel):
    """Result of a single validation."""

    name: str = Field(..., description="Validation name (e.g., 'lint', 'test')")
    status: ValidationStatus = Field(..., description="Pass or fail")
    message: str | None = Field(None, description="Optional message or error")


# ============================================================================
# C4 Core Models
# ============================================================================


class StatusResponse(BaseModel):
    """Response from c4_status endpoint."""

    state: str = Field(
        ..., description="Current C4 state (INIT, DISCOVERY, DESIGN, etc.)"
    )
    queue: dict[str, Any] = Field(default_factory=dict, description="Task queue summary")
    workers: dict[str, Any] = Field(default_factory=dict, description="Active workers")
    project_root: str | None = Field(None, description="Project root path")


class GetTaskRequest(BaseModel):
    """Request to get a task assignment."""

    worker_id: str = Field(..., description="Unique worker identifier")


class GetTaskResponse(BaseModel):
    """Response with assigned task details."""

    task_id: str | None = Field(None, description="Assigned task ID")
    title: str | None = Field(None, description="Task title")
    dod: str | None = Field(None, description="Definition of Done")
    scope: str | None = Field(None, description="File/directory scope")
    domain: str | None = Field(None, description="Task domain")
    dependencies: list[str] = Field(default_factory=list, description="Task dependencies")
    message: str | None = Field(None, description="Status message if no task available")


class SubmitRequest(BaseModel):
    """Request to submit task completion."""

    task_id: str = Field(..., description="ID of the completed task")
    commit_sha: str = Field(..., description="Git commit SHA of the work")
    validation_results: list[ValidationResult] = Field(..., description="Validation results")
    worker_id: str = Field(..., description="Worker ID for ownership verification")


class SubmitResponse(BaseModel):
    """Response from task submission."""

    success: bool = Field(..., description="Whether submission was successful")
    message: str = Field(..., description="Status message")
    next_task: dict[str, Any] | None = Field(None, description="Next task if available")


class AddTaskRequest(BaseModel):
    """Request to add a new task."""

    task_id: str = Field(..., description="Unique task ID (e.g., T-001)")
    title: str = Field(..., description="Task title")
    dod: str = Field(..., description="Definition of Done")
    scope: str | None = Field(None, description="File/directory scope for lock")
    domain: str | None = Field(None, description="Domain for agent routing")
    priority: int = Field(0, description="Priority (higher = first)")
    dependencies: list[str] = Field(
        default_factory=list, description="Task IDs that must complete first"
    )


class StartResponse(BaseModel):
    """Response from c4_start endpoint."""

    success: bool = Field(..., description="Whether state transition was successful")
    new_state: str = Field(..., description="New C4 state")
    message: str = Field(..., description="Status message")


# ============================================================================
# Discovery Phase Models
# ============================================================================


class EarsPattern(str, Enum):
    """EARS requirement pattern types."""

    UBIQUITOUS = "ubiquitous"
    STATE_DRIVEN = "state-driven"
    EVENT_DRIVEN = "event-driven"
    OPTIONAL = "optional"
    UNWANTED = "unwanted"


class Requirement(BaseModel):
    """An EARS-format requirement."""

    id: str = Field(..., description="Requirement ID (e.g., REQ-001)")
    text: str = Field(..., description="Full EARS requirement text")
    pattern: EarsPattern | None = Field(None, description="EARS pattern type")


class SaveSpecRequest(BaseModel):
    """Request to save a feature specification."""

    feature: str = Field(..., description="Feature name (e.g., 'user-auth')")
    requirements: list[Requirement] = Field(..., description="List of EARS requirements")
    domain: str = Field(..., description="Project domain")
    description: str | None = Field(None, description="Optional feature description")


class SpecResponse(BaseModel):
    """Response containing a feature specification."""

    feature: str
    requirements: list[Requirement]
    domain: str
    description: str | None = None


class ListSpecsResponse(BaseModel):
    """Response listing all specifications."""

    specs: list[str] = Field(..., description="List of feature names")


# ============================================================================
# Design Phase Models
# ============================================================================


class ArchitectureOption(BaseModel):
    """An architecture option for design decisions."""

    id: str = Field(..., description="Option ID")
    name: str = Field(..., description="Option name")
    description: str = Field(..., description="Option description")
    pros: list[str] = Field(default_factory=list, description="Advantages")
    cons: list[str] = Field(default_factory=list, description="Disadvantages")
    complexity: str | None = Field(None, description="low/medium/high")
    recommended: bool = Field(False, description="Whether this is recommended")


class ComponentDesign(BaseModel):
    """Design specification for a component."""

    name: str = Field(..., description="Component name")
    type: str = Field(..., description="Component type (service, repository, etc.)")
    description: str = Field(..., description="Component description")
    responsibilities: list[str] = Field(default_factory=list)
    interfaces: list[str] = Field(default_factory=list)
    dependencies: list[str] = Field(default_factory=list)


class DesignDecision(BaseModel):
    """A design decision."""

    id: str = Field(..., description="Decision ID")
    question: str = Field(..., description="The question being decided")
    decision: str = Field(..., description="The decision made")
    rationale: str = Field(..., description="Why this decision was made")
    alternatives_considered: list[str] = Field(default_factory=list)


class SaveDesignRequest(BaseModel):
    """Request to save a design specification."""

    feature: str = Field(..., description="Feature name")
    domain: str = Field(..., description="Project domain")
    description: str | None = Field(None)
    options: list[ArchitectureOption] = Field(default_factory=list)
    selected_option: str | None = Field(None, description="ID of selected option")
    components: list[ComponentDesign] = Field(default_factory=list)
    decisions: list[DesignDecision] = Field(default_factory=list)
    constraints: list[str] = Field(default_factory=list)
    nfr: dict[str, str] = Field(default_factory=dict, description="Non-functional requirements")
    mermaid_diagram: str | None = Field(None)


class DesignResponse(BaseModel):
    """Response containing a design specification."""

    feature: str
    domain: str
    description: str | None = None
    options: list[ArchitectureOption] = Field(default_factory=list)
    selected_option: str | None = None
    components: list[ComponentDesign] = Field(default_factory=list)
    decisions: list[DesignDecision] = Field(default_factory=list)


class ListDesignsResponse(BaseModel):
    """Response listing all designs."""

    designs: list[str] = Field(..., description="List of feature names with designs")


# ============================================================================
# Validation Models
# ============================================================================


class RunValidationRequest(BaseModel):
    """Request to run validations."""

    names: list[str] | None = Field(None, description="Validation names to run (None = all)")
    fail_fast: bool = Field(True, description="Stop on first failure")
    timeout: int = Field(300, description="Timeout per validation in seconds")


class RunValidationResponse(BaseModel):
    """Response from validation run."""

    results: list[ValidationResult]
    all_passed: bool
    duration_seconds: float


# ============================================================================
# Git Models
# ============================================================================


class GitCommitRequest(BaseModel):
    """Request to create a git commit."""

    task_id: str = Field(..., description="Task ID for commit message")
    message: str | None = Field(None, description="Optional custom message")


class GitCommitResponse(BaseModel):
    """Response from git commit."""

    success: bool
    commit_sha: str | None = None
    message: str = ""


class GitStatusResponse(BaseModel):
    """Response from git status."""

    branch: str
    is_clean: bool
    staged: list[str] = Field(default_factory=list)
    modified: list[str] = Field(default_factory=list)
    untracked: list[str] = Field(default_factory=list)


# ============================================================================
# Checkpoint Models
# ============================================================================


class CheckpointDecision(str, Enum):
    """Supervisor checkpoint decisions."""

    APPROVE = "APPROVE"
    REQUEST_CHANGES = "REQUEST_CHANGES"
    REPLAN = "REPLAN"


class CheckpointRequest(BaseModel):
    """Request to record a checkpoint decision."""

    checkpoint_id: str = Field(..., description="Checkpoint ID")
    decision: CheckpointDecision = Field(..., description="Decision")
    notes: str = Field(..., description="Decision notes")
    required_changes: list[str] = Field(default_factory=list, description="Changes required")


class CheckpointResponse(BaseModel):
    """Response from checkpoint recording."""

    success: bool
    message: str
    new_state: str | None = None


# ============================================================================
# Shell Execution Models
# ============================================================================


class ShellRunRequest(BaseModel):
    """Request to run a shell command in workspace."""

    workspace_id: str = Field(..., description="Workspace identifier")
    command: str = Field(..., description="Shell command to execute", min_length=1)
    timeout: int = Field(
        60,
        description="Timeout in seconds (default: 60, max: 300)",
        ge=1,
        le=300,
    )


class ShellRunResponse(BaseModel):
    """Response from shell command execution."""

    success: bool = Field(
        ..., description="Whether command executed successfully (exit_code == 0)"
    )
    stdout: str = Field(..., description="Standard output from command")
    stderr: str = Field(..., description="Standard error from command")
    exit_code: int = Field(..., description="Command exit code")
    timed_out: bool = Field(False, description="Whether command timed out")
    duration_seconds: float = Field(..., description="Execution time in seconds")


class ShellValidationRequest(BaseModel):
    """Request to run workspace validations."""

    workspace_id: str = Field(..., description="Workspace identifier")
    names: list[str] = Field(
        default_factory=list,
        description="Validation names to run (empty = all)",
    )
    fail_fast: bool = Field(True, description="Stop on first failure")
    timeout: int = Field(
        300,
        description="Timeout per validation in seconds (default: 300)",
        ge=1,
        le=300,
    )


class ShellValidationResponse(BaseModel):
    """Response from workspace validation execution."""

    results: list[ValidationResult] = Field(..., description="Validation results")
    all_passed: bool = Field(..., description="Whether all validations passed")
    duration_seconds: float = Field(..., description="Total execution time in seconds")


# ============================================================================
# Error Models
# ============================================================================



# ============================================================================
# Workspace Models
# ============================================================================


class WorkspaceCreateRequest(BaseModel):
    """Request to create a new workspace."""

    git_url: str = Field(..., description="Git repository URL to clone")
    branch: str = Field("main", description="Branch to checkout")


class WorkspaceResponse(BaseModel):
    """Response containing workspace details."""

    id: str = Field(..., description="Unique workspace identifier")
    user_id: str = Field(..., description="Owner user ID")
    git_url: str = Field(..., description="Git repository URL")
    branch: str = Field(..., description="Git branch name")
    status: str = Field(..., description="Workspace status (creating, ready, running, stopped, error)")
    created_at: datetime = Field(..., description="When the workspace was created")
    container_id: str | None = Field(None, description="Container/process ID")
    error_message: str | None = Field(None, description="Error details if status is error")


class WorkspaceListResponse(BaseModel):
    """Response containing list of workspaces."""

    workspaces: list[WorkspaceResponse] = Field(..., description="List of workspaces")
    total: int = Field(..., description="Total count of workspaces")


class WorkspaceStatusResponse(BaseModel):
    """Response containing workspace status and resource usage."""

    id: str = Field(..., description="Workspace identifier")
    status: str = Field(..., description="Workspace status")
    cpu_percent: float | None = Field(None, description="CPU usage percentage")
    memory_mb: float | None = Field(None, description="Memory usage in MB")
    disk_mb: float | None = Field(None, description="Disk usage in MB")
    is_healthy: bool = Field(..., description="Whether workspace is healthy")


class WorkspaceExecRequest(BaseModel):
    """Request to execute a command in workspace."""

    command: str = Field(..., description="Command to execute", min_length=1)
    timeout: int = Field(
        60,
        ge=1,
        le=300,
        description="Timeout in seconds (default: 60, max: 300)",
    )


class WorkspaceExecResponse(BaseModel):
    """Response from command execution."""

    exit_code: int = Field(..., description="Process exit code")
    stdout: str = Field(..., description="Standard output")
    stderr: str = Field(..., description="Standard error")
    timed_out: bool = Field(..., description="Whether execution timed out")
    duration_seconds: float = Field(..., description="Execution time in seconds")


class ErrorResponse(BaseModel):
    """Standard error response."""

    error: str = Field(..., description="Error type")
    message: str = Field(..., description="Error message")
    details: dict[str, Any] = Field(default_factory=dict, description="Additional details")
