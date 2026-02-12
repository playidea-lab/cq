"""
Tests for auto_review.analyzer module.
"""

from unittest.mock import MagicMock

import pytest

from c4.review.analyzer import PaperAnalyzer
from c4.review.models import (
    Figure,
    OverallAssessment,
    PageImage,
    PaperStructure,
    ReviewerProfile,
    Section,
    SectionAnalysis,
    SectionEval,
    Table,
)


@pytest.fixture
def mock_page_images():
    """Create mock PageImage objects."""
    return [
        PageImage(page_number=1, image_data=b"fake_page_1", width=800, height=1200),
        PageImage(page_number=2, image_data=b"fake_page_2", width=800, height=1200),
        PageImage(page_number=3, image_data=b"fake_page_3", width=800, height=1200),
    ]


@pytest.fixture
def mock_api_response():
    """Create a mock API response."""
    mock_response = MagicMock()
    mock_content = MagicMock()
    mock_content.text = """
    Here is the extracted structure:
    ```json
    {
      "title": "Deep Learning for Image Classification",
      "authors": ["Alice Smith", "Bob Jones"],
      "sections": [
        {"title": "Introduction", "page_number": 1, "level": 1},
        {"title": "Related Work", "page_number": 2, "level": 1},
        {"title": "Background", "page_number": 2, "level": 2},
        {"title": "Methodology", "page_number": 3, "level": 1}
      ],
      "figures": [
        {"caption": "System architecture", "page_number": 3, "figure_number": "Figure 1"},
        {"caption": "Results comparison", "page_number": 5, "figure_number": "Figure 2"}
      ],
      "tables": [
        {"caption": "Dataset statistics", "page_number": 4, "table_number": "Table 1"}
      ],
      "equations_count": 5,
      "references_count": 25
    }
    ```
    """
    mock_response.content = [mock_content]
    return mock_response


@pytest.fixture
def analyzer():
    """Create a PaperAnalyzer instance with a dummy API key."""
    return PaperAnalyzer(api_key="dummy_key_for_testing")


def test_analyzer_initialization():
    """Test PaperAnalyzer initialization."""
    analyzer = PaperAnalyzer(api_key="test_key", model_name="claude-opus-4", max_tokens=2048)
    assert analyzer.max_tokens == 2048


def test_analyze_structure_returns_valid_structure(
    analyzer, mock_page_images, mock_api_response
):
    """Test that analyze_structure returns a valid PaperStructure."""
    mock_client = MagicMock()
    mock_client.messages.create.return_value = mock_api_response
    analyzer.llm.client = mock_client

    structure = analyzer.analyze_structure(mock_page_images)

    assert isinstance(structure, PaperStructure)
    assert structure.title == "Deep Learning for Image Classification"
    assert len(structure.authors) == 2
    assert "Alice Smith" in structure.authors
    assert "Bob Jones" in structure.authors

    assert len(structure.sections) == 4
    assert isinstance(structure.sections[0], Section)
    assert structure.sections[0].title == "Introduction"
    assert structure.sections[0].page_number == 1
    assert structure.sections[0].level == 1

    assert len(structure.figures) == 2
    assert isinstance(structure.figures[0], Figure)
    assert structure.figures[0].caption == "System architecture"
    assert structure.figures[0].page_number == 3

    assert len(structure.tables) == 1
    assert isinstance(structure.tables[0], Table)
    assert structure.tables[0].caption == "Dataset statistics"

    assert structure.equations_count == 5
    assert structure.references_count == 25

    assert structure.figure_count == 2
    assert structure.table_count == 1
    assert structure.has_introduction is True
    assert structure.has_methodology is True


