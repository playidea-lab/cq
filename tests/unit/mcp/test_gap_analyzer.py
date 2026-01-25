"""Tests for MCP Gap Analyzer Tools."""

from __future__ import annotations

from dataclasses import dataclass
from unittest.mock import MagicMock, patch

import pytest

from c4.docs.gap import GapAnalysisResult, ImplementationStatus, Priority, RequirementGap
from c4.docs.testgen import EarsPattern, TestFormat, TestGenerationResult, TestStub


@pytest.fixture
def mock_gap_analyzer():
    """Create a mock GapAnalyzer."""
    analyzer = MagicMock()

    # Mock list_specs
    analyzer.list_specs.return_value = [
        {"id": "user-auth", "feature": "User Authentication", "domain": "web-backend", "requirements_count": 5},
        {"id": "dashboard", "feature": "Dashboard", "domain": "web-frontend", "requirements_count": 3},
    ]

    return analyzer


@pytest.fixture
def mock_test_generator():
    """Create a mock TestGenerator."""
    generator = MagicMock()
    return generator


@pytest.fixture
def sample_gap_result():
    """Create a sample GapAnalysisResult."""
    @dataclass
    class MockRequirement:
        id: str
        text: str
        pattern: EarsPattern
        testable: bool = True

    gaps = [
        RequirementGap(
            requirement=MockRequirement(
                id="REQ-001",
                text="When user submits login form, the system shall validate credentials",
                pattern=EarsPattern.EVENT_DRIVEN,
            ),
            implementation_status=ImplementationStatus.IMPLEMENTED,
            test_coverage=True,
            matched_symbols=["validate_credentials", "login"],
            matched_files=["c4/auth/login.py", "c4/auth/validators.py"],
            suggested_priority=Priority.LOW,
        ),
        RequirementGap(
            requirement=MockRequirement(
                id="REQ-002",
                text="If credentials are invalid, the system shall display error message",
                pattern=EarsPattern.UNWANTED,
            ),
            implementation_status=ImplementationStatus.PARTIALLY_IMPLEMENTED,
            test_coverage=False,
            matched_symbols=["display_error"],
            matched_files=["c4/auth/errors.py"],
            suggested_priority=Priority.MEDIUM,
        ),
        RequirementGap(
            requirement=MockRequirement(
                id="REQ-003",
                text="The system shall encrypt user passwords",
                pattern=EarsPattern.UBIQUITOUS,
            ),
            implementation_status=ImplementationStatus.NOT_IMPLEMENTED,
            test_coverage=False,
            matched_symbols=[],
            matched_files=[],
            suggested_priority=Priority.HIGH,
        ),
    ]

    return GapAnalysisResult(
        spec_id="user-auth",
        feature="User Authentication",
        domain="web-backend",
        analyzed_at="2024-01-15T10:00:00Z",
        total_requirements=3,
        testable_requirements=3,
        implementation_coverage=66.7,
        test_coverage=33.3,
        implemented_count=1,
        partially_implemented_count=1,
        not_implemented_count=1,
        tested_count=1,
        source_files_analyzed=10,
        test_files_analyzed=5,
        gaps=gaps,
    )


@pytest.fixture
def sample_test_result():
    """Create a sample TestGenerationResult."""
    stubs = [
        TestStub(
            name="test_login_validates_credentials_on_form_submit",
            description="When user submits login form, the system shall validate credentials",
            requirement_id="REQ-001",
            pattern=EarsPattern.EVENT_DRIVEN,
            code='def test_login_validates_credentials_on_form_submit():\n    """Test: When user submits login form, the system shall validate credentials."""\n    pass',
            language="python",
            format=TestFormat.PYTEST,
        ),
        TestStub(
            name="test_displays_error_on_invalid_credentials",
            description="If credentials are invalid, the system shall display error message",
            requirement_id="REQ-002",
            pattern=EarsPattern.UNWANTED,
            code='def test_displays_error_on_invalid_credentials():\n    """Test: If credentials are invalid, the system shall display error message."""\n    pass',
            language="python",
            format=TestFormat.PYTEST,
        ),
    ]

    return TestGenerationResult(
        spec_id="user-auth",
        generated_at="2024-01-15T10:00:00Z",
        total_requirements=3,
        stubs=stubs,
        skipped=["REQ-003 (already exists)"],
    )


