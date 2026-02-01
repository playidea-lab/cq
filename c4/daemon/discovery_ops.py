"""Discovery operations for C4 Daemon.

This module contains discovery phase operations extracted from C4Daemon:
- c4_save_spec: Save feature specification
- c4_list_specs: List all feature specifications
- c4_get_spec: Get a specific feature specification
- c4_add_verification: Add verification requirement to a feature
- c4_get_feature_verifications: Get verifications for a feature
- c4_discovery_complete: Complete discovery phase, transition to DESIGN

These operations are delegated from C4Daemon for modularity.
"""

from typing import TYPE_CHECKING, Any

from ..discovery import Domain, EARSPattern, EARSRequirement, FeatureSpec
from ..state_machine import StateTransitionError

if TYPE_CHECKING:
    from ..discovery import VerificationRequirement
    from .c4_daemon import C4Daemon


class DiscoveryOps:
    """Discovery operations handler for C4 Daemon.

    Provides discovery phase operations including spec management,
    verification requirements, and phase transitions.
    """

    def __init__(self, daemon: "C4Daemon"):
        """Initialize DiscoveryOps with parent daemon reference.

        Args:
            daemon: Parent C4Daemon instance for state and config access
        """
        self._daemon = daemon

    # =========================================================================
    # Verification Helpers
    # =========================================================================

    def _sync_verification_to_config(self, verification: "VerificationRequirement") -> None:
        """Sync a verification requirement to config.yaml.

        This ensures verifications collected during discovery are available
        for runtime verification during checkpoint review.
        """
        from c4.models.config import VerificationItem

        # Check if already exists (by name)
        existing_names = {item.name for item in self._daemon.config.verifications.items}
        if verification.name in existing_names:
            return  # Already synced

        # Add to config
        item = VerificationItem(
            type=verification.type,
            name=verification.name,
            config=verification.config,
            enabled=verification.enabled,
        )
        self._daemon.config.verifications.items.append(item)

        # Enable verifications if not already
        if not self._daemon.config.verifications.enabled:
            self._daemon.config.verifications.enabled = True

        self._daemon._save_config()

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
        existing_names = {item.name for item in self._daemon.config.verifications.items}

        for default in defaults:
            if default["name"] in existing_names:
                continue

            item = VerificationItem(
                type=default["type"],
                name=default["name"],
                config=default.get("config", {}),
                enabled=True,
            )
            self._daemon.config.verifications.items.append(item)
            added.append(default["name"])

        if added:
            if not self._daemon.config.verifications.enabled:
                self._daemon.config.verifications.enabled = True
            self._daemon._save_config()

        return added

    # =========================================================================
    # Spec Management
    # =========================================================================

    def save_spec(
        self,
        feature: str,
        requirements: list[dict],
        domain: str,
        description: str | None = None,
    ) -> dict[str, Any]:
        """Save feature specification to .c4/specs/.

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
            spec_file = self._daemon.spec_store.save(spec)

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

    def list_specs(self) -> dict[str, Any]:
        """List all feature specifications.

        Returns:
            Dictionary with feature list
        """
        try:
            features = self._daemon.spec_store.list_features()

            # Get summary for each feature
            specs_summary = []
            for feature in features:
                spec = self._daemon.spec_store.load(feature)
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

    def get_spec(self, feature: str) -> dict[str, Any]:
        """Get a specific feature specification.

        Args:
            feature: Feature name

        Returns:
            Dictionary with spec details
        """
        try:
            spec = self._daemon.spec_store.load(feature)
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

    # =========================================================================
    # Verification Management
    # =========================================================================

    def add_verification(
        self,
        feature: str,
        verification_type: str,
        name: str,
        reason: str,
        priority: int = 2,
        config: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        """Add a verification requirement to a feature spec from conversation context.

        Use this when the user requests specific verification or when conversation
        context suggests verification needs (e.g., "performance test needed").

        Args:
            feature: Feature name (must exist)
            verification_type: One of: http, browser, cli, metrics, visual, dryrun
            name: Human-readable name for the verification
            reason: Why this verification is needed (from conversation context)
            priority: 1=critical, 2=normal, 3=optional (default: 2)
            config: Verification-specific configuration

        Returns:
            Dictionary with success status and verification details
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
        spec = self._daemon.spec_store.load(feature)
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
            self._daemon.spec_store.save(spec)

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

    def get_feature_verifications(self, feature: str) -> dict[str, Any]:
        """Get all verification requirements for a feature.

        Args:
            feature: Feature name

        Returns:
            Dictionary with verification requirements
        """
        spec = self._daemon.spec_store.load(feature)
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

    # =========================================================================
    # Phase Transitions
    # =========================================================================

    def discovery_complete(self) -> dict[str, Any]:
        """Mark discovery phase as complete, transition to DESIGN.

        Returns:
            Dictionary with transition result
        """
        from ..models import ProjectStatus

        if self._daemon.state_machine is None:
            return {"success": False, "error": "C4 not initialized"}

        state = self._daemon.state_machine.state
        current_status = state.status.value

        # Verify we're in DISCOVERY state
        if state.status != ProjectStatus.DISCOVERY:
            return {
                "success": False,
                "error": f"Not in DISCOVERY state (current: {current_status})",
                "hint": "c4_discovery_complete can only be called from DISCOVERY state",
            }

        # Check if any specs have been created
        specs = self._daemon.spec_store.list_features()
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
                spec = self._daemon.spec_store.load(spec_name)
                if spec and spec.domain:
                    domain_value = spec.domain.value if hasattr(spec.domain, "value") else str(spec.domain)
                    domains_found.add(domain_value)

            # Apply domain defaults for each unique domain
            for domain in domains_found:
                added = self._apply_domain_default_verifications(domain)
                default_verifications_added.extend(added)

            # Also set domain in config if single domain project
            if len(domains_found) == 1:
                self._daemon._config.domain = list(domains_found)[0]
                self._daemon._save_config()

            # Transition to DESIGN
            self._daemon.state_machine.transition("discovery_complete")

            result = {
                "success": True,
                "previous_status": current_status,
                "new_status": self._daemon.state_machine.state.status.value,
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