def test_analyze_structure_long_paper(analyzer, mock_api_response):
    """Test analyze_structure with a long paper (20+ pages)."""
    long_paper_pages = [
        PageImage(page_number=i, image_data=f"fake_page_{i}".encode(), width=800, height=1200)
        for i in range(1, 26)
    ]

    mock_client = MagicMock()
    mock_client.messages.create.return_value = mock_api_response
    analyzer.llm.client = mock_client

    structure = analyzer.analyze_structure(long_paper_pages)

    assert isinstance(structure, PaperStructure)
    assert structure.title == "Deep Learning for Image Classification"

    mock_client.messages.create.assert_called_once()
    call_args = mock_client.messages.create.call_args
    content = call_args[1]["messages"][0]["content"]

    image_count = sum(1 for item in content if isinstance(item, dict) and item.get("type") == "image")
    assert image_count == 25


def test_analyze_structure_empty_pages(analyzer):
    """Test that analyze_structure raises ValueError for empty pages."""
    with pytest.raises(ValueError) as exc_info:
        analyzer.analyze_structure([])

    assert "empty" in str(exc_info.value).lower()


def test_analyze_structure_invalid_json_response(analyzer, mock_page_images):
    """Test handling of invalid JSON response from API."""
    mock_client = MagicMock()

    mock_response = MagicMock()
    mock_content = MagicMock()
    mock_content.text = "This is not JSON at all"
    mock_response.content = [mock_content]

    mock_client.messages.create.return_value = mock_response
    analyzer.llm.client = mock_client

    with pytest.raises(ValueError) as exc_info:
        analyzer.analyze_structure(mock_page_images)

    assert "json" in str(exc_info.value).lower()


def test_analyze_structure_malformed_data(analyzer, mock_page_images):
    """Test handling of malformed data in API response."""
    mock_client = MagicMock()

    mock_response = MagicMock()
    mock_content = MagicMock()
    mock_content.text = """
    {
      "title": "Test Paper",
      "sections": [
        {"title": "Missing fields"}
      ]
    }
    """
    mock_response.content = [mock_content]

    mock_client.messages.create.return_value = mock_response
    analyzer.llm.client = mock_client

    with pytest.raises(ValueError) as exc_info:
        analyzer.analyze_structure(mock_page_images)

    assert "failed to parse" in str(exc_info.value).lower()


def test_analyze_structure_minimal_valid_response(analyzer, mock_page_images):
    """Test with minimal but valid API response."""
    mock_client = MagicMock()

    mock_response = MagicMock()
    mock_content = MagicMock()
    mock_content.text = """
    {
      "title": "Minimal Paper"
    }
    """
    mock_response.content = [mock_content]

    mock_client.messages.create.return_value = mock_response
    analyzer.llm.client = mock_client

    structure = analyzer.analyze_structure(mock_page_images)

    assert structure.title == "Minimal Paper"
    assert structure.authors == []
    assert len(structure.sections) == 0
    assert len(structure.figures) == 0
    assert len(structure.tables) == 0
    assert structure.equations_count == 0
    assert structure.references_count == 0


# Pass 2 tests
@pytest.fixture
def mock_section_analysis_response():
    """Create a mock API response for section analysis."""
    mock_response = MagicMock()
    mock_content = MagicMock()
    mock_content.text = """
    Here is the section analysis:
    ```json
    {
      "sections": [
        {
          "section_name": "Introduction",
          "score": 8,
          "strengths": ["Clear motivation", "Good background"],
          "weaknesses": ["Missing recent citations"],
          "specific_comments": ["Add more recent work from 2024"]
        },
        {
          "section_name": "Methodology",
          "score": 7,
          "strengths": ["Well-structured", "Detailed description"],
          "weaknesses": ["Lacks ablation study", "Some parameters not justified"],
          "specific_comments": ["Explain why learning rate was set to 0.001"]
        }
      ],
      "overall_notes": "Sections are generally well-written but need more depth in some areas."
    }
    ```
    """
    mock_response.content = [mock_content]
    return mock_response


@pytest.fixture
def sample_structure():
    """Create a sample PaperStructure for testing."""
    return PaperStructure(
        title="Test Paper",
        authors=["Author One"],
        sections=[
            Section(title="Introduction", page_number=1, level=1),
            Section(title="Methodology", page_number=3, level=1),
            Section(title="Results", page_number=5, level=1),
        ],
    )


