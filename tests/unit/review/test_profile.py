"""
Tests for auto_review.profile module.
"""

import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
import yaml

from c4.review.models import OverallAssessment, ReviewDocument, ReviewerProfile, ReviewPoint
from c4.review.profile import ReviewProfile


@pytest.fixture
def profile_manager():
    """Create a ReviewProfile instance."""
    return ReviewProfile(api_key="dummy_key")


@pytest.fixture
def sample_profile():
    """Create a sample ReviewerProfile."""
    return ReviewerProfile(
        expertise_areas=["machine learning", "computer vision"],
        review_style="balanced",
        focus_on_novelty=True,
        focus_on_reproducibility=True,
        review_points=[
            ReviewPoint(point="Missing recent citations", frequency=2, category="weakness"),
            ReviewPoint(point="Clear presentation", frequency=1, category="strength"),
        ],
    )


def test_load_profile(profile_manager, sample_profile):
    """Test loading profile from YAML file."""
    with tempfile.TemporaryDirectory() as tmpdir:
        profile_path = Path(tmpdir) / "profile.yaml"

        # Save profile first
        with open(profile_path, "w") as f:
            yaml.dump(sample_profile.model_dump(), f)

        # Load it back
        loaded_profile = profile_manager.load_profile(profile_path)

        # Assertions
        assert loaded_profile.expertise_areas == sample_profile.expertise_areas
        assert loaded_profile.review_style == sample_profile.review_style
        assert loaded_profile.focus_on_novelty == sample_profile.focus_on_novelty
        assert len(loaded_profile.review_points) == 2


def test_load_profile_not_exists(profile_manager):
    """Test loading profile when file doesn't exist returns empty profile."""
    non_existent = Path("/tmp/nonexistent_profile_12345.yaml")
    profile = profile_manager.load_profile(non_existent)

    assert isinstance(profile, ReviewerProfile)
    assert len(profile.expertise_areas) == 0
    assert profile.review_style == "balanced"


def test_save_profile(profile_manager, sample_profile):
    """Test saving profile to YAML file."""
    with tempfile.TemporaryDirectory() as tmpdir:
        profile_path = Path(tmpdir) / "profile.yaml"

        # Save profile
        profile_manager.save_profile(sample_profile, profile_path)

        # Check file exists
        assert profile_path.exists()

        # Load and verify
        with open(profile_path) as f:
            data = yaml.safe_load(f)

        assert data["expertise_areas"] == sample_profile.expertise_areas
        assert data["review_style"] == sample_profile.review_style


def test_save_profile_creates_directory(profile_manager, sample_profile):
    """Test that save_profile creates parent directory if needed."""
    with tempfile.TemporaryDirectory() as tmpdir:
        profile_path = Path(tmpdir) / "subdir" / "profile.yaml"

        # Save profile (should create subdir)
        profile_manager.save_profile(sample_profile, profile_path)

        # Check file exists
        assert profile_path.exists()
        assert profile_path.parent.exists()


def test_update_profile_increments_frequency(profile_manager):
    """Test that update_profile increments frequency of existing points."""
    profile = ReviewerProfile(
        review_points=[
            ReviewPoint(point="Missing citations", frequency=1, category="weakness"),
        ]
    )

    # Create review with same weakness
    assessment = OverallAssessment(
        dimension_scores={},
        overall_score=7.0,
        recommendation="MinorRevision",
        summary="Good paper",
        key_weaknesses=["Missing citations"],
        key_strengths=["Novel approach"],
    )
    review_doc = ReviewDocument(overall_assessment=assessment, reviewer_notes="Test review")

    # Update profile
    updated_profile = profile_manager.update_profile(profile, review_doc)

    # Check frequency was incremented
    weakness_point = next(p for p in updated_profile.review_points if "citations" in p.point)
    assert weakness_point.frequency == 2

    # Check new strength was added
    strength_points = [p for p in updated_profile.review_points if p.category == "strength"]
    assert len(strength_points) == 1
    assert strength_points[0].point == "Novel approach"


