"""C4 API Routes.

Provides modular route handlers for different C4 phases:
- c4: Core orchestration (status, tasks, submit)
- discovery: Discovery phase (specs, requirements)
- design: Design phase (architecture, decisions)
- validation: Validation execution
- git: Git operations
- files: File operations (read, write, list, search, delete)
- shell: Shell command execution
- workspace: Workspace management
- webhooks: External webhook handlers (GitHub App)
- integrations: External integrations (GitHub App, Discord)
- teams: Team management and member invitations
- reports: Usage reports and audit logs
- sso: SSO/SAML authentication (Enterprise)
"""

from . import (
    c4,
    design,
    discovery,
    files,
    git,
    integrations,
    reports,
    shell,
    sso,
    teams,
    validation,
    webhooks,
    workspace,
)

__all__ = [
    "c4",
    "discovery",
    "design",
    "validation",
    "git",
    "files",
    "integrations",
    "teams",
    "reports",
    "shell",
    "sso",
    "webhooks",
    "workspace",
]
