"""Tests for run_write_learning() in c2.persona."""

from pathlib import Path

import pytest

from c4.c2.persona import run_write_learning


@pytest.fixture
def tmp_draft(tmp_path: Path) -> Path:
    draft = tmp_path / "draft_v1.md"
    draft.write_text(
        "# Introduction\n\n"
        "본 논문에서는 새로운 방법을 제안합니다.\n"
        "이 방법은 기존 접근법에 비해 우수한 성능을 보입니다.\n\n"
        "## Method\n\n"
        "제안하는 방법은 다음과 같습니다.\n"
        "알고리즘은 세 단계로 구성되어 있습니다.\n",
        encoding="utf-8",
    )
    return draft


@pytest.fixture
def tmp_final_shorter(tmp_path: Path) -> Path:
    final = tmp_path / "draft_v2.md"
    final.write_text(
        "# Introduction\n\n"
        "새로운 방법을 제안한다.\n\n"
        "## Method\n\n"
        "제안 방법은 세 단계로 구성된다.\n",
        encoding="utf-8",
    )
    return final


@pytest.fixture
def tmp_final_same(tmp_path: Path) -> Path:
    final = tmp_path / "draft_v2.md"
    final.write_text(
        "# Introduction\n\n"
        "본 논문에서는 새로운 방법을 제안합니다.\n"
        "이 방법은 기존 접근법에 비해 우수한 성능을 보입니다.\n\n"
        "## Method\n\n"
        "제안하는 방법은 다음과 같습니다.\n"
        "알고리즘은 세 단계로 구성되어 있습니다.\n",
        encoding="utf-8",
    )
    return final


class TestRunWriteLearning:
    def test_returns_profile_diff(self, tmp_draft, tmp_final_shorter):
        diff = run_write_learning(tmp_draft, tmp_final_shorter)
        assert diff is not None
        assert hasattr(diff, "summary")
        assert hasattr(diff, "new_patterns")

    def test_detects_shortening(self, tmp_draft, tmp_final_shorter):
        diff = run_write_learning(tmp_draft, tmp_final_shorter)
        descriptions = [p.description for p in diff.new_patterns]
        has_shorten = any("shortened" in d for d in descriptions)
        assert has_shorten, f"Expected shortening pattern, got: {descriptions}"

    def test_write_domain_tag(self, tmp_draft, tmp_final_shorter):
        diff = run_write_learning(tmp_draft, tmp_final_shorter)
        for p in diff.new_patterns:
            assert p.description.startswith("[write]"), (
                f"Pattern should be tagged with [write]: {p.description}"
            )

    def test_no_changes(self, tmp_draft, tmp_final_same):
        diff = run_write_learning(tmp_draft, tmp_final_same)
        assert diff.summary == "변경 없음" or len(diff.new_patterns) == 0

    def test_default_profile_path(self, tmp_draft, tmp_final_shorter):
        # Should not raise even when .c2/profile.yaml doesn't exist
        # (auto_apply=False so no file write)
        diff = run_write_learning(tmp_draft, tmp_final_shorter, auto_apply=False)
        assert diff is not None