class TestMCPGapAnalyzerInit:
    """Tests for MCPGapAnalyzer initialization."""

    def test_init_with_defaults(self, tmp_path):
        """Test initialization with default directories."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer"), \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)

            assert analyzer.project_root == tmp_path
            assert analyzer.specs_dir == tmp_path / ".c4" / "specs"
            assert analyzer.tests_dir == tmp_path / "tests"

    def test_init_with_custom_dirs(self, tmp_path):
        """Test initialization with custom directories."""
        specs = tmp_path / "custom_specs"
        source = tmp_path / "custom_src"
        tests = tmp_path / "custom_tests"

        with patch("c4.mcp.gap_analyzer.GapAnalyzer"), \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(
                project_root=tmp_path,
                specs_dir=specs,
                source_dir=source,
                tests_dir=tests,
            )

            assert analyzer.specs_dir == specs
            assert analyzer.source_dir == source
            assert analyzer.tests_dir == tests

    def test_init_finds_common_source_dirs(self, tmp_path):
        """Test that init finds common source directories."""
        # Create c4 directory
        (tmp_path / "c4").mkdir()

        with patch("c4.mcp.gap_analyzer.GapAnalyzer"), \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)

            # Should find c4 as source dir
            assert analyzer.source_dir == tmp_path / "c4"


class TestAnalyzeSpecGaps:
    """Tests for analyze_spec_gaps tool."""

    def test_analyze_gaps_json_format(self, tmp_path, sample_gap_result):
        """Test gap analysis with JSON format."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer") as MockGapAnalyzer, \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            mock_analyzer = MockGapAnalyzer.return_value
            mock_analyzer.analyze_gaps.return_value = sample_gap_result

            from c4.mcp.gap_analyzer import GapFormat, MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)
            result = analyzer.analyze_spec_gaps("user-auth", format=GapFormat.JSON)

            assert result["spec_id"] == "user-auth"
            assert result["feature"] == "User Authentication"
            assert result["summary"]["total_requirements"] == 3
            assert result["summary"]["implementation_coverage"] == 66.7
            assert len(result["gaps"]) == 3

    def test_analyze_gaps_markdown_format(self, tmp_path, sample_gap_result):
        """Test gap analysis with markdown format."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer") as MockGapAnalyzer, \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            mock_analyzer = MockGapAnalyzer.return_value
            mock_analyzer.analyze_gaps.return_value = sample_gap_result
            mock_analyzer.generate_report.return_value = "# Gap Analysis Report\n..."

            from c4.mcp.gap_analyzer import GapFormat, MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)
            result = analyzer.analyze_spec_gaps("user-auth", format=GapFormat.MARKDOWN)

            assert isinstance(result, str)
            assert "Gap Analysis Report" in result
            mock_analyzer.generate_report.assert_called_once()

    def test_analyze_gaps_file_not_found(self, tmp_path):
        """Test gap analysis when spec not found."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer") as MockGapAnalyzer, \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            mock_analyzer = MockGapAnalyzer.return_value
            mock_analyzer.analyze_gaps.side_effect = FileNotFoundError("Spec not found")

            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)
            result = analyzer.analyze_spec_gaps("nonexistent")

            assert "error" in result
            assert "Spec not found" in result["error"]

    def test_analyze_gaps_exception_handling(self, tmp_path):
        """Test gap analysis handles unexpected exceptions."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer") as MockGapAnalyzer, \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            mock_analyzer = MockGapAnalyzer.return_value
            mock_analyzer.analyze_gaps.side_effect = RuntimeError("Unexpected error")

            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)
            result = analyzer.analyze_spec_gaps("user-auth")

            assert "error" in result
            assert "Gap analysis failed" in result["error"]


class TestGenerateTestsFromSpec:
    """Tests for generate_tests_from_spec tool."""

    def test_generate_stubs_only(self, tmp_path, sample_test_result):
        """Test generating test stubs without writing to file."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer"), \
             patch("c4.mcp.gap_analyzer.TestGenerator") as MockTestGen:
            mock_gen = MockTestGen.return_value
            mock_gen.generate_test_stubs.return_value = sample_test_result

            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)
            result = analyzer.generate_tests_from_spec(
                spec_id="user-auth",
                language="python",
                test_format="pytest",
                output_file=False,
            )

            assert result["spec_id"] == "user-auth"
            assert result["generated_count"] == 2
            assert len(result["stubs"]) == 2
            assert result["stubs"][0]["requirement_id"] == "REQ-001"

    def test_generate_to_file(self, tmp_path):
        """Test generating tests and writing to file."""
        output_path = tmp_path / "tests" / "test_user_auth.py"

        with patch("c4.mcp.gap_analyzer.GapAnalyzer"), \
             patch("c4.mcp.gap_analyzer.TestGenerator") as MockTestGen:
            mock_gen = MockTestGen.return_value
            mock_gen.generate_test_file.return_value = output_path

            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)
            result = analyzer.generate_tests_from_spec(
                spec_id="user-auth",
                output_file=True,
            )

            assert result["success"] is True
            assert result["output_path"] == str(output_path)
            mock_gen.generate_test_file.assert_called_once()

    def test_generate_vitest_format(self, tmp_path, sample_test_result):
        """Test generating tests for vitest."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer"), \
             patch("c4.mcp.gap_analyzer.TestGenerator") as MockTestGen:
            mock_gen = MockTestGen.return_value
            mock_gen.generate_test_stubs.return_value = sample_test_result

            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)
            analyzer.generate_tests_from_spec(
                spec_id="user-auth",
                language="typescript",
                test_format="vitest",
            )

            mock_gen.generate_test_stubs.assert_called_once()
            call_kwargs = mock_gen.generate_test_stubs.call_args
            assert call_kwargs.kwargs.get("test_format") == TestFormat.VITEST

    def test_generate_file_not_found(self, tmp_path):
        """Test generation when spec not found."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer"), \
             patch("c4.mcp.gap_analyzer.TestGenerator") as MockTestGen:
            mock_gen = MockTestGen.return_value
            mock_gen.generate_test_stubs.side_effect = FileNotFoundError("Spec not found")

            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)
            result = analyzer.generate_tests_from_spec("nonexistent")

            assert "error" in result


