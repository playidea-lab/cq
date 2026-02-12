"""
End-to-end integration tests for the full review pipeline.
"""

import os
from unittest.mock import patch

import pytest

from c4.review.converter import PDFConverter
from c4.review.generator import ReviewGenerator
from c4.review.llm import create_llm
from c4.review.models import ReviewConfig, ReviewerProfile
from c4.review.orchestrator import ReviewOrchestrator
from c4.review.profile import ReviewProfile


def _make_mock_orchestrator(mock_anthropic_client, mock_page_images, converter=None):
    """Helper: create orchestrator with mock LLM client injected."""
    mock_converter = converter or PDFConverter()
    orchestrator = ReviewOrchestrator(api_key="test_key", converter=mock_converter)
    orchestrator.analyzer.llm.client = mock_anthropic_client
    orchestrator.generator.llm.client = mock_anthropic_client
    return orchestrator, mock_converter


def test_e2e_mock_pipeline(
    tmp_path, mock_anthropic_client, mock_page_images, mock_paper_structure, mock_section_analysis, mock_overall_assessment
):
    """
    Test the full pipeline with mocked components.

    This test validates the entire workflow without making real API calls:
    PDF conversion -> Pass 1 -> Pass 2 -> Pass 3 -> Review generation
    """
    pdf_path = tmp_path / "test_paper.pdf"
    pdf_path.write_bytes(b"%PDF-1.4\nfake pdf content")

    mock_converter = PDFConverter()
    with patch.object(mock_converter, "convert", return_value=mock_page_images):
        orchestrator = ReviewOrchestrator(api_key="test_key", converter=mock_converter)
        orchestrator.analyzer.llm.client = mock_anthropic_client
        orchestrator.generator.llm.client = mock_anthropic_client

        # Step 1: Start review
        config = ReviewConfig(model_name="claude-opus-4")
        session = orchestrator.start_review(pdf_path, config)

        assert session is not None
        assert session.pdf_path == str(pdf_path)
        assert session.pages is not None
        assert len(session.pages) > 0

        # Step 2: Run Pass 1 - Structure analysis
        structure = orchestrator.run_pass1(session)

        assert structure is not None
        assert structure.title != ""
        assert len(structure.sections) >= 1
        assert len(structure.figures) >= 0
        assert session.structure is not None

        # Step 3: Run Pass 2 - Section analysis
        user_feedback_pass1 = "Focus on the methodology section"
        section_analysis = orchestrator.run_pass2(session, user_feedback_pass1)

        assert section_analysis is not None
        assert len(section_analysis.sections) >= 1
        assert all(1 <= s.score <= 10 for s in section_analysis.sections)
        assert session.section_analysis is not None
        assert session.pass1_feedback == user_feedback_pass1

        # Step 4: Run Pass 3 - Overall assessment
        user_feedback_pass2 = "Check the experimental rigor"
        assessment = orchestrator.run_pass3(session, user_feedback_pass2)

        assert assessment is not None
        assert 1.0 <= assessment.overall_score <= 10.0
        assert assessment.recommendation in ["Accept", "MinorRevision", "MajorRevision", "Reject"]
        assert len(assessment.key_strengths) > 0
        assert len(assessment.key_weaknesses) > 0
        assert session.overall_assessment is not None
        assert session.pass2_feedback == user_feedback_pass2

        # Step 5: Finalize and generate review
        user_feedback_pass3 = "Emphasize reproducibility concerns"
        review_doc = orchestrator.finalize(session, user_feedback_pass3)

        assert review_doc is not None
        assert review_doc.overall_assessment is not None
        assert review_doc.reviewer_notes is not None
        assert len(review_doc.reviewer_notes) > 0
        assert session.final_review is not None
        assert session.pass3_feedback == user_feedback_pass3
        assert session.completed_at is not None

        # Step 6: Save review and verify
        output_dir = tmp_path / "output"
        output_dir.mkdir()

        generator = ReviewGenerator(api_key="test_key")
        review_path = generator.save_review(review_doc, output_dir)

        assert review_path.exists()
        assert review_path.name == "review.md"

        content = review_path.read_text()
        assert len(content) > 0


def test_e2e_profile_update(tmp_path, mock_anthropic_client, mock_page_images):
    """
    Test that the profile is updated after a review.
    """
    pdf_path = tmp_path / "test_paper.pdf"
    pdf_path.write_bytes(b"%PDF-1.4\nfake pdf")

    profile_path = tmp_path / "profile.yaml"

    initial_profile = ReviewerProfile(
        expertise_areas=["machine learning"],
        review_style="balanced",
    )

    profile_mgr = ReviewProfile(api_key="test_key")
    profile_mgr.save_profile(initial_profile, profile_path)

    mock_converter = PDFConverter()
    with patch.object(mock_converter, "convert", return_value=mock_page_images):
        orchestrator = ReviewOrchestrator(api_key="test_key", converter=mock_converter)
        orchestrator.analyzer.llm.client = mock_anthropic_client
        orchestrator.generator.llm.client = mock_anthropic_client

        session = orchestrator.start_review(pdf_path)
        orchestrator.run_pass1(session)
        orchestrator.run_pass2(session)
        orchestrator.run_pass3(session)
        review_doc = orchestrator.finalize(session)

        updated_profile = profile_mgr.update_profile(initial_profile, review_doc)
        profile_mgr.save_profile(updated_profile, profile_path)

        assert profile_path.exists()
        loaded_profile = profile_mgr.load_profile(profile_path)

        assert len(loaded_profile.review_points) > 0


