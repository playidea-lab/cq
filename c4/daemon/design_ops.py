"""Design operations for C4 Daemon.

This module contains design phase operations extracted from C4Daemon:
- c4_save_design: Save design specification for a feature
- c4_get_design: Get design specification for a feature
- c4_list_designs: List all features with design specifications
- c4_design_complete: Complete design phase, transition to PLAN

These operations are delegated from C4Daemon for modularity.
"""

from typing import TYPE_CHECKING, Any

from ..models import ProjectStatus
from ..state_machine import StateTransitionError

if TYPE_CHECKING:
    from .c4_daemon import C4Daemon


class DesignOps:
    """Design operations handler for C4 Daemon.

    Provides design phase operations including design spec management
    and phase transitions.
    """

    def __init__(self, daemon: "C4Daemon"):
        """Initialize DesignOps with parent daemon reference.

        Args:
            daemon: Parent C4Daemon instance for state and config access
        """
        self._daemon = daemon

    # =========================================================================
    # Design Spec Management
    # =========================================================================

    def save_design(
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
        """Save design specification for a feature.

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
            description: Optional description

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
            yaml_path, md_path = self._daemon.design_store.save(spec)

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

    def get_design(self, feature: str) -> dict[str, Any]:
        """Get design specification for a feature.

        Args:
            feature: Feature name to retrieve

        Returns:
            Dictionary with design details or error
        """
        try:
            spec = self._daemon.design_store.load(feature)

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

    def list_designs(self) -> dict[str, Any]:
        """List all features with design specifications.

        Returns:
            Dictionary with list of features
        """
        try:
            features = self._daemon.design_store.list_features_with_design()
            designs = []

            for feature_name in features:
                spec = self._daemon.design_store.load(feature_name)
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

    # =========================================================================
    # Phase Transitions
    # =========================================================================

    def design_complete(self) -> dict[str, Any]:
        """Mark design phase as complete, transition to PLAN.

        Returns:
            Dictionary with transition result
        """
        if self._daemon.state_machine is None:
            return {"success": False, "error": "C4 not initialized"}

        state = self._daemon.state_machine.state
        current_status = state.status.value

        # Verify we're in DESIGN state
        if state.status != ProjectStatus.DESIGN:
            return {
                "success": False,
                "error": f"Not in DESIGN state (current: {current_status})",
                "hint": "c4_design_complete can only be called from DESIGN state",
            }

        # Check if any designs have been created
        designs = self._daemon.design_store.list_features_with_design()
        if not designs:
            return {
                "success": False,
                "error": "No design specifications found",
                "hint": "Create at least one design with c4_save_design before completing design phase",
            }

        # Check if designs have selected options
        incomplete_designs = []
        for feature_name in designs:
            spec = self._daemon.design_store.load(feature_name)
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
            self._daemon.state_machine.transition("design_approved")

            return {
                "success": True,
                "previous_status": current_status,
                "new_status": self._daemon.state_machine.state.status.value,
                "designs_count": len(designs),
                "message": "Design phase complete. Ready for planning.",
            }

        except StateTransitionError as e:
            return {"success": False, "error": str(e)}