def test_update_profile_adds_new_points(profile_manager):
    """Test that update_profile adds new review points."""
    profile = ReviewerProfile()

    assessment = OverallAssessment(
        dimension_scores={},
        overall_score=7.0,
        recommendation="Accept",
        summary="Excellent paper",
        key_weaknesses=["Minor formatting issues"],
        key_strengths=["Strong theoretical foundation", "Comprehensive experiments"],
    )
    review_doc = ReviewDocument(overall_assessment=assessment, reviewer_notes="Test")

    # Update profile
    updated_profile = profile_manager.update_profile(profile, review_doc)

    # Check points were added
    assert len(updated_profile.review_points) == 3
    assert any("formatting" in p.point.lower() for p in updated_profile.review_points)
    assert any("theoretical" in p.point.lower() for p in updated_profile.review_points)


def test_get_emphasis_points(profile_manager):
    """Test getting emphasis points (frequency >= 2)."""
    profile = ReviewerProfile(
        review_points=[
            ReviewPoint(point="Point 1", frequency=3, category="weakness"),
            ReviewPoint(point="Point 2", frequency=1, category="weakness"),
            ReviewPoint(point="Point 3", frequency=2, category="strength"),
        ]
    )

    emphasis = profile_manager.get_emphasis_points(profile)

    assert len(emphasis) == 2
    assert "Point 1" in emphasis
    assert "Point 3" in emphasis
    assert "Point 2" not in emphasis


def test_get_style_guide(profile_manager, sample_profile):
    """Test getting style guide from profile."""
    style_guide = profile_manager.get_style_guide(sample_profile)

    assert style_guide.tone == "professional"


@patch("c4.review.profile.Anthropic")
def test_bootstrap_from_existing(mock_anthropic, profile_manager):
    """Test bootstrapping profile from existing reviews."""
    with tempfile.TemporaryDirectory() as tmpdir:
        review_dir = Path(tmpdir)

        # Create sample review files
        review1 = review_dir / "review1.md"
        review1.write_text(
            "논문 검토 결과\n\n강점:\n- 새로운 접근법\n\n약점:\n- 최근 연구 비교 부족\n"
        )

        review2 = review_dir / "review2.md"
        review2.write_text("리뷰\n\n강점:\n- 명확한 표현\n\n약점:\n- 최근 연구 비교 부족\n")

        # Mock API response
        mock_client = MagicMock()
        mock_anthropic.return_value = mock_client

        mock_response = MagicMock()
        mock_content = MagicMock()
        mock_content.text = """
        {
          "expertise_areas": ["machine learning"],
          "review_style": "balanced",
          "focus_on_novelty": true,
          "focus_on_reproducibility": true,
          "review_points": [
            {"point": "Lack of recent work comparison", "frequency": 2, "category": "weakness"},
            {"point": "Clear presentation", "frequency": 1, "category": "strength"}
          ],
          "style_guide": {
            "tone": "professional",
            "emphasis_points": ["novelty", "clarity"],
            "common_phrases": ["needs improvement"]
          }
        }
        """
        mock_response.content = [mock_content]
        mock_client.messages.create.return_value = mock_response

        profile_manager.client = mock_client

        # Bootstrap profile
        profile = profile_manager.bootstrap_from_existing(review_dir)

        # Assertions
        assert isinstance(profile, ReviewerProfile)
        assert "machine learning" in profile.expertise_areas
        assert profile.review_style == "balanced"
        assert len(profile.review_points) == 2


def test_bootstrap_from_existing_no_files(profile_manager):
    """Test bootstrap fails when no review files found."""
    with tempfile.TemporaryDirectory() as tmpdir:
        empty_dir = Path(tmpdir)

        with pytest.raises(ValueError) as exc_info:
            profile_manager.bootstrap_from_existing(empty_dir)

        assert "no review files" in str(exc_info.value).lower()


def test_bootstrap_from_existing_invalid_dir(profile_manager):
    """Test bootstrap fails with invalid directory."""
    invalid_dir = Path("/tmp/nonexistent_dir_12345")

    with pytest.raises(ValueError) as exc_info:
        profile_manager.bootstrap_from_existing(invalid_dir)

    assert "invalid" in str(exc_info.value).lower()