class TestLinkImplToSpec:
    """Tests for link_impl_to_spec tool."""

    def test_link_with_symbols(self, tmp_path, sample_gap_result):
        """Test linking implementations to specifications."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer") as MockGapAnalyzer, \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            mock_analyzer = MockGapAnalyzer.return_value
            mock_analyzer.analyze_gaps.return_value = sample_gap_result

            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)
            result = analyzer.link_impl_to_spec("user-auth")

            assert result["spec_id"] == "user-auth"
            assert result["summary"]["total_requirements"] == 3
            assert len(result["links"]) > 0
            # REQ-003 has no symbols, so should be unlinked
            assert "REQ-003" in result["unlinked_requirements"]

    def test_link_with_min_confidence(self, tmp_path, sample_gap_result):
        """Test linking with high confidence threshold."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer") as MockGapAnalyzer, \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            mock_analyzer = MockGapAnalyzer.return_value
            mock_analyzer.analyze_gaps.return_value = sample_gap_result

            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)
            result = analyzer.link_impl_to_spec("user-auth", min_confidence=0.8)

            # With high confidence, fewer links should be included
            high_conf_links = [link for link in result["links"] if link["confidence"] >= 0.8]
            assert len(high_conf_links) <= len(result["links"])

    def test_link_file_not_found(self, tmp_path):
        """Test linking when spec not found."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer") as MockGapAnalyzer, \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            mock_analyzer = MockGapAnalyzer.return_value
            mock_analyzer.analyze_gaps.side_effect = FileNotFoundError("Spec not found")

            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)
            result = analyzer.link_impl_to_spec("nonexistent")

            assert "error" in result


class TestVerifySpecCompletion:
    """Tests for verify_spec_completion tool."""

    def test_verify_incomplete_spec(self, tmp_path, sample_gap_result):
        """Test verification of incomplete specification."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer") as MockGapAnalyzer, \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            mock_analyzer = MockGapAnalyzer.return_value
            mock_analyzer.analyze_gaps.return_value = sample_gap_result

            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)
            result = analyzer.verify_spec_completion("user-auth")

            assert result["is_complete"] is False
            assert "INCOMPLETE" in result["verdict"]
            assert result["coverage"]["implementation"] == 66.7
            assert result["coverage"]["test"] == 33.3
            assert len(result["missing"]["implementations"]) > 0
            assert len(result["missing"]["tests"]) > 0

    def test_verify_complete_spec(self, tmp_path):
        """Test verification of complete specification."""
        @dataclass
        class MockRequirement:
            id: str
            text: str
            pattern: EarsPattern
            testable: bool = True

        complete_result = GapAnalysisResult(
            spec_id="complete-spec",
            feature="Complete Feature",
            domain="web-backend",
            analyzed_at="2024-01-15T10:00:00Z",
            total_requirements=2,
            testable_requirements=2,
            implementation_coverage=100.0,
            test_coverage=100.0,
            implemented_count=0,
            partially_implemented_count=0,
            not_implemented_count=0,
            tested_count=2,
            source_files_analyzed=5,
            test_files_analyzed=3,
            gaps=[
                RequirementGap(
                    requirement=MockRequirement(
                        id="REQ-001",
                        text="Test requirement 1",
                        pattern=EarsPattern.UBIQUITOUS,
                    ),
                    implementation_status=ImplementationStatus.TESTED,
                    test_coverage=True,
                    matched_symbols=["func1"],
                    matched_files=["file1.py"],
                    suggested_priority=Priority.LOW,
                ),
            ],
        )

        with patch("c4.mcp.gap_analyzer.GapAnalyzer") as MockGapAnalyzer, \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            mock_analyzer = MockGapAnalyzer.return_value
            mock_analyzer.analyze_gaps.return_value = complete_result

            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)
            result = analyzer.verify_spec_completion("complete-spec")

            assert result["is_complete"] is True
            assert "COMPLETE" in result["verdict"]
            assert result["coverage"]["implementation"] == 100.0
            assert result["coverage"]["test"] == 100.0

    def test_verify_with_custom_thresholds(self, tmp_path, sample_gap_result):
        """Test verification with custom coverage thresholds."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer") as MockGapAnalyzer, \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            mock_analyzer = MockGapAnalyzer.return_value
            mock_analyzer.analyze_gaps.return_value = sample_gap_result

            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)
            # With lower thresholds, should pass
            result = analyzer.verify_spec_completion(
                "user-auth",
                impl_threshold=50.0,
                test_threshold=30.0,
            )

            assert result["is_complete"] is True
            assert result["thresholds"]["implementation"] == 50.0
            assert result["thresholds"]["test"] == 30.0

    def test_verify_file_not_found(self, tmp_path):
        """Test verification when spec not found."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer") as MockGapAnalyzer, \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            mock_analyzer = MockGapAnalyzer.return_value
            mock_analyzer.analyze_gaps.side_effect = FileNotFoundError("Spec not found")

            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)
            result = analyzer.verify_spec_completion("nonexistent")

            assert "error" in result


