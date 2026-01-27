"""DNS verification utilities.

Provides DNS TXT record verification for domain ownership validation.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    pass

logger = logging.getLogger(__name__)


async def verify_dns_txt_record(
    domain: str,
    expected_token: str,
    *,
    record_prefix: str = "_c4-verify",
) -> bool:
    """Verify domain ownership via DNS TXT record.

    Checks if the expected verification token exists in the DNS TXT records
    for the specified domain. The TXT record should be set at:
    {record_prefix}.{domain}

    For example, to verify example.com with token "abc123":
    - DNS TXT record: _c4-verify.example.com
    - Expected value: "c4-domain-verification=abc123"

    Args:
        domain: The domain to verify (e.g., "example.com")
        expected_token: The verification token to look for
        record_prefix: DNS record prefix (default: "_c4-verify")

    Returns:
        True if the verification token is found in DNS TXT records
    """
    import asyncio

    import dns.resolver

    record_name = f"{record_prefix}.{domain}"
    expected_value = f"c4-domain-verification={expected_token}"

    try:
        # Run DNS query in thread pool to avoid blocking
        loop = asyncio.get_event_loop()
        answers = await loop.run_in_executor(
            None,
            lambda: dns.resolver.resolve(record_name, "TXT"),
        )

        # Check each TXT record for the expected token
        for rdata in answers:
            # TXT records may have multiple strings, join them
            txt_value = "".join(s.decode("utf-8") for s in rdata.strings)

            if expected_value in txt_value or expected_token in txt_value:
                logger.info(f"DNS TXT verification successful for {domain}")
                return True

        logger.warning(
            f"DNS TXT record found for {record_name} but token not matched. "
            f"Expected: {expected_value}"
        )
        return False

    except dns.resolver.NXDOMAIN:
        logger.warning(f"DNS TXT record not found: {record_name} (NXDOMAIN)")
        return False
    except dns.resolver.NoAnswer:
        logger.warning(f"DNS TXT record not found: {record_name} (NoAnswer)")
        return False
    except dns.resolver.Timeout:
        logger.error(f"DNS query timeout for {record_name}")
        return False
    except dns.exception.DNSException as e:
        logger.error(f"DNS query failed for {record_name}: {e}")
        return False
    except Exception as e:
        logger.error(f"Unexpected error during DNS verification: {e}")
        return False


def get_verification_instructions(
    domain: str,
    token: str,
    *,
    record_prefix: str = "_c4-verify",
) -> dict:
    """Get DNS verification instructions for users.

    Args:
        domain: The domain to verify
        token: The verification token
        record_prefix: DNS record prefix

    Returns:
        Dict with verification instructions
    """
    record_name = f"{record_prefix}.{domain}"
    record_value = f"c4-domain-verification={token}"

    return {
        "record_type": "TXT",
        "record_name": record_name,
        "record_value": record_value,
        "instructions": (
            f"Add a TXT record to your DNS settings:\n"
            f"  Name/Host: {record_prefix}\n"
            f"  Type: TXT\n"
            f"  Value: {record_value}\n\n"
            f"DNS changes may take up to 48 hours to propagate."
        ),
    }