@patch("c4.review.profile.Anthropic")
def test_bootstrap_creates_profile_yaml(mock_anthropic):
    """Test that bootstrap creates profile.yaml file."""
    with tempfile.TemporaryDirectory() as tmpdir:
        review_dir = Path(tmpdir) / "reviews"
        review_dir.mkdir()

        # Create sample review file
        review1 = review_dir / "review.md"
        review1.write_text("리뷰 내용: 데이터셋 설명 부족")

        # Mock API response
        mock_client = MagicMock()
        mock_anthropic.return_value = mock_client

        mock_response = MagicMock()
        mock_content = MagicMock()
        mock_content.text = """
        {
          "expertise_areas": ["data science"],
          "review_style": "strict",
          "focus_on_novelty": true,
          "focus_on_reproducibility": true,
          "review_points": [
            {"point": "데이터셋 설명 부족", "frequency": 1, "category": "weakness"}
          ],
          "style_guide": {
            "tone": "professional",
            "emphasis_points": [],
            "common_phrases": []
          }
        }
        """
        mock_response.content = [mock_content]
        mock_client.messages.create.return_value = mock_response

        # Bootstrap and save
        profile_mgr = ReviewProfile(api_key="test_key")
        profile_mgr.client = mock_client
        profile = profile_mgr.bootstrap_from_existing(review_dir)

        # Save to file
        profile_path = Path(tmpdir) / "profile.yaml"
        profile_mgr.save_profile(profile, profile_path)

        # Verify file exists
        assert profile_path.exists()

        # Verify content
        with open(profile_path) as f:
            data = yaml.safe_load(f)

        assert data["review_style"] == "strict"
        assert len(data["review_points"]) == 1


@patch("c4.review.profile.Anthropic")
def test_bootstrap_extracts_known_points(mock_anthropic):
    """Test that bootstrap extracts known review points from existing reviews."""
    with tempfile.TemporaryDirectory() as tmpdir:
        review_dir = Path(tmpdir)

        # Create review files with known issues mentioned multiple times
        review1 = review_dir / "review1.md"
        review1.write_text(
            """
            논문 검토 결과:
            1. 데이터셋에 대한 설명이 필요합니다.
            2. 그래프의 해상도와 스타일이 통일되어 있지 않습니다.
            3. 통계적 유의성 검증이 부재합니다.
            """
        )

        review2 = review_dir / "review2.md"
        review2.write_text(
            """
            리뷰:
            - 데이터셋 설명 부족
            - 수식 변수 설명 필요
            - 그래프 해상도 문제
            """
        )

        # Mock API response with extracted patterns
        mock_client = MagicMock()
        mock_anthropic.return_value = mock_client

        mock_response = MagicMock()
        mock_content = MagicMock()
        mock_content.text = """
        {
          "expertise_areas": ["machine learning", "data analysis"],
          "review_style": "balanced",
          "focus_on_novelty": true,
          "focus_on_reproducibility": true,
          "review_points": [
            {"point": "데이터셋 설명 부족", "frequency": 2, "category": "weakness"},
            {"point": "그래프 해상도/스타일 불일치", "frequency": 2, "category": "weakness"},
            {"point": "통계적 유의성 검증 부재", "frequency": 1, "category": "weakness"},
            {"point": "수식 변수 설명 부족", "frequency": 1, "category": "weakness"}
          ],
          "style_guide": {
            "tone": "professional",
            "emphasis_points": ["data quality", "visualization"],
            "common_phrases": ["확인 바랍니다", "필요합니다"]
          }
        }
        """
        mock_response.content = [mock_content]
        mock_client.messages.create.return_value = mock_response

        # Bootstrap profile
        profile_mgr = ReviewProfile(api_key="test_key")
        profile_mgr.client = mock_client
        profile = profile_mgr.bootstrap_from_existing(review_dir)

        # Verify known points were extracted
        assert len(profile.review_points) >= 4

        # Check for specific known points
        point_texts = [p.point for p in profile.review_points]
        assert any("데이터셋 설명" in p for p in point_texts)
        assert any("그래프" in p for p in point_texts)
        assert any("통계적 유의성" in p for p in point_texts)
        assert any("수식 변수" in p for p in point_texts)

        # Check frequencies are correct
        dataset_point = next(p for p in profile.review_points if "데이터셋 설명" in p.point)
        assert dataset_point.frequency == 2

        graph_point = next(p for p in profile.review_points if "그래프" in p.point)
        assert graph_point.frequency == 2
