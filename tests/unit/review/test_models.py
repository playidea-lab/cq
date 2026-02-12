"""
Tests for auto_review.models module.
"""

import pytest

from c4.review.models import (
    OverallAssessment,
    PageImage,
    PaperMetadata,
    PaperStructure,
    ReviewConfig,
    ReviewDocument,
    ReviewerProfile,
    SectionAnalysis,
    SectionEval,
)


def test_page_image_creation():
    """Test PageImage model creation."""
    page = PageImage(
        page_number=1,
        image_data=b"fake_image_data",
        width=800,
        height=1200
    )
    assert page.page_number == 1
    assert page.width == 800
    assert page.height == 1200


def test_paper_metadata_creation():
    """Test PaperMetadata model creation."""
    metadata = PaperMetadata(
        title="Test Paper",
        authors=["Author One", "Author Two"],
        abstract="This is a test abstract.",
        keywords=["test", "paper"]
    )
    assert metadata.title == "Test Paper"
    assert len(metadata.authors) == 2
    assert "test" in metadata.keywords


def test_paper_structure_defaults():
    """Test PaperStructure default values."""
    structure = PaperStructure(title="Test Paper")
    assert structure.has_introduction is False
    assert structure.figure_count == 0
    assert len(structure.sections) == 0
    assert structure.table_count == 0
    assert structure.equations_count == 0
    assert structure.references_count == 0


def test_section_eval_creation():
    """Test SectionEval model creation."""
    section_eval = SectionEval(
        section_name="Introduction",
        score=8,
        strengths=["Clear motivation"],
        weaknesses=["Missing related work"],
        specific_comments=["Add more background"],
    )
    assert section_eval.section_name == "Introduction"
    assert section_eval.score == 8
    assert len(section_eval.strengths) == 1
    assert len(section_eval.weaknesses) == 1


def test_section_analysis_creation():
    """Test SectionAnalysis model creation."""
    analysis = SectionAnalysis(
        sections=[
            SectionEval(
                section_name="Introduction", score=8, strengths=["Good"], weaknesses=[]
            ),
            SectionEval(section_name="Methods", score=7, strengths=[], weaknesses=["Unclear"]),
        ],
        overall_notes="Overall good quality",
    )
    assert len(analysis.sections) == 2
    assert analysis.overall_notes == "Overall good quality"


def test_overall_assessment_validation():
    """Test OverallAssessment creation and validation."""
    assessment = OverallAssessment(
        dimension_scores={"novelty": 4, "clarity": 3, "rigor": 5, "significance": 4},
        overall_score=7.5,
        recommendation="Accept",
        summary="Good paper",
        key_strengths=["Novel approach"],
        key_weaknesses=["Missing details"],
    )
    assert assessment.overall_score == 7.5
    assert assessment.recommendation == "Accept"

    # Test legacy properties
    assert assessment.novelty_score == 4
    assert assessment.clarity_score == 3
    assert assessment.overall_recommendation == "accept"

    # Test score boundaries
    with pytest.raises(Exception):  # Pydantic validation error
        OverallAssessment(
            dimension_scores={},
            overall_score=11.0,  # Invalid: > 10
            recommendation="Accept",
            summary="Test",
        )


def test_review_document_creation():
    """Test ReviewDocument model creation."""
    metadata = PaperMetadata(title="Test Paper", authors=["Author"])
    structure = PaperStructure(title="Test Paper")
    assessment = OverallAssessment(
        dimension_scores={"novelty": 3, "clarity": 3, "rigor": 3, "significance": 3},
        overall_score=6.5,
        recommendation="MinorRevision",
        summary="Needs improvement",
    )

    review = ReviewDocument(
        metadata=metadata,
        structure=structure,
        overall_assessment=assessment,
    )
    assert review.metadata.title == "Test Paper"
    assert review.structure.title == "Test Paper"
    assert review.overall_assessment.recommendation == "MinorRevision"
    assert review.overall_assessment.overall_recommendation == "minorrevision"  # Legacy property


def test_review_config_defaults():
    """Test ReviewConfig default values."""
    config = ReviewConfig()
    assert config.model_name == "claude-opus-4"
    assert config.strictness == 1.0
    assert "novelty" in config.focus_areas


def test_reviewer_profile_creation():
    """Test ReviewerProfile model creation."""
    profile = ReviewerProfile(
        expertise_areas=["machine learning", "computer vision"],
        review_style="strict",
        focus_on_novelty=True
    )
    assert "machine learning" in profile.expertise_areas
    assert profile.review_style == "strict"
