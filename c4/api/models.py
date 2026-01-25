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

    state: str = Field(..., description="Current C4 state (INIT, DISCOVERY, DESIGN, etc.)")
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


# ============================================================================
# File Operations Models
# ============================================================================


class FileReadRequest(BaseModel):
    """Request to read a file from workspace."""

    workspace_id: str = Field(..., description="Workspace identifier")
    path: str = Field(..., description="Relative path to file within workspace")


class FileReadResponse(BaseModel):
    """Response containing file content."""

    success: bool = Field(..., description="Whether read was successful")
    path: str = Field(..., description="File path that was read")
    content: str | None = Field(None, description="File content (utf-8)")
    size: int | None = Field(None, description="File size in bytes")
    error: str | None = Field(None, description="Error message if failed")


class FileWriteRequest(BaseModel):
    """Request to write a file to workspace."""

    workspace_id: str = Field(..., description="Workspace identifier")
    path: str = Field(..., description="Relative path to file within workspace")
    content: str = Field(..., description="File content to write (utf-8)")
    create_dirs: bool = Field(True, description="Create parent directories if needed")


class FileWriteResponse(BaseModel):
    """Response from file write operation."""

    success: bool = Field(..., description="Whether write was successful")
    path: str = Field(..., description="File path that was written")
    size: int | None = Field(None, description="Written file size in bytes")
    error: str | None = Field(None, description="Error message if failed")


class DirectoryListRequest(BaseModel):
    """Request to list directory contents."""

    workspace_id: str = Field(..., description="Workspace identifier")
    path: str = Field(".", description="Relative path to directory (default: root)")
    recursive: bool = Field(False, description="List recursively")
    include_hidden: bool = Field(False, description="Include hidden files (starting with .)")


class FileInfo(BaseModel):
    """Information about a file or directory."""

    name: str = Field(..., description="File/directory name")
    path: str = Field(..., description="Relative path from workspace root")
    is_dir: bool = Field(..., description="Whether this is a directory")
    size: int | None = Field(None, description="File size in bytes (None for directories)")


class DirectoryListResponse(BaseModel):
    """Response containing directory listing."""

    success: bool = Field(..., description="Whether listing was successful")
    path: str = Field(..., description="Directory path that was listed")
    entries: list[FileInfo] = Field(default_factory=list, description="Directory entries")
    error: str | None = Field(None, description="Error message if failed")


class FileSearchRequest(BaseModel):
    """Request to search files using glob or grep."""

    workspace_id: str = Field(..., description="Workspace identifier")
    pattern: str = Field(..., description="Search pattern (glob for files, regex for content)")
    path: str = Field(".", description="Starting directory for search")
    search_type: str = Field("glob", description="Search type: 'glob' or 'grep'")
    max_results: int = Field(100, description="Maximum results to return")


class SearchMatch(BaseModel):
    """A search result match."""

    path: str = Field(..., description="File path")
    line_number: int | None = Field(None, description="Line number (for grep)")
    line_content: str | None = Field(None, description="Matching line content (for grep)")


class FileSearchResponse(BaseModel):
    """Response containing search results."""

    success: bool = Field(..., description="Whether search was successful")
    pattern: str = Field(..., description="Search pattern used")
    search_type: str = Field(..., description="Search type used")
    matches: list[SearchMatch] = Field(default_factory=list, description="Search matches")
    truncated: bool = Field(False, description="Whether results were truncated")
    error: str | None = Field(None, description="Error message if failed")


class FileDeleteRequest(BaseModel):
    """Request to delete a file."""

    workspace_id: str = Field(..., description="Workspace identifier")
    path: str = Field(..., description="Relative path to file within workspace")


class FileDeleteResponse(BaseModel):
    """Response from file delete operation."""

    success: bool = Field(..., description="Whether delete was successful")
    path: str = Field(..., description="File path that was deleted")
    error: str | None = Field(None, description="Error message if failed")


# ============================================================================
# Team Models
# ============================================================================


class TeamRole(str, Enum):
    """Team member roles for RBAC."""

    OWNER = "owner"
    ADMIN = "admin"
    MEMBER = "member"
    VIEWER = "viewer"


class TeamPlan(str, Enum):
    """Team subscription plans."""

    FREE = "free"
    PRO = "pro"
    TEAM = "team"
    AGENCY = "agency"
    ENTERPRISE = "enterprise"


class InviteStatus(str, Enum):
    """Team invitation status."""

    PENDING = "pending"
    ACCEPTED = "accepted"
    EXPIRED = "expired"


class TeamCreateRequest(BaseModel):
    """Request to create a new team."""

    name: str = Field(..., description="Team display name", min_length=1, max_length=100)
    slug: str | None = Field(
        None,
        description="URL-friendly identifier (auto-generated if not provided)",
        pattern="^[a-z0-9-]+$",
        min_length=1,
        max_length=50,
    )
    settings: dict[str, Any] | None = Field(None, description="Initial team settings")


class TeamUpdateRequest(BaseModel):
    """Request to update team settings."""

    name: str | None = Field(None, description="New team name", min_length=1, max_length=100)
    settings: dict[str, Any] | None = Field(None, description="Team settings to update")