class TestListSpecs:
    """Tests for list_specs tool."""

    def test_list_specs(self, tmp_path):
        """Test listing available specifications."""
        mock_specs = [
            {"id": "user-auth", "feature": "Auth", "domain": "backend", "requirements_count": 5},
            {"id": "dashboard", "feature": "Dashboard", "domain": "frontend", "requirements_count": 3},
        ]

        with patch("c4.mcp.gap_analyzer.GapAnalyzer") as MockGapAnalyzer, \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            mock_analyzer = MockGapAnalyzer.return_value
            mock_analyzer.list_specs.return_value = mock_specs

            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)
            result = analyzer.list_specs()

            assert len(result) == 2
            assert result[0]["id"] == "user-auth"


class TestMCPToolHandling:
    """Tests for MCP tool handling functions."""

    def test_get_mcp_tools(self):
        """Test getting MCP tool definitions."""
        from c4.mcp.gap_analyzer import get_mcp_tools

        tools = get_mcp_tools()

        assert len(tools) == 5
        tool_names = [t["name"] for t in tools]
        assert "analyze_spec_gaps" in tool_names
        assert "generate_tests_from_spec" in tool_names
        assert "link_impl_to_spec" in tool_names
        assert "verify_spec_completion" in tool_names
        assert "list_specs" in tool_names

    def test_handle_analyze_spec_gaps(self, tmp_path, sample_gap_result):
        """Test handling analyze_spec_gaps tool call."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer") as MockGapAnalyzer, \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            mock_analyzer = MockGapAnalyzer.return_value
            mock_analyzer.analyze_gaps.return_value = sample_gap_result

            from c4.mcp.gap_analyzer import handle_mcp_tool_call

            result = handle_mcp_tool_call(
                "analyze_spec_gaps",
                {"spec_id": "user-auth", "language": "python", "format": "json"},
                project_root=tmp_path,
            )

            assert result["spec_id"] == "user-auth"

    def test_handle_generate_tests(self, tmp_path, sample_test_result):
        """Test handling generate_tests_from_spec tool call."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer"), \
             patch("c4.mcp.gap_analyzer.TestGenerator") as MockTestGen:
            mock_gen = MockTestGen.return_value
            mock_gen.generate_test_stubs.return_value = sample_test_result

            from c4.mcp.gap_analyzer import handle_mcp_tool_call

            result = handle_mcp_tool_call(
                "generate_tests_from_spec",
                {"spec_id": "user-auth"},
                project_root=tmp_path,
            )

            assert result["spec_id"] == "user-auth"
            assert "stubs" in result

    def test_handle_link_impl(self, tmp_path, sample_gap_result):
        """Test handling link_impl_to_spec tool call."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer") as MockGapAnalyzer, \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            mock_analyzer = MockGapAnalyzer.return_value
            mock_analyzer.analyze_gaps.return_value = sample_gap_result

            from c4.mcp.gap_analyzer import handle_mcp_tool_call

            result = handle_mcp_tool_call(
                "link_impl_to_spec",
                {"spec_id": "user-auth", "min_confidence": 0.5},
                project_root=tmp_path,
            )

            assert result["spec_id"] == "user-auth"
            assert "links" in result

    def test_handle_verify_completion(self, tmp_path, sample_gap_result):
        """Test handling verify_spec_completion tool call."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer") as MockGapAnalyzer, \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            mock_analyzer = MockGapAnalyzer.return_value
            mock_analyzer.analyze_gaps.return_value = sample_gap_result

            from c4.mcp.gap_analyzer import handle_mcp_tool_call

            result = handle_mcp_tool_call(
                "verify_spec_completion",
                {"spec_id": "user-auth", "impl_threshold": 80, "test_threshold": 70},
                project_root=tmp_path,
            )

            assert result["spec_id"] == "user-auth"
            assert "verdict" in result

    def test_handle_list_specs(self, tmp_path):
        """Test handling list_specs tool call."""
        mock_specs = [{"id": "test-spec", "feature": "Test"}]

        with patch("c4.mcp.gap_analyzer.GapAnalyzer") as MockGapAnalyzer, \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            mock_analyzer = MockGapAnalyzer.return_value
            mock_analyzer.list_specs.return_value = mock_specs

            from c4.mcp.gap_analyzer import handle_mcp_tool_call

            result = handle_mcp_tool_call(
                "list_specs",
                {},
                project_root=tmp_path,
            )

            assert "specs" in result
            assert len(result["specs"]) == 1

    def test_handle_unknown_tool(self, tmp_path):
        """Test handling unknown tool call."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer"), \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            from c4.mcp.gap_analyzer import handle_mcp_tool_call

            result = handle_mcp_tool_call(
                "unknown_tool",
                {},
                project_root=tmp_path,
            )

            assert "error" in result
            assert "Unknown tool" in result["error"]

    def test_handle_markdown_format(self, tmp_path, sample_gap_result):
        """Test handling analyze_spec_gaps with markdown format."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer") as MockGapAnalyzer, \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            mock_analyzer = MockGapAnalyzer.return_value
            mock_analyzer.analyze_gaps.return_value = sample_gap_result
            mock_analyzer.generate_report.return_value = "# Report"

            from c4.mcp.gap_analyzer import handle_mcp_tool_call

            result = handle_mcp_tool_call(
                "analyze_spec_gaps",
                {"spec_id": "user-auth", "format": "markdown"},
                project_root=tmp_path,
            )

            assert result == "# Report"


