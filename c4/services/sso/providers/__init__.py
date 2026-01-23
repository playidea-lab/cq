"""SSO Providers.

Implementations of SSO providers for different identity providers.
"""

from c4.services.sso.providers.google import GoogleOIDCProvider
from c4.services.sso.providers.microsoft import MicrosoftOIDCProvider

__all__ = [
    "GoogleOIDCProvider",
    "MicrosoftOIDCProvider",
]
