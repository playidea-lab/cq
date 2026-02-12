"""
Tests for auto_review.orchestrator module.
"""

from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from c4.review.models import (
    OverallAssessment,
    PageImage,
    PaperStructure,
    ReviewConfig,
    ReviewSession,
    Section,
    SectionAnalysis,
    SectionEval,
)
from c4.review.orchestrator import ReviewOrchestrator


@pytest.fixture
def mock_page_images():
    """Create mock PageImage objects."""
    return [
        PageImage(page_number=1, image_data=b"fake_page_1", width=800, height=1200),
        PageImage(page_number=2, image_data=b"fake_page_2", width=800, height=1200),
    ]


@pytest.fixture
def mock_structure():
    """Create mock PaperStructure."""
    return PaperStructure(
        title="Test Paper",
        authors=["Alice Smith"],
        sections=[
            Section(title="Introduction", page_number=1, level=1),
            Section(title="Methodology", page_number=2, level=1),
        ],
        figures=[],
        tables=[],
        equations_count=5,
        references_count=20,
    )


@pytest.fixture
def mock_section_analysis():
    """Create mock SectionAnalysis."""
    return SectionAnalysis(
        sections=[
            SectionEval(
                section_name="Introduction",
                score=8,
                strengths=["Clear motivation"],
                weaknesses=["Limited background"],
                specific_comments=["Add more context"],
            ),
            SectionEval(
                section_name="Methodology",
                score=7,
                strengths=["Novel approach"],
                weaknesses=["Missing details"],
                specific_comments=["Clarify steps"],
            ),
        ],
        overall_notes="Good structure overall",
    )


@pytest.fixture
def mock_overall_assessment():
    """Create mock OverallAssessment."""
    return OverallAssessment(
        dimension_scores={"novelty": 4, "clarity": 3, "rigor": 4, "significance": 3},
        overall_score=7.5,
        recommendation="MinorRevision",
        summary="Interesting work with minor issues",
        key_strengths=["Novel approach", "Clear writing"],
        key_weaknesses=["Limited evaluation", "Missing baselines"],
    )


@pytest.fixture
def mock_converter(mock_page_images):
    """Create mock PDFConverter."""
    converter = MagicMock()
    converter.convert.return_value = mock_page_images
    return converter


@pytest.fixture
def mock_analyzer(mock_structure, mock_section_analysis, mock_overall_assessment):
    """Create mock PaperAnalyzer."""
    analyzer = MagicMock()
    analyzer.analyze_structure.return_value = mock_structure
    analyzer.analyze_sections.return_value = mock_section_analysis
    analyzer.generate_assessment.return_value = mock_overall_assessment
    return analyzer


@pytest.fixture
def mock_generator():
    """Create mock ReviewGenerator."""
    from c4.review.models import ReviewDocument

    generator = MagicMock()
    mock_review_doc = ReviewDocument(
        overall_assessment=OverallAssessment(
            dimension_scores={},
            overall_score=7.5,
            recommendation="MinorRevision",
            summary="Test summary",
        ),
        reviewer_notes="Test review content",
    )
    generator.generate_review.return_value = mock_review_doc
    return generator


@pytest.fixture
def orchestrator(mock_converter, mock_analyzer, mock_generator):
    """Create ReviewOrchestrator with mocked dependencies."""
    return ReviewOrchestrator(
        api_key="test-key",
        converter=mock_converter,
        analyzer=mock_analyzer,
        generator=mock_generator,
    )


def test_start_review_success(orchestrator, tmp_path):
    """Test successful review start."""
    # Create a dummy PDF file
    pdf_path = tmp_path / "test.pdf"
    pdf_path.write_bytes(b"fake pdf content")

    config = ReviewConfig(model_name="claude-opus-4")

    session = orchestrator.start_review(pdf_path, config)

    # Verify session
    assert session.pdf_path == str(pdf_path)
    assert session.config.model_name == "claude-opus-4"
    assert session.pages is not None
    assert len(session.pages) == 2
    assert session.created_at is not None
    assert session.structure is None  # Not yet run
    assert session.section_analysis is None
    assert session.overall_assessment is None


def test_start_review_invalid_pdf(orchestrator, tmp_path):
    """Test starting review with non-existent PDF."""
    pdf_path = tmp_path / "nonexistent.pdf"

    with pytest.raises(FileNotFoundError, match="PDF file not found"):
        orchestrator.start_review(pdf_path)


def test_run_pass1_success(orchestrator, tmp_path):
    """Test Pass 1 structure analysis."""
    # Create session
    pdf_path = tmp_path / "test.pdf"
    pdf_path.write_bytes(b"fake pdf")
    session = orchestrator.start_review(pdf_path)

    # Run Pass 1
    structure = orchestrator.run_pass1(session)

    # Verify results
    assert structure.title == "Test Paper"
    assert len(structure.sections) == 2
    assert session.structure is not None
    assert session.structure.title == "Test Paper"


def test_run_pass1_no_pages(orchestrator):
    """Test Pass 1 with no pages in session."""
    session = ReviewSession(pdf_path="/fake/path.pdf", pages=None)

    with pytest.raises(ValueError, match="Session has no pages"):
        orchestrator.run_pass1(session)


def test_run_pass2_success(orchestrator, tmp_path):
    """Test Pass 2 section analysis."""
    # Create session and run Pass 1
    pdf_path = tmp_path / "test.pdf"
    pdf_path.write_bytes(b"fake pdf")
    session = orchestrator.start_review(pdf_path)
    orchestrator.run_pass1(session)

    # Run Pass 2 with user feedback
    user_feedback = "Please focus on the methodology section"
    section_analysis = orchestrator.run_pass2(session, user_feedback)

    # Verify results
    assert len(section_analysis.sections) == 2
    assert section_analysis.sections[0].section_name == "Introduction"
    assert session.section_analysis is not None
    assert session.pass1_feedback == user_feedback


