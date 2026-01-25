"""Gap Analyzer for EARS Specifications.

Analyzes the gap between specifications and implementation:
- Spec coverage analysis (which requirements are implemented)
- Test coverage integration
- Structured gap reports (JSON/Markdown)
- Priority suggestions for unimplemented requirements
"""

from __future__ import annotations

import json
import re
from dataclasses import dataclass, field
from datetime import datetime, timezone
from enum import Enum
from pathlib import Path
from typing import Any

import yaml

from .testgen import EarsPattern, Requirement, TestGenerator


class ImplementationStatus(Enum):
    """Implementation status of a requirement."""

    NOT_IMPLEMENTED = "not_implemented"
    PARTIALLY_IMPLEMENTED = "partially_implemented"
    IMPLEMENTED = "implemented"
    TESTED = "tested"


class Priority(Enum):
    """Priority level for implementation."""

    CRITICAL = 1
    HIGH = 2
    MEDIUM = 3
    LOW = 4


@dataclass
class RequirementGap:
    """Gap analysis result for a single requirement."""

    requirement: Requirement
    implementation_status: ImplementationStatus
    test_coverage: bool
    matched_symbols: list[str] = field(default_factory=list)
    matched_files: list[str] = field(default_factory=list)
    suggested_priority: Priority = Priority.MEDIUM
    notes: str = ""


@dataclass
class GapAnalysisResult:
    """Result of gap analysis for a specification."""

    spec_id: str
    feature: str
    domain: str
    total_requirements: int
    testable_requirements: int
    analyzed_at: str

    # Coverage metrics
    implementation_coverage: float  # 0-100
    test_coverage: float  # 0-100

    # Detailed gaps
    gaps: list[RequirementGap]

    # Summary counts
    implemented_count: int
    partially_implemented_count: int
    not_implemented_count: int
    tested_count: int

    # Files analyzed
    source_files_analyzed: int
    test_files_analyzed: int