class TeamResponse(BaseModel):
    """Response containing team details."""

    id: str = Field(..., description="Unique team identifier")
    name: str = Field(..., description="Team display name")
    slug: str = Field(..., description="URL-friendly identifier")
    owner_id: str = Field(..., description="Team owner's user ID")
    plan: TeamPlan = Field(TeamPlan.FREE, description="Team subscription plan")
    settings: dict[str, Any] = Field(default_factory=dict, description="Team settings")
    created_at: datetime = Field(..., description="When the team was created")
    updated_at: datetime = Field(..., description="When the team was last updated")


class TeamListResponse(BaseModel):
    """Response containing list of teams."""

    teams: list[TeamResponse] = Field(..., description="List of teams")
    total: int = Field(..., description="Total count of teams")


class TeamMemberResponse(BaseModel):
    """Response containing team member details."""

    id: str = Field(..., description="Unique member record ID")
    team_id: str = Field(..., description="Team ID")
    user_id: str = Field(..., description="User ID")
    email: str | None = Field(None, description="Member's email")
    role: TeamRole = Field(..., description="Member's role in the team")
    joined_at: datetime = Field(..., description="When the member joined")


class TeamMemberListResponse(BaseModel):
    """Response containing list of team members."""

    members: list[TeamMemberResponse] = Field(..., description="List of team members")
    total: int = Field(..., description="Total count of members")


class TeamInviteRequest(BaseModel):
    """Request to invite a member to a team."""

    email: str = Field(..., description="Email of the person to invite")
    role: TeamRole = Field(TeamRole.MEMBER, description="Role to assign to the invitee")


class TeamInviteResponse(BaseModel):
    """Response from team invitation."""

    id: str = Field(..., description="Invite ID")
    team_id: str = Field(..., description="Team ID")
    email: str = Field(..., description="Invited email")
    role: TeamRole = Field(..., description="Assigned role")
    status: InviteStatus = Field(..., description="Invite status")
    invited_by: str = Field(..., description="User who sent the invite")
    invited_at: datetime = Field(..., description="When the invite was sent")
    expires_at: datetime | None = Field(None, description="When the invite expires")


class TeamMemberUpdateRequest(BaseModel):
    """Request to update a team member's role."""

    role: TeamRole = Field(..., description="New role for the member")


# ============================================================================
# Integration Models
# ============================================================================


class IntegrationProviderResponse(BaseModel):
    """Response containing integration provider information."""

    id: str = Field(..., description="Provider ID (e.g., 'github', 'discord')")
    name: str = Field(..., description="Provider display name")
    category: str = Field(..., description="Provider category")
    capabilities: list[str] = Field(default_factory=list, description="Provider capabilities")
    description: str | None = Field(None, description="Provider description")
    icon_url: str | None = Field(None, description="Provider icon URL")
    docs_url: str | None = Field(None, description="Documentation URL")


class ProvidersListResponse(BaseModel):
    """Response containing list of integration providers."""

    providers: list[IntegrationProviderResponse] = Field(..., description="Available providers")
    total: int = Field(..., description="Total count of providers")


class OAuthUrlResponse(BaseModel):
    """Response containing OAuth URL for integration connection."""

    url: str = Field(..., description="OAuth authorization URL")
    state: str = Field(..., description="State parameter for CSRF protection")


class OAuthCallbackRequest(BaseModel):
    """Request parameters from OAuth callback."""

    code: str = Field(..., description="Authorization code")
    state: str = Field(..., description="State parameter")


class IntegrationResponse(BaseModel):
    """Response containing integration details."""

    id: str = Field(..., description="Integration ID")
    team_id: str = Field(..., description="Team ID")
    provider_id: str = Field(..., description="Provider ID")
    external_id: str = Field(..., description="External service ID")
    external_name: str | None = Field(None, description="External service name")
    status: str = Field("active", description="Integration status")
    settings: dict[str, Any] = Field(default_factory=dict, description="Integration settings")
    connected_by: str | None = Field(None, description="User who connected")
    connected_at: datetime | None = Field(None, description="When connected")
    last_used_at: datetime | None = Field(None, description="Last activity time")


class IntegrationsListResponse(BaseModel):
    """Response containing list of integrations."""

    integrations: list[IntegrationResponse] = Field(..., description="List of integrations")
    total: int = Field(..., description="Total count of integrations")


class IntegrationSettingsUpdate(BaseModel):
    """Request to update integration settings."""

    settings: dict[str, Any] = Field(..., description="New settings to apply")


class IntegrationConnectResponse(BaseModel):
    """Response from integration connection."""

    success: bool = Field(..., description="Whether connection was successful")
    integration_id: str | None = Field(None, description="New integration ID")
    message: str = Field(..., description="Status message")


# ============================================================================
# Branding Models
# ============================================================================