class TestConfidenceCalculation:
    """Tests for confidence score calculation."""

    def test_calculate_high_confidence(self, tmp_path):
        """Test high confidence for tested implementations."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer"), \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)

            confidence = analyzer._calculate_link_confidence(
                ImplementationStatus.TESTED,
                symbol_count=3,
                has_test=True,
            )

            # Should be close to 1.0
            assert confidence >= 0.95

    def test_calculate_low_confidence(self, tmp_path):
        """Test low confidence for not implemented."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer"), \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)

            confidence = analyzer._calculate_link_confidence(
                ImplementationStatus.NOT_IMPLEMENTED,
                symbol_count=1,
                has_test=False,
            )

            # Should be low
            assert confidence < 0.5

    def test_symbol_count_affects_confidence(self, tmp_path):
        """Test that more symbols increase confidence."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer"), \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)

            conf_1_symbol = analyzer._calculate_link_confidence(
                ImplementationStatus.IMPLEMENTED,
                symbol_count=1,
                has_test=False,
            )

            conf_3_symbols = analyzer._calculate_link_confidence(
                ImplementationStatus.IMPLEMENTED,
                symbol_count=3,
                has_test=False,
            )

            assert conf_3_symbols > conf_1_symbol


class TestMatchReasonGeneration:
    """Tests for match reason generation."""

    def test_tested_reason(self, tmp_path):
        """Test reason for tested status."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer"), \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)

            reason = analyzer._generate_match_reason(
                ImplementationStatus.TESTED,
                has_test=True,
            )

            assert "Fully implemented with tests" in reason

    def test_partial_with_test_reason(self, tmp_path):
        """Test reason for partial implementation with test."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer"), \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            from c4.mcp.gap_analyzer import MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)

            reason = analyzer._generate_match_reason(
                ImplementationStatus.PARTIALLY_IMPLEMENTED,
                has_test=True,
            )

            assert "Partially implemented" in reason
            assert "+ has test" in reason


class TestNextStepsGeneration:
    """Tests for next steps suggestion."""

    def test_next_steps_with_missing_impl(self, tmp_path):
        """Test next steps with missing implementations."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer"), \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            from c4.mcp.gap_analyzer import CompletionStatus, MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)

            status = CompletionStatus(
                spec_id="test",
                feature="Test",
                is_complete=False,
                implementation_coverage=50.0,
                test_coverage=30.0,
                total_requirements=4,
                implemented_count=2,
                tested_count=1,
                missing_implementations=["REQ-001", "REQ-002"],
                missing_tests=["REQ-003"],
                verified_at="2024-01-15T10:00:00Z",
            )

            steps = analyzer._generate_next_steps(status)

            assert len(steps) == 2
            assert "Implement 2 missing" in steps[0]
            assert "Add tests for 1" in steps[1]

    def test_next_steps_all_complete(self, tmp_path):
        """Test next steps when all is complete."""
        with patch("c4.mcp.gap_analyzer.GapAnalyzer"), \
             patch("c4.mcp.gap_analyzer.TestGenerator"):
            from c4.mcp.gap_analyzer import CompletionStatus, MCPGapAnalyzer

            analyzer = MCPGapAnalyzer(project_root=tmp_path)

            status = CompletionStatus(
                spec_id="test",
                feature="Test",
                is_complete=True,
                implementation_coverage=100.0,
                test_coverage=100.0,
                total_requirements=4,
                implemented_count=4,
                tested_count=4,
                missing_implementations=[],
                missing_tests=[],
                verified_at="2024-01-15T10:00:00Z",
            )

            steps = analyzer._generate_next_steps(status)

            assert len(steps) == 1
            assert "All requirements" in steps[0]


