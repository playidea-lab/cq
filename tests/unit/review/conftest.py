"""
Shared pytest fixtures for auto_review tests.
"""

import os
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from c4.review.models import (
    Figure,
    OverallAssessment,
    PageImage,
    PaperStructure,
    ReviewDocument,
    Section,
    SectionAnalysis,
    SectionEval,
    Table,
)


@pytest.fixture
def sample_pdf_path():
    """Return path to a sample PDF for E2E testing."""
    # Use a real PDF from the review directory if available
    review_dir = Path(__file__).parent.parent / "review"
    pdf_files = list(review_dir.rglob("*.pdf"))

    if pdf_files:
        return pdf_files[0]

    # Fallback: return None if no PDFs found
    return None


@pytest.fixture
def skip_if_no_api_key():
    """Skip test if ANTHROPIC_API_KEY is not set."""
    if not os.getenv("ANTHROPIC_API_KEY"):
        pytest.skip("ANTHROPIC_API_KEY not set - skipping API-dependent test")


@pytest.fixture
def skip_if_no_pdf(sample_pdf_path):
    """Skip test if no sample PDF is available."""
    if sample_pdf_path is None:
        pytest.skip("No sample PDF available for E2E testing")


@pytest.fixture
def mock_page_images():
    """Create mock PageImage objects for testing."""
    return [
        PageImage(page_number=1, image_data=b"fake_page_1", width=800, height=1200),
        PageImage(page_number=2, image_data=b"fake_page_2", width=800, height=1200),
        PageImage(page_number=3, image_data=b"fake_page_3", width=800, height=1200),
    ]


@pytest.fixture
def mock_paper_structure():
    """Create a mock PaperStructure for testing."""
    return PaperStructure(
        title="Deep Learning for Image Classification",
        authors=["Alice Smith", "Bob Jones"],
        sections=[
            Section(title="Introduction", page_number=1, level=1),
            Section(title="Related Work", page_number=2, level=1),
            Section(title="Background", page_number=2, level=2),
            Section(title="Methodology", page_number=3, level=1),
            Section(title="Experiments", page_number=5, level=1),
            Section(title="Results", page_number=6, level=1),
            Section(title="Conclusion", page_number=7, level=1),
        ],
        figures=[
            Figure(caption="Model architecture", page_number=4, figure_number="Figure 1"),
            Figure(caption="Performance comparison", page_number=6, figure_number="Figure 2"),
        ],
        tables=[
            Table(caption="Dataset statistics", page_number=5, table_number="Table 1"),
        ],
        equations_count=12,
        references_count=35,
    )


@pytest.fixture
def mock_section_analysis():
    """Create a mock SectionAnalysis for testing."""
    return SectionAnalysis(
        sections=[
            SectionEval(
                section_name="Introduction",
                score=8,
                strengths=["Clear motivation", "Good background"],
                weaknesses=["Limited scope"],
                specific_comments=["Consider adding more recent work"],
            ),
            SectionEval(
                section_name="Methodology",
                score=7,
                strengths=["Novel approach", "Well-structured"],
                weaknesses=["Missing implementation details", "No complexity analysis"],
                specific_comments=["Add pseudo-code", "Discuss computational complexity"],
            ),
            SectionEval(
                section_name="Experiments",
                score=6,
                strengths=["Comprehensive datasets"],
                weaknesses=["Limited baselines", "No ablation study"],
                specific_comments=["Add more baseline comparisons"],
            ),
        ],
        overall_notes="Solid paper with good structure but needs more experimental rigor",
    )


@pytest.fixture
def mock_overall_assessment():
    """Create a mock OverallAssessment for testing."""
    return OverallAssessment(
        dimension_scores={
            "novelty": 4,
            "clarity": 4,
            "rigor": 3,
            "significance": 3,
        },
        overall_score=7.0,
        recommendation="MinorRevision",
        summary="The paper presents an interesting approach to image classification using deep learning. "
        "While the methodology is sound and results are promising, the experimental evaluation "
        "could be more rigorous with additional baselines and ablation studies.",
        key_strengths=[
            "Novel architecture design",
            "Clear presentation and writing",
            "Comprehensive dataset coverage",
        ],
        key_weaknesses=[
            "Limited baseline comparisons",
            "Missing ablation study",
            "No computational complexity analysis",
            "Insufficient implementation details",
        ],
    )


@pytest.fixture
def mock_review_document(mock_overall_assessment, mock_paper_structure, mock_section_analysis):
    """Create a mock ReviewDocument for testing."""
    korean_review = """# 논문 리뷰

## 요약
이 논문은 딥러닝을 이용한 이미지 분류에 대한 새로운 접근법을 제시합니다.

## 강점
- 새로운 아키텍처 디자인
- 명확한 표현과 작성

## 약점
- 제한된 베이스라인 비교
- 어블레이션 연구 부재

## 추천사항
Minor Revision을 권장합니다.
"""

    english_review = """# Paper Review

## Summary
This paper presents a novel approach to image classification using deep learning.

## Strengths
- Novel architecture design
- Clear presentation and writing

## Weaknesses
- Limited baseline comparisons
- Missing ablation study

## Recommendation
Minor Revision recommended.
"""

    return ReviewDocument(
        structure=mock_paper_structure,
        section_analysis=mock_section_analysis,
        overall_assessment=mock_overall_assessment,
        reviewer_notes=f"{korean_review}\n\n---\n\n[English Translation]\n\n{english_review}",
    )


@pytest.fixture
def mock_anthropic_client():
    """Create a mock Anthropic client for testing (legacy - use mock_llm instead)."""
    mock_client = MagicMock()

    call_count = [0]

    def create_response(**kwargs):
        call_count[0] += 1
        mock_response = MagicMock()
        mock_content = MagicMock()

        if call_count[0] == 1:
            mock_content.text = """
            {
              "title": "Deep Learning for Image Classification",
              "authors": ["Alice Smith", "Bob Jones"],
              "sections": [
                {"title": "Introduction", "page_number": 1, "level": 1},
                {"title": "Methodology", "page_number": 3, "level": 1}
              ],
              "figures": [{"caption": "Model architecture", "page_number": 4, "figure_number": "Figure 1"}],
              "tables": [{"caption": "Results", "page_number": 5, "table_number": "Table 1"}],
              "equations_count": 12,
              "references_count": 35
            }
            """
        elif call_count[0] == 2:
            mock_content.text = """
            {
              "sections": [
                {
                  "section_name": "Introduction",
                  "score": 8,
                  "strengths": ["Clear motivation"],
                  "weaknesses": ["Limited scope"],
                  "specific_comments": ["Add more recent work"]
                },
                {
                  "section_name": "Methodology",
                  "score": 7,
                  "strengths": ["Novel approach"],
                  "weaknesses": ["Missing details"],
                  "specific_comments": ["Add implementation details"]
                }
              ],
              "overall_notes": "Good structure overall"
            }
            """
        elif call_count[0] == 3:
            mock_content.text = """
            {
              "dimension_scores": {"novelty": 4, "clarity": 4, "rigor": 3, "significance": 3},
              "overall_score": 7.0,
              "recommendation": "MinorRevision",
              "summary": "Interesting approach with minor issues",
              "key_strengths": ["Novel design", "Clear writing"],
              "key_weaknesses": ["Limited baselines", "Missing ablation"]
            }
            """
        else:
            mock_content.text = "Generated review content in Korean\n\n---\n\nEnglish translation"

        mock_response.content = [mock_content]
        return mock_response

    mock_client.messages.create = MagicMock(side_effect=create_response)

    return mock_client


