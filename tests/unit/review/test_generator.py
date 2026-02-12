"""
Tests for auto_review.generator module.
"""

import tempfile
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from c4.review.generator import ReviewGenerator
from c4.review.models import OverallAssessment, ReviewConfig, ReviewDocument


@pytest.fixture
def sample_assessment():
    """Create a sample OverallAssessment for testing."""
    return OverallAssessment(
        dimension_scores={
            "novelty": 7.5,
            "clarity": 8.0,
            "rigor": 7.0,
            "significance": 8.5,
            "reproducibility": 6.5,
        },
        overall_score=7.5,
        recommendation="MinorRevision",
        summary="Good paper with novel approach but needs minor improvements.",
        key_strengths=[
            "Novel approach to problem",
            "Comprehensive experiments",
            "Clear presentation",
        ],
        key_weaknesses=[
            "Limited comparison with recent work",
            "Some methodological details unclear",
            "Reproducibility concerns",
        ],
    )


@pytest.fixture
def generator():
    """Create a ReviewGenerator instance."""
    return ReviewGenerator(api_key="dummy_key")


@pytest.fixture
def mock_korean_review():
    """Mock Korean review text."""
    return """안녕하세요, 논문을 검토해주셔서 감사합니다.

## 논문 요약
본 논문은 딥러닝을 활용한 새로운 접근법을 제시합니다.

## 강점
1. 새로운 방법론
2. 포괄적인 실험
3. 명확한 표현

## 세부 코멘트
1. 최근 연구와의 비교가 필요합니다.
2. 일부 파라미터 선택에 대한 설명이 부족합니다.

## 종합 평가
Minor Revision을 권장합니다.
"""


@pytest.fixture
def mock_english_review():
    """Mock English review text."""
    return """Hello, thank you for your paper submission.

## Summary
This paper presents a novel deep learning approach.

## Strengths
1. Novel methodology
2. Comprehensive experiments
3. Clear presentation

## Detailed Comments
1. Comparison with recent work is needed.
2. Some parameter choices lack explanation.

## Overall Assessment
Minor Revision is recommended.
"""


def _setup_generator_mock(generator, mock_korean_review, mock_english_review):
    """Helper: set up mock client for generator tests."""
    mock_client = MagicMock()

    mock_review_response = MagicMock()
    mock_review_content = MagicMock()
    mock_review_content.text = mock_korean_review
    mock_review_response.content = [mock_review_content]

    mock_translation_response = MagicMock()
    mock_translation_content = MagicMock()
    mock_translation_content.text = mock_english_review
    mock_translation_response.content = [mock_translation_content]

    mock_client.messages.create.side_effect = [mock_review_response, mock_translation_response]
    generator.llm.client = mock_client
    return mock_client


def test_generate_review_has_all_sections(
    generator, sample_assessment, mock_korean_review, mock_english_review
):
    """Test that generated review has all required sections."""
    _setup_generator_mock(generator, mock_korean_review, mock_english_review)

    review = generator.generate_review(sample_assessment)

    assert isinstance(review, ReviewDocument)
    assert review.overall_assessment == sample_assessment
    assert isinstance(review.reviewer_notes, str)
    assert len(review.reviewer_notes) > 0

    assert "논문" in review.reviewer_notes
    assert "강점" in review.reviewer_notes

    assert "[English Translation]" in review.reviewer_notes


def test_generate_review_korean_and_english(
    generator, sample_assessment, mock_korean_review, mock_english_review
):
    """Test that review contains both Korean and English."""
    _setup_generator_mock(generator, mock_korean_review, mock_english_review)

    review = generator.generate_review(sample_assessment)

    assert "논문" in review.reviewer_notes or "강점" in review.reviewer_notes
    assert "Summary" in review.reviewer_notes or "Strengths" in review.reviewer_notes


def test_generate_review_with_user_inputs(
    generator, sample_assessment, mock_korean_review, mock_english_review
):
    """Test that user inputs are incorporated into the review."""
    mock_client = _setup_generator_mock(generator, mock_korean_review, mock_english_review)

    user_inputs = ["How does this compare to Smith et al.?", "Can this scale to larger datasets?"]

    generator.generate_review(sample_assessment, user_inputs=user_inputs)

    assert mock_client.messages.create.call_count == 2
    first_call_args = mock_client.messages.create.call_args_list[0]
    prompt = first_call_args[1]["messages"][0]["content"]

    assert "Smith et al." in prompt or "토론" in prompt


def test_generate_review_with_config(
    generator, sample_assessment, mock_korean_review, mock_english_review
):
    """Test that config is used in review generation."""
    _setup_generator_mock(generator, mock_korean_review, mock_english_review)

    config = ReviewConfig(strictness=2.0, focus_areas=["novelty", "reproducibility"])

    review = generator.generate_review(sample_assessment, config=config)

    assert isinstance(review, ReviewDocument)


def test_save_review_creates_file(generator, sample_assessment):
    """Test that save_review creates a file."""
    with tempfile.TemporaryDirectory() as tmpdir:
        output_dir = Path(tmpdir)

        review = ReviewDocument(
            overall_assessment=sample_assessment,
            reviewer_notes="Test review content\n\n[English Translation]\n\nTest English content",
        )

        output_file = generator.save_review(review, output_dir)

        assert output_file.exists()
        assert output_file.name == "review.md"
        assert output_file.parent == output_dir

        content = output_file.read_text(encoding="utf-8")
        assert "Test review content" in content
        assert "[English Translation]" in content


def test_save_review_invalid_dir(generator, sample_assessment):
    """Test that save_review raises error for invalid directory."""
    review = ReviewDocument(
        overall_assessment=sample_assessment, reviewer_notes="Test content"
    )

    invalid_dir = Path("/tmp/nonexistent_directory_12345")

    with pytest.raises(ValueError) as exc_info:
        generator.save_review(review, invalid_dir)

    assert "does not exist" in str(exc_info.value).lower()


def test_save_review_not_a_directory(generator, sample_assessment):
    """Test that save_review raises error when path is not a directory."""
    with tempfile.NamedTemporaryFile() as tmpfile:
        review = ReviewDocument(
            overall_assessment=sample_assessment, reviewer_notes="Test content"
        )

        not_a_dir = Path(tmpfile.name)

        with pytest.raises(ValueError) as exc_info:
            generator.save_review(review, not_a_dir)

        assert "not a directory" in str(exc_info.value).lower()