class TestGapFormat:
    """Tests for GapFormat enum."""

    def test_gap_format_values(self):
        """Test GapFormat enum values."""
        from c4.mcp.gap_analyzer import GapFormat

        assert GapFormat.JSON.value == "json"
        assert GapFormat.MARKDOWN.value == "markdown"


class TestDataclasses:
    """Tests for dataclasses."""

    def test_spec_link_creation(self):
        """Test SpecLink dataclass."""
        from c4.mcp.gap_analyzer import SpecLink

        link = SpecLink(
            requirement_id="REQ-001",
            requirement_text="Test requirement",
            pattern="event-driven",
            symbol_name="test_func",
            file_path="test.py",
            line_number=42,
            confidence=0.85,
            match_reason="Fully implemented",
        )

        assert link.requirement_id == "REQ-001"
        assert link.confidence == 0.85

    def test_spec_link_result_creation(self):
        """Test SpecLinkResult dataclass."""
        from c4.mcp.gap_analyzer import SpecLinkResult

        result = SpecLinkResult(
            spec_id="test",
            feature="Test Feature",
            total_requirements=5,
            linked_count=4,
            unlinked_count=1,
            links=[],
            unlinked_requirements=["REQ-005"],
            analyzed_at="2024-01-15T10:00:00Z",
        )

        assert result.linked_count == 4

    def test_completion_status_creation(self):
        """Test CompletionStatus dataclass."""
        from c4.mcp.gap_analyzer import CompletionStatus

        status = CompletionStatus(
            spec_id="test",
            feature="Test Feature",
            is_complete=False,
            implementation_coverage=80.0,
            test_coverage=70.0,
            total_requirements=10,
            implemented_count=8,
            tested_count=7,
            missing_implementations=["REQ-009", "REQ-010"],
            missing_tests=["REQ-008", "REQ-009", "REQ-010"],
            verified_at="2024-01-15T10:00:00Z",
        )

        assert status.is_complete is False
        assert len(status.missing_implementations) == 2
