"""SSO Service Module.

Provides SSO/SAML authentication for Enterprise users.
Supports OIDC (Google, Microsoft) and SAML (Okta, custom) providers.
"""

from c4.services.sso.models import (
    SSOConfig,
    SSOProvider,
    SSOSession,
    SSOUserInfo,
)
from c4.services.sso.service import SSOService, create_sso_service

__all__ = [
    "SSOConfig",
    "SSOProvider",
    "SSOService",
    "SSOSession",
    "SSOUserInfo",
    "create_sso_service",
]