def test_e2e_with_config_options(tmp_path, mock_anthropic_client, mock_page_images):
    """
    Test E2E pipeline with different ReviewConfig options.
    """
    pdf_path = tmp_path / "test.pdf"
    pdf_path.write_bytes(b"%PDF-1.4\nfake")

    config = ReviewConfig(
        model_name="claude-opus-4",
        max_pages=10,
        focus_areas=["novelty", "reproducibility"],
        strictness=1.5,
    )

    mock_converter = PDFConverter()
    with patch.object(mock_converter, "convert", return_value=mock_page_images):
        orchestrator = ReviewOrchestrator(api_key="test_key", converter=mock_converter)
        orchestrator.analyzer.llm.client = mock_anthropic_client
        orchestrator.generator.llm.client = mock_anthropic_client

        session = orchestrator.start_review(pdf_path, config)

        assert session.config.model_name == "claude-opus-4"
        assert session.config.max_pages == 10
        assert "novelty" in session.config.focus_areas
        assert "reproducibility" in session.config.focus_areas
        assert session.config.strictness == 1.5

        orchestrator.run_pass1(session)
        orchestrator.run_pass2(session)
        orchestrator.run_pass3(session)
        review_doc = orchestrator.finalize(session)

        assert review_doc is not None


def test_e2e_user_feedback_flow(tmp_path, mock_anthropic_client, mock_page_images):
    """
    Test that user feedback flows through all passes correctly.
    """
    pdf_path = tmp_path / "test.pdf"
    pdf_path.write_bytes(b"%PDF-1.4\nfake")

    mock_converter = PDFConverter()
    with patch.object(mock_converter, "convert", return_value=mock_page_images):
        orchestrator = ReviewOrchestrator(api_key="test_key", converter=mock_converter)
        orchestrator.analyzer.llm.client = mock_anthropic_client
        orchestrator.generator.llm.client = mock_anthropic_client

        session = orchestrator.start_review(pdf_path)

        orchestrator.run_pass1(session)
        feedback1 = "Focus on novelty"
        orchestrator.run_pass2(session, feedback1)

        assert session.pass1_feedback == feedback1

        feedback2 = "Check experimental rigor"
        orchestrator.run_pass3(session, feedback2)

        assert session.pass2_feedback == feedback2

        feedback3 = "Emphasize limitations"
        orchestrator.finalize(session, feedback3)

        assert session.pass3_feedback == feedback3

        assert session.pass1_feedback is not None
        assert session.pass2_feedback is not None
        assert session.pass3_feedback is not None


@pytest.mark.skipif(
    not (os.getenv("ANTHROPIC_API_KEY") or os.getenv("OPENAI_API_KEY")),
    reason="No API key set (ANTHROPIC_API_KEY or OPENAI_API_KEY) - real API test skipped",
)
def test_e2e_full_pipeline_real_api(sample_pdf_path, skip_if_no_pdf, tmp_path):
    """
    E2E test with real PDF and real API calls.

    Supports both Anthropic and OpenAI backends via create_llm() auto-detection.
    """
    assert sample_pdf_path is not None, "No sample PDF available"
    assert sample_pdf_path.exists(), f"PDF not found: {sample_pdf_path}"

    llm = create_llm()
    orchestrator = ReviewOrchestrator(
        converter=PDFConverter(),
        analyzer=__import__("c4.review.analyzer", fromlist=["PaperAnalyzer"]).PaperAnalyzer(llm=llm),
        generator=ReviewGenerator(llm=llm),
    )

    config = ReviewConfig(
        max_pages=5,
    )

    try:
        session = orchestrator.start_review(sample_pdf_path, config)

        assert session.pages is not None
        assert len(session.pages) > 0
        print(f"  PDF converted: {len(session.pages)} pages")

        structure = orchestrator.run_pass1(session)
        assert structure.title != ""
        assert len(structure.sections) >= 1
        print(f"  Pass 1 complete: {len(structure.sections)} sections found")

        section_analysis = orchestrator.run_pass2(session, "Focus on methodology")
        assert len(section_analysis.sections) >= 1
        assert all(1 <= s.score <= 10 for s in section_analysis.sections)
        print(f"  Pass 2 complete: {len(section_analysis.sections)} sections analyzed")

        assessment = orchestrator.run_pass3(session, "Check rigor")
        assert 1.0 <= assessment.overall_score <= 10.0
        assert assessment.recommendation in ["Accept", "MinorRevision", "MajorRevision", "Reject"]
        print(f"  Pass 3 complete: {assessment.recommendation} (score: {assessment.overall_score})")

        review_doc = orchestrator.finalize(session)
        assert review_doc is not None
        assert review_doc.reviewer_notes is not None
        assert len(review_doc.reviewer_notes) > 100
        print(f"  Review generated: {len(review_doc.reviewer_notes)} characters")

        output_dir = tmp_path / "review_output"
        output_dir.mkdir()

        generator = ReviewGenerator(llm=llm)
        review_path = generator.save_review(review_doc, output_dir)

        assert review_path.exists()
        review_path.read_text()
        print(f"  Review saved to: {review_path}")

        profile_path = tmp_path / "profile.yaml"
        profile_mgr = ReviewProfile(api_key="dummy")
        initial_profile = ReviewerProfile()

        updated_profile = profile_mgr.update_profile(initial_profile, review_doc)
        profile_mgr.save_profile(updated_profile, profile_path)

        assert profile_path.exists()
        loaded_profile = profile_mgr.load_profile(profile_path)
        assert len(loaded_profile.review_points) > 0
        print(f"  Profile updated: {len(loaded_profile.review_points)} patterns learned")

        print("\n  Full E2E pipeline completed successfully!")

    except Exception as e:
        pytest.fail(f"E2E test failed with real API: {e}")