def test_analyze_sections_returns_evaluations(
    analyzer, sample_structure, mock_page_images, mock_section_analysis_response
):
    """Test that analyze_sections returns valid SectionAnalysis."""
    mock_client = MagicMock()
    mock_client.messages.create.return_value = mock_section_analysis_response
    analyzer.llm.client = mock_client

    analysis = analyzer.analyze_sections(sample_structure, mock_page_images)

    assert isinstance(analysis, SectionAnalysis)
    assert len(analysis.sections) == 2

    section1 = analysis.sections[0]
    assert isinstance(section1, SectionEval)
    assert section1.section_name == "Introduction"
    assert section1.score == 8
    assert "Clear motivation" in section1.strengths
    assert "Missing recent citations" in section1.weaknesses
    assert len(section1.specific_comments) > 0

    assert "well-written" in analysis.overall_notes


def test_analyze_sections_with_profile(
    analyzer, sample_structure, mock_page_images, mock_section_analysis_response
):
    """Test that analyze_sections incorporates ReviewerProfile."""
    mock_client = MagicMock()
    mock_client.messages.create.return_value = mock_section_analysis_response
    analyzer.llm.client = mock_client

    profile = ReviewerProfile(
        expertise_areas=["machine learning", "computer vision"],
        review_style="strict",
        focus_on_novelty=True,
        focus_on_reproducibility=True,
    )

    analysis = analyzer.analyze_sections(sample_structure, mock_page_images, profile)

    assert isinstance(analysis, SectionAnalysis)

    mock_client.messages.create.assert_called_once()
    call_args = mock_client.messages.create.call_args
    content = call_args[1]["messages"][0]["content"]

    text_content = [item for item in content if isinstance(item, dict) and item.get("type") == "text"]
    assert len(text_content) == 1

    prompt_text = text_content[0]["text"]
    assert "machine learning" in prompt_text
    assert "strict" in prompt_text


def test_analyze_sections_no_sections(analyzer, mock_page_images):
    """Test that analyze_sections raises error when structure has no sections."""
    empty_structure = PaperStructure(title="Empty Paper")

    with pytest.raises(ValueError) as exc_info:
        analyzer.analyze_sections(empty_structure, mock_page_images)

    assert "no sections" in str(exc_info.value).lower()


def test_analyze_sections_empty_pages(analyzer, sample_structure):
    """Test that analyze_sections raises error when pages list is empty."""
    with pytest.raises(ValueError) as exc_info:
        analyzer.analyze_sections(sample_structure, [])

    assert "empty" in str(exc_info.value).lower()


def test_analyze_sections_missing_section(
    analyzer, sample_structure, mock_page_images
):
    """Test handling when API response is missing some sections."""
    mock_client = MagicMock()

    mock_response = MagicMock()
    mock_content = MagicMock()
    mock_content.text = """
    {
      "sections": [
        {
          "section_name": "Introduction",
          "score": 8,
          "strengths": ["Good"],
          "weaknesses": [],
          "specific_comments": []
        }
      ]
    }
    """
    mock_response.content = [mock_content]

    mock_client.messages.create.return_value = mock_response
    analyzer.llm.client = mock_client

    analysis = analyzer.analyze_sections(sample_structure, mock_page_images)

    assert isinstance(analysis, SectionAnalysis)
    assert len(analysis.sections) == 1
    assert analysis.sections[0].section_name == "Introduction"


# Pass 3 tests
@pytest.fixture
def mock_assessment_response():
    """Create a mock API response for overall assessment."""
    mock_response = MagicMock()
    mock_content = MagicMock()
    mock_content.text = """
    Here is the overall assessment:
    ```json
    {
      "dimension_scores": {
        "novelty": 7.5,
        "clarity": 8.0,
        "rigor": 7.0,
        "significance": 8.5,
        "reproducibility": 6.5
      },
      "overall_score": 7.5,
      "recommendation": "MinorRevision",
      "summary": "This paper presents a novel approach with strong results, but needs minor improvements in methodology description.",
      "key_strengths": [
        "Novel approach to the problem",
        "Comprehensive experimental evaluation",
        "Clear presentation of results"
      ],
      "key_weaknesses": [
        "Limited comparison with recent work",
        "Some methodological details unclear",
        "Reproducibility could be improved"
      ]
    }
    ```
    """
    mock_response.content = [mock_content]
    return mock_response