def test_run_pass2_no_structure(orchestrator):
    """Test Pass 2 without structure from Pass 1."""
    session = ReviewSession(pdf_path="/fake/path.pdf", structure=None)

    with pytest.raises(ValueError, match="Session has no structure"):
        orchestrator.run_pass2(session)


def test_run_pass3_success(orchestrator, tmp_path):
    """Test Pass 3 overall assessment."""
    # Create session and run Pass 1 and Pass 2
    pdf_path = tmp_path / "test.pdf"
    pdf_path.write_bytes(b"fake pdf")
    session = orchestrator.start_review(pdf_path)
    orchestrator.run_pass1(session)
    orchestrator.run_pass2(session, "Focus on novelty")

    # Run Pass 3 with user feedback
    user_feedback = "Compare with recent work"
    assessment = orchestrator.run_pass3(session, user_feedback)

    # Verify results
    assert assessment.overall_score == 7.5
    assert assessment.recommendation == "MinorRevision"
    assert session.overall_assessment is not None
    assert session.pass2_feedback == user_feedback


def test_run_pass3_no_section_analysis(orchestrator):
    """Test Pass 3 without section analysis from Pass 2."""
    session = ReviewSession(pdf_path="/fake/path.pdf", section_analysis=None)

    with pytest.raises(ValueError, match="Session has no section_analysis"):
        orchestrator.run_pass3(session)


def test_finalize_success(orchestrator, tmp_path):
    """Test finalizing the review."""
    # Create session and run all passes
    pdf_path = tmp_path / "test.pdf"
    pdf_path.write_bytes(b"fake pdf")
    session = orchestrator.start_review(pdf_path)
    orchestrator.run_pass1(session)
    orchestrator.run_pass2(session, "Feedback 1")
    orchestrator.run_pass3(session, "Feedback 2")

    # Finalize with user feedback
    user_feedback = "Final comments"
    review_doc = orchestrator.finalize(session, user_feedback)

    # Verify results
    assert review_doc.overall_assessment is not None
    assert review_doc.reviewer_notes is not None
    assert session.final_review is not None
    assert session.pass3_feedback == user_feedback
    assert session.completed_at is not None


def test_finalize_no_assessment(orchestrator):
    """Test finalize without overall assessment from Pass 3."""
    session = ReviewSession(pdf_path="/fake/path.pdf", overall_assessment=None)

    with pytest.raises(ValueError, match="Session has no overall_assessment"):
        orchestrator.finalize(session)


def test_full_workflow(orchestrator, tmp_path):
    """Test complete workflow: start -> Pass1 -> Pass2 -> Pass3 -> finalize."""
    # Create a dummy PDF
    pdf_path = tmp_path / "test.pdf"
    pdf_path.write_bytes(b"fake pdf content")

    # Start review
    config = ReviewConfig(model_name="claude-opus-4")
    session = orchestrator.start_review(pdf_path, config)

    # Verify initial state
    assert session.pages is not None
    assert session.structure is None

    # Run Pass 1
    structure = orchestrator.run_pass1(session)
    assert structure.title == "Test Paper"
    assert session.structure is not None

    # Run Pass 2 with user feedback
    section_analysis = orchestrator.run_pass2(session, "Check methodology carefully")
    assert len(section_analysis.sections) == 2
    assert session.section_analysis is not None
    assert session.pass1_feedback == "Check methodology carefully"

    # Run Pass 3 with user feedback
    assessment = orchestrator.run_pass3(session, "Consider practical applications")
    assert assessment.overall_score == 7.5
    assert session.overall_assessment is not None
    assert session.pass2_feedback == "Consider practical applications"

    # Finalize
    review_doc = orchestrator.finalize(session, "Please include limitations section")
    assert review_doc is not None
    assert session.final_review is not None
    assert session.pass3_feedback == "Please include limitations section"
    assert session.completed_at is not None


def test_user_feedback_propagation(orchestrator, tmp_path, mock_analyzer):
    """Test that user feedback is properly propagated through passes."""
    # Create session
    pdf_path = tmp_path / "test.pdf"
    pdf_path.write_bytes(b"fake pdf")
    session = orchestrator.start_review(pdf_path)

    # Run all passes with different feedback
    orchestrator.run_pass1(session)
    orchestrator.run_pass2(session, "Pass1 feedback")
    orchestrator.run_pass3(session, "Pass2 feedback")
    orchestrator.finalize(session, "Pass3 feedback")

    # Verify feedback was stored
    assert session.pass1_feedback == "Pass1 feedback"
    assert session.pass2_feedback == "Pass2 feedback"
    assert session.pass3_feedback == "Pass3 feedback"

    # Verify user inputs were passed to generate_assessment
    call_args = mock_analyzer.generate_assessment.call_args
    user_inputs = call_args.kwargs["user_inputs"]
    assert user_inputs is not None
    assert len(user_inputs) == 2
    assert "[After Pass 1]" in user_inputs[0]
    assert "[After Pass 2]" in user_inputs[1]

    # Verify user inputs were passed to generate_review
    call_args = orchestrator.generator.generate_review.call_args
    user_inputs = call_args.kwargs["user_inputs"]
    assert user_inputs is not None
    assert len(user_inputs) == 3
    assert "[After Pass 1]" in user_inputs[0]
    assert "[After Pass 2]" in user_inputs[1]
    assert "[After Pass 3]" in user_inputs[2]