class GapAnalyzer:
    """Gap Analyzer for EARS Specifications.

    Analyzes implementation and test coverage for EARS requirements.

    Example:
        analyzer = GapAnalyzer(specs_dir=".c4/specs", source_dir="src")
        result = analyzer.analyze_gaps("user-auth")

        # Generate JSON report
        json_report = analyzer.generate_report(result, format="json")

        # Generate Markdown report
        md_report = analyzer.generate_report(result, format="markdown")
    """

    # Keywords commonly found in implementations
    IMPL_KEYWORDS = {
        "ubiquitous": ["shall", "must", "always", "ensure"],
        "event-driven": ["when", "on", "handle", "trigger", "event", "listener"],
        "state-driven": ["while", "during", "state", "status", "is_active"],
        "optional": ["if", "when enabled", "optional", "feature flag", "config"],
        "unwanted": ["prevent", "block", "deny", "reject", "error", "exception"],
    }

    def __init__(
        self,
        specs_dir: str | Path = ".c4/specs",
        source_dir: str | Path = "src",
        tests_dir: str | Path = "tests",
    ) -> None:
        """Initialize the gap analyzer.

        Args:
            specs_dir: Directory containing spec files
            source_dir: Directory containing source code
            tests_dir: Directory containing test files
        """
        self.specs_dir = Path(specs_dir)
        self.source_dir = Path(source_dir)
        self.tests_dir = Path(tests_dir)
        self._test_generator = TestGenerator(
            specs_dir=specs_dir, tests_dir=tests_dir
        )

    def _load_spec(self, spec_id: str) -> dict[str, Any]:
        """Load a specification file.

        Args:
            spec_id: Specification identifier (directory name)

        Returns:
            Parsed spec data

        Raises:
            FileNotFoundError: If spec file doesn't exist
        """
        spec_path = self.specs_dir / spec_id / "requirements.yaml"

        if not spec_path.exists():
            raise FileNotFoundError(f"Spec not found: {spec_path}")

        with open(spec_path, encoding="utf-8") as f:
            return yaml.safe_load(f)

    def _parse_requirements(self, spec_data: dict[str, Any]) -> list[Requirement]:
        """Parse requirements from spec data.

        Args:
            spec_data: Parsed YAML spec data

        Returns:
            List of Requirement objects
        """
        requirements = []

        for req_data in spec_data.get("requirements", []):
            try:
                pattern = EarsPattern(req_data.get("pattern", "ubiquitous"))
            except ValueError:
                pattern = EarsPattern.UBIQUITOUS

            req = Requirement(
                id=req_data.get("id", ""),
                pattern=pattern,
                text=req_data.get("text", ""),
                domain=req_data.get("domain", ""),
                priority=req_data.get("priority", 0),
                testable=req_data.get("testable", True),
            )
            requirements.append(req)

        return requirements

    def _get_source_files(self, extensions: set[str] | None = None) -> list[Path]:
        """Get all source files to analyze.

        Args:
            extensions: File extensions to include (default: Python and TypeScript)

        Returns:
            List of source file paths
        """
        if extensions is None:
            extensions = {".py", ".ts", ".tsx", ".js", ".jsx"}

        files = []

        if not self.source_dir.exists():
            return files

        for ext in extensions:
            files.extend(self.source_dir.rglob(f"*{ext}"))

        # Also check project root for common files
        project_root = self.specs_dir.parent.parent
        if project_root.exists():
            for common_dir in ["c4", "src", "lib", "app"]:
                common_path = project_root / common_dir
                if common_path.exists():
                    for ext in extensions:
                        files.extend(common_path.rglob(f"*{ext}"))

        return list(set(files))

    def _extract_keywords_from_requirement(self, req: Requirement) -> list[str]:
        """Extract searchable keywords from a requirement.

        Args:
            req: Requirement to analyze

        Returns:
            List of keywords to search for
        """
        keywords = []

        # Extract significant words from requirement text
        text = req.text.lower()

        # Remove common EARS prefixes
        text = re.sub(r"^(the system shall|when|while|if|where)\s+", "", text)
        text = re.sub(r"^(be able to|have|provide|support|allow)\s+", "", text)

        # Split into words and filter
        words = re.findall(r"\b[a-z_]{3,}\b", text)

        # Remove common stop words
        stop_words = {
            "the", "and", "for", "that", "with", "this", "from", "have",
            "will", "shall", "must", "should", "can", "may", "not",
        }
        keywords = [w for w in words if w not in stop_words]

        # Add requirement ID as searchable (normalized)
        req_id_normalized = req.id.lower().replace("-", "_")
        keywords.append(req_id_normalized)

        return keywords[:10]  # Limit to top 10 keywords

    def _search_implementation(
        self,
        req: Requirement,
        source_files: list[Path],
    ) -> tuple[ImplementationStatus, list[str], list[str]]:
        """Search for implementation of a requirement.

        Args:
            req: Requirement to search for
            source_files: List of source files to search

        Returns:
            Tuple of (status, matched_symbols, matched_files)
        """
        keywords = self._extract_keywords_from_requirement(req)
        pattern_keywords = self.IMPL_KEYWORDS.get(req.pattern.value, [])

        matched_symbols = []
        matched_files = []
        keyword_matches = 0
        pattern_matches = 0

        for file_path in source_files:
            try:
                content = file_path.read_text(encoding="utf-8", errors="ignore")
                content_lower = content.lower()

                file_matched = False

                # Search for requirement keywords
                for keyword in keywords:
                    if keyword in content_lower:
                        keyword_matches += 1
                        file_matched = True

                        # Try to find the symbol/function name
                        # Look for function definitions containing the keyword
                        func_pattern = rf"(?:def|function|const|class)\s+(\w*{re.escape(keyword)}\w*)"
                        matches = re.findall(func_pattern, content, re.IGNORECASE)
                        matched_symbols.extend(matches)

                # Search for pattern-specific keywords
                for pattern_kw in pattern_keywords:
                    if pattern_kw in content_lower:
                        pattern_matches += 1
                        file_matched = True

                if file_matched:
                    matched_files.append(str(file_path))

            except Exception:
                continue

        # Deduplicate
        matched_symbols = list(set(matched_symbols))[:10]
        matched_files = list(set(matched_files))[:10]

        # Determine implementation status
        if keyword_matches >= 3 and pattern_matches >= 1:
            status = ImplementationStatus.IMPLEMENTED
        elif keyword_matches >= 1 or pattern_matches >= 1:
            status = ImplementationStatus.PARTIALLY_IMPLEMENTED
        else:
            status = ImplementationStatus.NOT_IMPLEMENTED

        return status, matched_symbols, matched_files

    def _calculate_priority(
        self,
        req: Requirement,
        impl_status: ImplementationStatus,
        has_test: bool,
    ) -> Priority:
        """Calculate suggested priority for a requirement.

        Args:
            req: Requirement
            impl_status: Current implementation status
            has_test: Whether requirement has test coverage

        Returns:
            Suggested priority
        """
        # Base priority from requirement
        base_priority = req.priority if req.priority > 0 else 2

        # Adjust based on EARS pattern (unwanted = higher priority)
        pattern_adjustment = {
            EarsPattern.UNWANTED: -1,  # Higher priority (error handling)
            EarsPattern.UBIQUITOUS: 0,
            EarsPattern.EVENT_DRIVEN: 0,
            EarsPattern.STATE_DRIVEN: 0,
            EarsPattern.OPTIONAL: 1,  # Lower priority
        }
        adjusted = base_priority + pattern_adjustment.get(req.pattern, 0)

        # Adjust based on implementation status
        if impl_status == ImplementationStatus.NOT_IMPLEMENTED:
            adjusted -= 1  # Higher priority if not implemented
        elif impl_status == ImplementationStatus.IMPLEMENTED and not has_test:
            adjusted = 2  # Medium priority for implemented but untested

        # Clamp to valid range
        adjusted = max(1, min(4, adjusted))

        return Priority(adjusted)

    def analyze_gaps(
        self,
        spec_id: str,
        language: str = "python",
    ) -> GapAnalysisResult:
        """Analyze implementation and test gaps for a specification.

        Args:
            spec_id: Specification identifier
            language: Primary language ("python" or "typescript")

        Returns:
            GapAnalysisResult with detailed analysis
        """
        # Load specification
        spec_data = self._load_spec(spec_id)
        requirements = self._parse_requirements(spec_data)

        # Get test coverage summary
        test_coverage_summary = self._test_generator.get_coverage_summary(
            spec_id, language=language
        )
        tested_reqs = set(test_coverage_summary.get("uncovered", []))
        tested_reqs = set(
            req.id for req in requirements if req.testable
        ) - tested_reqs

        # Get source files
        extensions = {".py", ".pyi"} if language == "python" else {".ts", ".tsx", ".js", ".jsx"}
        source_files = self._get_source_files(extensions)

        # Analyze each requirement
        gaps: list[RequirementGap] = []
        implemented_count = 0
        partially_count = 0
        not_implemented_count = 0
        tested_count = 0

        for req in requirements:
            # Skip non-testable requirements for gap analysis
            if not req.testable:
                continue

            # Check implementation status
            impl_status, matched_symbols, matched_files = self._search_implementation(
                req, source_files
            )

            # Check test coverage
            has_test = req.id in tested_reqs

            # Update status if tested
            if has_test and impl_status == ImplementationStatus.IMPLEMENTED:
                impl_status = ImplementationStatus.TESTED
                tested_count += 1
            elif impl_status == ImplementationStatus.IMPLEMENTED:
                implemented_count += 1
            elif impl_status == ImplementationStatus.PARTIALLY_IMPLEMENTED:
                partially_count += 1
            else:
                not_implemented_count += 1

            # Calculate suggested priority
            priority = self._calculate_priority(req, impl_status, has_test)

            gap = RequirementGap(
                requirement=req,
                implementation_status=impl_status,
                test_coverage=has_test,
                matched_symbols=matched_symbols,
                matched_files=matched_files,
                suggested_priority=priority,
            )
            gaps.append(gap)

        # Calculate coverage percentages
        testable_count = len([r for r in requirements if r.testable])
        impl_coverage = (
            (implemented_count + partially_count * 0.5 + tested_count)
            / testable_count * 100
            if testable_count > 0
            else 0
        )
        test_coverage = (
            tested_count / testable_count * 100 if testable_count > 0 else 0
        )

        return GapAnalysisResult(
            spec_id=spec_id,
            feature=spec_data.get("feature", spec_id),
            domain=spec_data.get("domain", "unknown"),
            total_requirements=len(requirements),
            testable_requirements=testable_count,
            analyzed_at=datetime.now(timezone.utc).isoformat(),
            implementation_coverage=round(impl_coverage, 2),
            test_coverage=round(test_coverage, 2),
            gaps=gaps,
            implemented_count=implemented_count,
            partially_implemented_count=partially_count,
            not_implemented_count=not_implemented_count,
            tested_count=tested_count,
            source_files_analyzed=len(source_files),
            test_files_analyzed=test_coverage_summary.get("testable_requirements", 0),
        )

    def get_unimplemented_requirements(
        self,
        spec_id: str,
        language: str = "python",
    ) -> list[dict[str, Any]]:
        """Get list of unimplemented requirements with priority suggestions.

        Args:
            spec_id: Specification identifier
            language: Primary language

        Returns:
            List of unimplemented requirements with priorities
        """
        result = self.analyze_gaps(spec_id, language)

        unimplemented = []
        for gap in result.gaps:
            if gap.implementation_status in (
                ImplementationStatus.NOT_IMPLEMENTED,
                ImplementationStatus.PARTIALLY_IMPLEMENTED,
            ):
                unimplemented.append({
                    "id": gap.requirement.id,
                    "text": gap.requirement.text,
                    "pattern": gap.requirement.pattern.value,
                    "status": gap.implementation_status.value,
                    "priority": gap.suggested_priority.value,
                    "priority_label": gap.suggested_priority.name,
                    "has_test": gap.test_coverage,
                    "matched_symbols": gap.matched_symbols,
                    "notes": gap.notes,
                })

        # Sort by priority (lower value = higher priority)
        unimplemented.sort(key=lambda x: x["priority"])

        return unimplemented

    def generate_report(
        self,
        result: GapAnalysisResult,
        format: str = "json",  # noqa: A002
    ) -> str:
        """Generate a gap analysis report.

        Args:
            result: GapAnalysisResult to report
            format: Output format ("json" or "markdown")

        Returns:
            Formatted report string
        """
        if format == "json":
            return self._generate_json_report(result)
        elif format == "markdown":
            return self._generate_markdown_report(result)
        else:
            raise ValueError(f"Unknown format: {format}. Use 'json' or 'markdown'.")

    def _generate_json_report(self, result: GapAnalysisResult) -> str:
        """Generate JSON report.

        Args:
            result: GapAnalysisResult

        Returns:
            JSON string
        """
        report = {
            "spec_id": result.spec_id,
            "feature": result.feature,
            "domain": result.domain,
            "analyzed_at": result.analyzed_at,
            "summary": {
                "total_requirements": result.total_requirements,
                "testable_requirements": result.testable_requirements,
                "implementation_coverage": result.implementation_coverage,
                "test_coverage": result.test_coverage,
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

        return json.dumps(report, indent=2, ensure_ascii=False)

    def _generate_markdown_report(self, result: GapAnalysisResult) -> str:
        """Generate Markdown report.

        Args:
            result: GapAnalysisResult

        Returns:
            Markdown string
        """
        lines = [
            f"# Gap Analysis Report: {result.feature}",
            "",
            f"**Spec ID:** {result.spec_id}",
            f"**Domain:** {result.domain}",
            f"**Analyzed:** {result.analyzed_at}",
            "",
            "## Summary",
            "",
            "| Metric | Value |",
            "|--------|-------|",
            f"| Total Requirements | {result.total_requirements} |",
            f"| Testable Requirements | {result.testable_requirements} |",
            f"| Implementation Coverage | {result.implementation_coverage}% |",
            f"| Test Coverage | {result.test_coverage}% |",
            "",
            "### Implementation Status",
            "",
            f"- ✅ **Tested:** {result.tested_count}",
            f"- ✓ **Implemented:** {result.implemented_count}",
            f"- ⚠️ **Partially Implemented:** {result.partially_implemented_count}",
            f"- ❌ **Not Implemented:** {result.not_implemented_count}",
            "",
            f"**Source Files Analyzed:** {result.source_files_analyzed}",
            "",
            "## Detailed Gaps",
            "",
        ]

        # Group by status
        status_order = [
            ImplementationStatus.NOT_IMPLEMENTED,
            ImplementationStatus.PARTIALLY_IMPLEMENTED,
            ImplementationStatus.IMPLEMENTED,
            ImplementationStatus.TESTED,
        ]

        status_emoji = {
            ImplementationStatus.NOT_IMPLEMENTED: "❌",
            ImplementationStatus.PARTIALLY_IMPLEMENTED: "⚠️",
            ImplementationStatus.IMPLEMENTED: "✓",
            ImplementationStatus.TESTED: "✅",
        }

        for status in status_order:
            status_gaps = [g for g in result.gaps if g.implementation_status == status]
            if not status_gaps:
                continue

            lines.append(f"### {status_emoji[status]} {status.value.replace('_', ' ').title()}")
            lines.append("")

            for gap in sorted(status_gaps, key=lambda g: g.suggested_priority.value):
                priority_badge = f"P{gap.suggested_priority.value}"
                test_badge = "🧪" if gap.test_coverage else ""

                lines.append(f"#### {gap.requirement.id} [{priority_badge}] {test_badge}")
                lines.append("")
                lines.append(f"> {gap.requirement.text}")
                lines.append("")
                lines.append(f"- **Pattern:** {gap.requirement.pattern.value}")

                if gap.matched_symbols:
                    lines.append(f"- **Matched Symbols:** `{'`, `'.join(gap.matched_symbols[:5])}`")

                if gap.matched_files:
                    files_display = [Path(f).name for f in gap.matched_files[:3]]
                    lines.append(f"- **Found in:** {', '.join(files_display)}")

                lines.append("")

        # Priority recommendations
        high_priority = [
            g for g in result.gaps
            if g.suggested_priority in (Priority.CRITICAL, Priority.HIGH)
            and g.implementation_status != ImplementationStatus.TESTED
        ]

        if high_priority:
            lines.append("## 🎯 Priority Recommendations")
            lines.append("")
            lines.append("The following requirements should be addressed first:")
            lines.append("")

            for gap in sorted(high_priority, key=lambda g: g.suggested_priority.value):
                lines.append(f"1. **{gap.requirement.id}** - {gap.requirement.text[:80]}...")
                lines.append(f"   - Priority: {gap.suggested_priority.name}")
                lines.append(f"   - Status: {gap.implementation_status.value}")
                lines.append("")

        return "\n".join(lines)

    def list_specs(self) -> list[dict[str, Any]]:
        """List all available specifications.

        Returns:
            List of spec summaries
        """
        specs = []

        if not self.specs_dir.exists():
            return specs

        for spec_dir in self.specs_dir.iterdir():
            if not spec_dir.is_dir():
                continue

            req_file = spec_dir / "requirements.yaml"
            if not req_file.exists():
                continue

            try:
                with open(req_file, encoding="utf-8") as f:
                    data = yaml.safe_load(f)

                specs.append({
                    "id": spec_dir.name,
                    "feature": data.get("feature", spec_dir.name),
                    "domain": data.get("domain", "unknown"),
                    "requirements_count": len(data.get("requirements", [])),
                })
            except Exception:
                continue

        return specs