@pytest.fixture
def sample_section_analysis():
    """Create a sample SectionAnalysis for testing."""
    return SectionAnalysis(
        sections=[
            SectionEval(
                section_name="Introduction",
                score=8,
                strengths=["Clear motivation"],
                weaknesses=["Missing citations"],
                specific_comments=["Add more background"],
            ),
            SectionEval(
                section_name="Methodology",
                score=7,
                strengths=["Well-structured"],
                weaknesses=["Some details unclear"],
                specific_comments=["Clarify parameter choices"],
            ),
        ],
        overall_notes="Good quality overall",
    )


def test_generate_assessment_returns_scores(
    analyzer, sample_section_analysis, mock_assessment_response
):
    """Test that generate_assessment returns valid scores and recommendation."""
    mock_client = MagicMock()
    mock_client.messages.create.return_value = mock_assessment_response
    analyzer.llm.client = mock_client

    assessment = analyzer.generate_assessment(sample_section_analysis)

    assert isinstance(assessment, OverallAssessment)

    assert "novelty" in assessment.dimension_scores
    assert "clarity" in assessment.dimension_scores
    assert 1.0 <= assessment.dimension_scores["novelty"] <= 10.0
    assert 1.0 <= assessment.dimension_scores["clarity"] <= 10.0

    assert 1.0 <= assessment.overall_score <= 10.0
    assert assessment.overall_score == 7.5

    assert assessment.recommendation in ["Accept", "MinorRevision", "MajorRevision", "Reject"]
    assert assessment.recommendation == "MinorRevision"

    assert isinstance(assessment.summary, str)
    assert len(assessment.summary) > 0
    assert len(assessment.key_strengths) > 0
    assert len(assessment.key_weaknesses) > 0


def test_generate_assessment_with_user_inputs(
    analyzer, sample_section_analysis, mock_assessment_response
):
    """Test that generate_assessment incorporates user inputs."""
    mock_client = MagicMock()
    mock_client.messages.create.return_value = mock_assessment_response
    analyzer.llm.client = mock_client

    user_inputs = ["How does this compare to Smith et al. 2024?", "Can the method scale to larger datasets?"]

    assessment = analyzer.generate_assessment(sample_section_analysis, user_inputs)

    assert isinstance(assessment, OverallAssessment)

    mock_client.messages.create.assert_called_once()
    call_args = mock_client.messages.create.call_args
    prompt_text = call_args[1]["messages"][0]["content"]

    assert "Smith et al. 2024" in prompt_text
    assert "scale to larger datasets" in prompt_text


def test_generate_assessment_empty_analysis(analyzer):
    """Test that generate_assessment raises error for empty analysis."""
    empty_analysis = SectionAnalysis(sections=[], overall_notes="")

    with pytest.raises(ValueError) as exc_info:
        analyzer.generate_assessment(empty_analysis)

    assert "no section evaluations" in str(exc_info.value).lower()


def test_generate_assessment_legacy_properties(
    analyzer, sample_section_analysis, mock_assessment_response
):
    """Test that legacy properties work correctly."""
    mock_client = MagicMock()
    mock_client.messages.create.return_value = mock_assessment_response
    analyzer.llm.client = mock_client

    assessment = analyzer.generate_assessment(sample_section_analysis)

    assert isinstance(assessment.novelty_score, int)
    assert isinstance(assessment.clarity_score, int)
    assert isinstance(assessment.rigor_score, int)
    assert isinstance(assessment.significance_score, int)
    assert isinstance(assessment.overall_recommendation, str)
    assert "revision" in assessment.overall_recommendation or "accept" in assessment.overall_recommendation