class BrandingResponse(BaseModel):
    """Response containing team branding details."""

    id: str = Field(..., description="Branding record ID")
    team_id: str = Field(..., description="Team ID")

    # Basic Branding
    logo_url: str | None = Field(None, description="Main logo URL (for light backgrounds)")
    logo_dark_url: str | None = Field(None, description="Logo URL for dark backgrounds")
    favicon_url: str | None = Field(None, description="Favicon/browser icon URL")
    brand_name: str | None = Field(None, description="Display name (overrides team name)")

    # Color Scheme
    primary_color: str = Field("#2563EB", description="Main brand color")
    secondary_color: str = Field("#64748B", description="Secondary color")
    accent_color: str = Field("#F59E0B", description="Accent/highlight color")
    background_color: str = Field("#FFFFFF", description="Background color")
    text_color: str = Field("#1F2937", description="Primary text color")

    # Typography
    heading_font: str | None = Field(None, description="Font for headings (Google Fonts name)")
    body_font: str | None = Field(None, description="Font for body text")
    font_scale: float = Field(1.0, description="Font size multiplier")

    # Custom Domain
    custom_domain: str | None = Field(None, description="Custom domain (e.g., 'projects.agency.com')")
    custom_domain_verified: bool = Field(False, description="Whether custom domain is verified")

    # Email Branding
    email_from_name: str | None = Field(None, description="'From' name in emails")
    email_footer_text: str | None = Field(None, description="Custom email footer")

    # Advanced
    meta_description: str | None = Field(None, description="SEO meta description")
    social_preview_image_url: str | None = Field(None, description="OG image for link previews")
    hide_powered_by: bool = Field(False, description="Hide 'Powered by C4' (Enterprise)")
    custom_login_background_url: str | None = Field(None, description="Custom login page background")

    # Metadata
    created_at: datetime | None = Field(None, description="When branding was created")
    updated_at: datetime | None = Field(None, description="When branding was last updated")


class BrandingUpdateRequest(BaseModel):
    """Request to update team branding."""

    # Basic Branding
    logo_url: str | None = Field(None, description="Main logo URL")
    logo_dark_url: str | None = Field(None, description="Logo URL for dark backgrounds")
    favicon_url: str | None = Field(None, description="Favicon URL")
    brand_name: str | None = Field(None, description="Display name", max_length=100)

    # Color Scheme
    primary_color: str | None = Field(None, description="Main brand color", pattern="^#[0-9A-Fa-f]{6}$")
    secondary_color: str | None = Field(None, description="Secondary color", pattern="^#[0-9A-Fa-f]{6}$")
    accent_color: str | None = Field(None, description="Accent color", pattern="^#[0-9A-Fa-f]{6}$")
    background_color: str | None = Field(None, description="Background color", pattern="^#[0-9A-Fa-f]{6}$")
    text_color: str | None = Field(None, description="Text color", pattern="^#[0-9A-Fa-f]{6}$")

    # Typography
    heading_font: str | None = Field(None, description="Heading font name", max_length=100)
    body_font: str | None = Field(None, description="Body font name", max_length=100)
    font_scale: float | None = Field(None, description="Font scale", ge=0.5, le=2.0)

    # Email Branding
    email_from_name: str | None = Field(None, description="Email 'From' name", max_length=100)
    email_footer_text: str | None = Field(None, description="Email footer text", max_length=500)

    # Advanced
    meta_description: str | None = Field(None, description="SEO meta description", max_length=300)
    social_preview_image_url: str | None = Field(None, description="Social preview image URL")
    custom_login_background_url: str | None = Field(None, description="Login background URL")


class DomainVerificationRequest(BaseModel):
    """Request to initiate custom domain verification."""

    domain: str = Field(
        ...,
        description="Custom domain to verify (e.g., 'projects.agency.com')",
        pattern=r"^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$",
    )


class DomainVerificationResponse(BaseModel):
    """Response from domain verification initiation."""

    success: bool = Field(..., description="Whether initiation was successful")
    verification_token: str | None = Field(None, description="Token for DNS TXT record")
    instructions: dict[str, str] | None = Field(None, description="DNS setup instructions")
    error: str | None = Field(None, description="Error message if failed")


class DomainVerifyResponse(BaseModel):
    """Response from domain verification check."""

    success: bool = Field(..., description="Whether domain is now verified")
    domain: str | None = Field(None, description="Verified domain")
    verified_at: datetime | None = Field(None, description="When domain was verified")
    error: str | None = Field(None, description="Error message if verification failed")


class PublicBrandingResponse(BaseModel):
    """Public branding info accessible without authentication (for login pages)."""

    brand_name: str | None = Field(None, description="Display name")
    logo_url: str | None = Field(None, description="Logo URL")
    logo_dark_url: str | None = Field(None, description="Logo for dark backgrounds")
    favicon_url: str | None = Field(None, description="Favicon URL")
    primary_color: str = Field("#2563EB", description="Primary color")
    background_color: str = Field("#FFFFFF", description="Background color")
    text_color: str = Field("#1F2937", description="Text color")
    heading_font: str | None = Field(None, description="Heading font")
    body_font: str | None = Field(None, description="Body font")
    custom_login_background_url: str | None = Field(None, description="Login background")
