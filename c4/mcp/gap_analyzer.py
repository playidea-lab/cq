"""MCP Gap Analyzer Tools.

Provides MCP tools for analyzing gaps between EARS specifications and implementations:
- analyze_spec_gaps: Analyze implementation coverage of EARS requirements
- generate_tests_from_spec: Generate test stubs from EARS patterns
- link_impl_to_spec: Map code symbols to requirements
- verify_spec_completion: Verify all requirements are implemented and tested
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from datetime import datetime, timezone
from enum import Enum
from pathlib import Path
from typing import Any

from c4.docs.gap import GapAnalysisResult, GapAnalyzer, ImplementationStatus
from c4.docs.testgen import TestFormat, TestGenerator

logger = logging.getLogger(__name__)


class GapFormat(Enum):
    """Output format for gap analysis."""

    JSON = "json"
    MARKDOWN = "markdown"


@dataclass
class SpecLink:
    """Link between a code symbol and a requirement."""

    requirement_id: str
    requirement_text: str
    pattern: str
    symbol_name: str
    file_path: str
    line_number: int | None = None
    confidence: float = 0.0  # 0-1 confidence score
    match_reason: str = ""


@dataclass
class SpecLinkResult:
    """Result of linking implementations to specifications."""

    spec_id: str
    feature: str
    total_requirements: int
    linked_count: int
    unlinked_count: int
    links: list[SpecLink]
    unlinked_requirements: list[str]
    analyzed_at: str


@dataclass
class CompletionStatus:
    """Completion status for a specification."""

    spec_id: str
    feature: str
    is_complete: bool
    implementation_coverage: float
    test_coverage: float
    total_requirements: int
    implemented_count: int
    tested_count: int
    missing_implementations: list[str]
    missing_tests: list[str]
    verified_at: str


class MCPGapAnalyzer:
    """MCP-compatible Gap Analyzer.

    Wraps GapAnalyzer and TestGenerator for MCP tool integration.

    Example:
        analyzer = MCPGapAnalyzer(project_root="/path/to/project")

        # Analyze gaps
        result = analyzer.analyze_spec_gaps("user-auth")

        # Generate tests
        tests = analyzer.generate_tests_from_spec("user-auth")

        # Link implementations
        links = analyzer.link_impl_to_spec("user-auth")

        # Verify completion
        status = analyzer.verify_spec_completion("user-auth")
    """

    def __init__(
        self,
        project_root: str | Path | None = None,
        specs_dir: str | Path | None = None,
        source_dir: str | Path | None = None,
        tests_dir: str | Path | None = None,
    ) -> None:
        """Initialize the MCP Gap Analyzer.

        Args:
            project_root: Project root directory
            specs_dir: Directory containing spec files (default: .c4/specs)
            source_dir: Directory containing source code (default: src or c4)
            tests_dir: Directory containing test files (default: tests)
        """
        self.project_root = Path(project_root) if project_root else Path.cwd()

        # Resolve directories
        if specs_dir:
            self.specs_dir = Path(specs_dir)
        else:
            self.specs_dir = self.project_root / ".c4" / "specs"

        if source_dir:
            self.source_dir = Path(source_dir)
        else:
            # Try common source directories
            for candidate in ["src", "c4", "lib", "app"]:
                candidate_path = self.project_root / candidate
                if candidate_path.exists():
                    self.source_dir = candidate_path
                    break
            else:
                self.source_dir = self.project_root / "src"

        if tests_dir:
            self.tests_dir = Path(tests_dir)
        else:
            self.tests_dir = self.project_root / "tests"

        # Initialize analyzers
        self._gap_analyzer = GapAnalyzer(
            specs_dir=self.specs_dir,
            source_dir=self.source_dir,
            tests_dir=self.tests_dir,
        )
        self._test_generator = TestGenerator(
            specs_dir=self.specs_dir,
            tests_dir=self.tests_dir,
        )

    def list_specs(self) -> list[dict[str, Any]]:
        """List all available specifications.

        Returns:
            List of spec summaries with id, feature, domain, requirements_count
        """
        return self._gap_analyzer.list_specs()

    def analyze_spec_gaps(
        self,
        spec_id: str,
        language: str = "python",
        format: GapFormat = GapFormat.JSON,  # noqa: A002
    ) -> dict[str, Any] | str:
        """Analyze gaps between EARS specifications and implementation.

        Args:
            spec_id: Specification identifier (directory name in .c4/specs/)
            language: Primary language ("python" or "typescript")
            format: Output format (json or markdown)

        Returns:
            Gap analysis result as dict or markdown string
        """
        try:
            result = self._gap_analyzer.analyze_gaps(spec_id, language)

            if format == GapFormat.MARKDOWN:
                return self._gap_analyzer.generate_report(result, format="markdown")

            return self._gap_result_to_dict(result)
        except FileNotFoundError as e:
            return {"error": str(e)}
        except Exception as e:
            logger.error(f"Failed to analyze gaps for {spec_id}: {e}")
            return {"error": f"Gap analysis failed: {e}"}

    def _gap_result_to_dict(self, result: GapAnalysisResult) -> dict[str, Any]:
        """Convert GapAnalysisResult to dictionary."""
        return {
            "spec_id": result.spec_id,
            "feature": result.feature,
            "domain": result.domain,
            "analyzed_at": result.analyzed_at,
            "summary": {
                "total_requirements": result.total_requirements,
                "testable_requirements": result.testable_requirements,
                "implementation_coverage": result.implementation_coverage,
                "test_coverage": result.test_coverage,
            },
            "counts": {
                "implemented": result.implemented_count,
                "partially_implemented": result.partially_implemented_count,
                "not_implemented": result.not_implemented_count,
                "tested": result.tested_count,
            },
            "files_analyzed": {
                "source_files": result.source_files_analyzed,
                "test_files": result.test_files_analyzed,
            },
            "gaps": [
                {
                    "requirement_id": gap.requirement.id,
                    "requirement_text": gap.requirement.text,
                    "pattern": gap.requirement.pattern.value,
                    "status": gap.implementation_status.value,
                    "has_test": gap.test_coverage,
                    "priority": gap.suggested_priority.value,
                    "priority_label": gap.suggested_priority.name,
                    "matched_symbols": gap.matched_symbols,
                    "matched_files": gap.matched_files,
                }
                for gap in result.gaps
            ],
        }

    def generate_tests_from_spec(
        self,
        spec_id: str,
        language: str = "python",
        test_format: str = "pytest",
        output_file: bool = False,
        check_duplicates: bool = True,
    ) -> dict[str, Any]:
        """Generate test stubs from EARS specification.

        Args:
            spec_id: Specification identifier
            language: Target language ("python" or "typescript")
            test_format: Test framework ("pytest", "vitest", or "jest")
            output_file: If True, write to file and return path
            check_duplicates: Check for existing tests to avoid duplicates

        Returns:
            Generated test stubs and metadata
        """
        try:
            # Map test format
            format_map = {
                "pytest": TestFormat.PYTEST,
                "vitest": TestFormat.VITEST,
                "jest": TestFormat.JEST,
            }
            ts_format = format_map.get(test_format, TestFormat.VITEST)

            if output_file:
                # Generate complete file
                output_path = self._test_generator.generate_test_file(
                    spec_id=spec_id,
                    language=language,
                    test_format=ts_format,
                    check_duplicates=check_duplicates,
                )
                return {
                    "success": True,
                    "output_path": str(output_path),
                    "language": language,
                    "format": test_format,
                }
            else:
                # Generate stubs only
                result = self._test_generator.generate_test_stubs(
                    spec_id=spec_id,
                    languages=[language],
                    test_format=ts_format,
                    check_duplicates=check_duplicates,
                )

                return {
                    "spec_id": result.spec_id,
                    "generated_at": result.generated_at,
                    "total_requirements": result.total_requirements,
                    "generated_count": len(result.stubs),
                    "skipped": result.skipped,
                    "stubs": [
                        {
                            "name": stub.name,
                            "requirement_id": stub.requirement_id,
                            "pattern": stub.pattern.value,
                            "language": stub.language,
                            "format": stub.format.value,
                            "code": stub.code,
                        }
                        for stub in result.stubs
                    ],
                }
        except FileNotFoundError as e:
            return {"error": str(e)}
        except Exception as e:
            logger.error(f"Failed to generate tests for {spec_id}: {e}")
            return {"error": f"Test generation failed: {e}"}

    def link_impl_to_spec(
        self,
        spec_id: str,
        language: str = "python",
        min_confidence: float = 0.3,
    ) -> dict[str, Any]:
        """Link code symbols to requirements in a specification.

        Analyzes source code to find symbols that likely implement each requirement,
        based on keyword matching and pattern analysis.

        Args:
            spec_id: Specification identifier
            language: Primary language ("python" or "typescript")
            min_confidence: Minimum confidence score to include a link (0-1)

        Returns:
            Linked implementations with confidence scores
        """
        try:
            # Get gap analysis (includes symbol matching)
            result = self._gap_analyzer.analyze_gaps(spec_id, language)

            links: list[SpecLink] = []
            unlinked: list[str] = []

            for gap in result.gaps:
                if not gap.matched_symbols and not gap.matched_files:
                    unlinked.append(gap.requirement.id)
                    continue

                # Create links for each matched symbol
                for i, symbol in enumerate(gap.matched_symbols):
                    file_path = gap.matched_files[i] if i < len(gap.matched_files) else ""

                    # Calculate confidence based on match quality
                    confidence = self._calculate_link_confidence(
                        gap.implementation_status,
                        len(gap.matched_symbols),
                        gap.test_coverage,
                    )

                    if confidence >= min_confidence:
                        link = SpecLink(
                            requirement_id=gap.requirement.id,
                            requirement_text=gap.requirement.text,
                            pattern=gap.requirement.pattern.value,
                            symbol_name=symbol,
                            file_path=file_path,
                            confidence=confidence,
                            match_reason=self._generate_match_reason(
                                gap.implementation_status, gap.test_coverage
                            ),
                        )
                        links.append(link)

                # If no symbols matched but files did, create file-level links
                if not gap.matched_symbols and gap.matched_files:
                    for file_path in gap.matched_files:
                        confidence = 0.3  # Lower confidence for file-only matches
                        if confidence >= min_confidence:
                            link = SpecLink(
                                requirement_id=gap.requirement.id,
                                requirement_text=gap.requirement.text,
                                pattern=gap.requirement.pattern.value,
                                symbol_name="(file-level)",
                                file_path=file_path,
                                confidence=confidence,
                                match_reason="File contains related keywords",
                            )
                            links.append(link)

            link_result = SpecLinkResult(
                spec_id=spec_id,
                feature=result.feature,
                total_requirements=result.testable_requirements,
                linked_count=len(set(link.requirement_id for link in links)),
                unlinked_count=len(unlinked),
                links=links,
                unlinked_requirements=unlinked,
                analyzed_at=datetime.now(timezone.utc).isoformat(),
            )

            return self._link_result_to_dict(link_result)
        except FileNotFoundError as e:
            return {"error": str(e)}
        except Exception as e:
            logger.error(f"Failed to link implementations for {spec_id}: {e}")
            return {"error": f"Implementation linking failed: {e}"}

    def _calculate_link_confidence(
        self,
        impl_status: ImplementationStatus,
        symbol_count: int,
        has_test: bool,
    ) -> float:
        """Calculate confidence score for a link.

        Args:
            impl_status: Implementation status
            symbol_count: Number of matched symbols
            has_test: Whether requirement has test coverage

        Returns:
            Confidence score 0-1
        """
        base_score = {
            ImplementationStatus.TESTED: 0.95,
            ImplementationStatus.IMPLEMENTED: 0.75,
            ImplementationStatus.PARTIALLY_IMPLEMENTED: 0.5,
            ImplementationStatus.NOT_IMPLEMENTED: 0.2,
        }.get(impl_status, 0.3)

        # Adjust for symbol count (more symbols = higher confidence)
        symbol_bonus = min(0.1 * symbol_count, 0.2)

        # Adjust for test coverage
        test_bonus = 0.1 if has_test else 0

        return min(1.0, base_score + symbol_bonus + test_bonus)

    def _generate_match_reason(
        self,
        impl_status: ImplementationStatus,
        has_test: bool,
    ) -> str:
        """Generate human-readable match reason.

        Args:
            impl_status: Implementation status
            has_test: Whether requirement has test coverage

        Returns:
            Match reason string
        """
        reasons = {
            ImplementationStatus.TESTED: "Fully implemented with tests",
            ImplementationStatus.IMPLEMENTED: "Implemented (no test found)",
            ImplementationStatus.PARTIALLY_IMPLEMENTED: "Partially implemented",
            ImplementationStatus.NOT_IMPLEMENTED: "Keywords found but not fully implemented",
        }
        reason = reasons.get(impl_status, "Unknown")
        if has_test and impl_status != ImplementationStatus.TESTED:
            reason += " + has test"
        return reason

    def _link_result_to_dict(self, result: SpecLinkResult) -> dict[str, Any]:
        """Convert SpecLinkResult to dictionary."""
        return {
            "spec_id": result.spec_id,
            "feature": result.feature,
            "analyzed_at": result.analyzed_at,
            "summary": {
                "total_requirements": result.total_requirements,
                "linked_count": result.linked_count,
                "unlinked_count": result.unlinked_count,
                "link_coverage": round(
                    result.linked_count / result.total_requirements * 100
                    if result.total_requirements > 0
                    else 0,
                    2,
                ),
            },
            "links": [
                {
                    "requirement_id": link.requirement_id,
                    "requirement_text": link.requirement_text[:100] + "..."
                    if len(link.requirement_text) > 100
                    else link.requirement_text,
                    "pattern": link.pattern,
                    "symbol_name": link.symbol_name,
                    "file_path": link.file_path,
                    "confidence": round(link.confidence, 2),
                    "match_reason": link.match_reason,
                }
                for link in result.links
            ],
            "unlinked_requirements": result.unlinked_requirements,
        }

    def verify_spec_completion(
        self,
        spec_id: str,
        language: str = "python",
        impl_threshold: float = 90.0,
        test_threshold: float = 80.0,
    ) -> dict[str, Any]:
        """Verify that all requirements in a specification are implemented and tested.

        Args:
            spec_id: Specification identifier
            language: Primary language ("python" or "typescript")
            impl_threshold: Minimum implementation coverage percentage (default: 90%)
            test_threshold: Minimum test coverage percentage (default: 80%)

        Returns:
            Completion verification result
        """
        try:
            result = self._gap_analyzer.analyze_gaps(spec_id, language)

            # Check completion status
            is_complete = (
                result.implementation_coverage >= impl_threshold
                and result.test_coverage >= test_threshold
            )

            # Collect missing implementations
            missing_impl = [
                gap.requirement.id
                for gap in result.gaps
                if gap.implementation_status
                in (ImplementationStatus.NOT_IMPLEMENTED, ImplementationStatus.PARTIALLY_IMPLEMENTED)
            ]

            # Collect missing tests
            missing_tests = [
                gap.requirement.id
                for gap in result.gaps
                if not gap.test_coverage and gap.requirement.testable
            ]

            status = CompletionStatus(
                spec_id=spec_id,
                feature=result.feature,
                is_complete=is_complete,
                implementation_coverage=result.implementation_coverage,
                test_coverage=result.test_coverage,
                total_requirements=result.testable_requirements,
                implemented_count=result.implemented_count + result.tested_count,
                tested_count=result.tested_count,
                missing_implementations=missing_impl,
                missing_tests=missing_tests,
                verified_at=datetime.now(timezone.utc).isoformat(),
            )

            return self._completion_status_to_dict(status, impl_threshold, test_threshold)
        except FileNotFoundError as e:
            return {"error": str(e)}
        except Exception as e:
            logger.error(f"Failed to verify completion for {spec_id}: {e}")
            return {"error": f"Completion verification failed: {e}"}

    def _completion_status_to_dict(
        self,
        status: CompletionStatus,
        impl_threshold: float,
        test_threshold: float,
    ) -> dict[str, Any]:
        """Convert CompletionStatus to dictionary."""
        # Generate verdict
        if status.is_complete:
            verdict = "✅ COMPLETE: All requirements met"
        else:
            issues = []
            if status.implementation_coverage < impl_threshold:
                issues.append(
                    f"Implementation coverage {status.implementation_coverage}% < {impl_threshold}%"
                )
            if status.test_coverage < test_threshold:
                issues.append(
                    f"Test coverage {status.test_coverage}% < {test_threshold}%"
                )
            verdict = "❌ INCOMPLETE: " + "; ".join(issues)

        return {
            "spec_id": status.spec_id,
            "feature": status.feature,
            "verified_at": status.verified_at,
            "verdict": verdict,
            "is_complete": status.is_complete,
            "thresholds": {
                "implementation": impl_threshold,
                "test": test_threshold,
            },
            "coverage": {
                "implementation": status.implementation_coverage,
                "test": status.test_coverage,
            },
            "counts": {
                "total_requirements": status.total_requirements,
                "implemented": status.implemented_count,
                "tested": status.tested_count,
            },
            "missing": {
                "implementations": status.missing_implementations,
                "tests": status.missing_tests,
            },
            "next_steps": self._generate_next_steps(status),
        }

    def _generate_next_steps(self, status: CompletionStatus) -> list[str]:
        """Generate suggested next steps based on completion status.

        Args:
            status: Completion status

        Returns:
            List of suggested next steps
        """
        steps = []

        if status.missing_implementations:
            count = len(status.missing_implementations)
            steps.append(
                f"Implement {count} missing requirement(s): "
                + ", ".join(status.missing_implementations[:3])
                + ("..." if count > 3 else "")
            )

        if status.missing_tests:
            count = len(status.missing_tests)
            steps.append(
                f"Add tests for {count} requirement(s): "
                + ", ".join(status.missing_tests[:3])
                + ("..." if count > 3 else "")
            )

        if not steps:
            steps.append("All requirements are implemented and tested!")

        return steps


# MCP Tool Definitions
MCP_TOOLS = [
    {
        "name": "analyze_spec_gaps",
        "description": "Analyze gaps between EARS specifications and implementation. "
        "Shows which requirements are implemented, partially implemented, or missing.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "spec_id": {
                    "type": "string",
                    "description": "Specification identifier (directory name in .c4/specs/)",
                },
                "language": {
                    "type": "string",
                    "enum": ["python", "typescript"],
                    "default": "python",
                    "description": "Primary programming language",
                },
                "format": {
                    "type": "string",
                    "enum": ["json", "markdown"],
                    "default": "json",
                    "description": "Output format",
                },
            },
            "required": ["spec_id"],
        },
    },
    {
        "name": "generate_tests_from_spec",
        "description": "Generate test stubs from EARS specification patterns. "
        "Creates pytest or vitest/jest test templates based on requirements.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "spec_id": {
                    "type": "string",
                    "description": "Specification identifier",
                },
                "language": {
                    "type": "string",
                    "enum": ["python", "typescript"],
                    "default": "python",
                    "description": "Target test language",
                },
                "test_format": {
                    "type": "string",
                    "enum": ["pytest", "vitest", "jest"],
                    "default": "pytest",
                    "description": "Test framework format",
                },
                "output_file": {
                    "type": "boolean",
                    "default": False,
                    "description": "Write to file (True) or return stubs (False)",
                },
                "check_duplicates": {
                    "type": "boolean",
                    "default": True,
                    "description": "Skip tests that already exist",
                },
            },
            "required": ["spec_id"],
        },
    },
    {
        "name": "link_impl_to_spec",
        "description": "Map code symbols to EARS requirements. "
        "Finds which functions/classes likely implement each requirement.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "spec_id": {
                    "type": "string",
                    "description": "Specification identifier",
                },
                "language": {
                    "type": "string",
                    "enum": ["python", "typescript"],
                    "default": "python",
                    "description": "Primary programming language",
                },
                "min_confidence": {
                    "type": "number",
                    "minimum": 0,
                    "maximum": 1,
                    "default": 0.3,
                    "description": "Minimum confidence score to include a link",
                },
            },
            "required": ["spec_id"],
        },
    },
    {
        "name": "verify_spec_completion",
        "description": "Verify that all EARS requirements are implemented and tested. "
        "Checks against configurable coverage thresholds.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "spec_id": {
                    "type": "string",
                    "description": "Specification identifier",
                },
                "language": {
                    "type": "string",
                    "enum": ["python", "typescript"],
                    "default": "python",
                    "description": "Primary programming language",
                },
                "impl_threshold": {
                    "type": "number",
                    "minimum": 0,
                    "maximum": 100,
                    "default": 90,
                    "description": "Minimum implementation coverage percentage",
                },
                "test_threshold": {
                    "type": "number",
                    "minimum": 0,
                    "maximum": 100,
                    "default": 80,
                    "description": "Minimum test coverage percentage",
                },
            },
            "required": ["spec_id"],
        },
    },
    {
        "name": "list_specs",
        "description": "List all available EARS specifications in the project.",
        "inputSchema": {
            "type": "object",
            "properties": {},
        },
    },
]


def get_mcp_tools() -> list[dict[str, Any]]:
    """Get MCP tool definitions for gap analyzer.

    Returns:
        List of MCP tool definitions
    """
    return MCP_TOOLS


def handle_mcp_tool_call(
    tool_name: str,
    arguments: dict[str, Any],
    project_root: str | Path | None = None,
) -> dict[str, Any] | str:
    """Handle an MCP tool call.

    Args:
        tool_name: Name of the tool to call
        arguments: Tool arguments
        project_root: Project root directory

    Returns:
        Tool result
    """
    analyzer = MCPGapAnalyzer(project_root=project_root)

    if tool_name == "analyze_spec_gaps":
        format_str = arguments.get("format", "json")
        format_enum = GapFormat.MARKDOWN if format_str == "markdown" else GapFormat.JSON
        return analyzer.analyze_spec_gaps(
            spec_id=arguments["spec_id"],
            language=arguments.get("language", "python"),
            format=format_enum,
        )

    elif tool_name == "generate_tests_from_spec":
        return analyzer.generate_tests_from_spec(
            spec_id=arguments["spec_id"],
            language=arguments.get("language", "python"),
            test_format=arguments.get("test_format", "pytest"),
            output_file=arguments.get("output_file", False),
            check_duplicates=arguments.get("check_duplicates", True),
        )

    elif tool_name == "link_impl_to_spec":
        return analyzer.link_impl_to_spec(
            spec_id=arguments["spec_id"],
            language=arguments.get("language", "python"),
            min_confidence=arguments.get("min_confidence", 0.3),
        )

    elif tool_name == "verify_spec_completion":
        return analyzer.verify_spec_completion(
            spec_id=arguments["spec_id"],
            language=arguments.get("language", "python"),
            impl_threshold=arguments.get("impl_threshold", 90.0),
            test_threshold=arguments.get("test_threshold", 80.0),
        )

    elif tool_name == "list_specs":
        return {"specs": analyzer.list_specs()}

    else:
        return {"error": f"Unknown tool: {tool_name}"}
