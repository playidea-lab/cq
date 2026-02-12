"""Tests for c2 package."""

from datetime import date
from pathlib import Path

import pytest

from c4.c2.models import (
    Artifact,
    ChangeEntry,
    ClaimEvidence,
    EditPattern,
    ProfileDiff,
    ProjectType,
    ReadingNote,
    ReadingPass,
    Relevance,
    ReviewRecord,
    ReviewReflectionStatus,
    ReviewType,
    SectionState,
    SectionStatus,
    Source,
    SourceStatus,
    SourceType,
    WorkspaceState,
)
from c4.c2.workspace import create_workspace, parse_workspace, render_workspace, _enum_by_value
from c4.c2.profile import (
    get_preference,
    get_learned_patterns,
    load_profile,
    save_profile,
    update_learned_patterns,
)
from c4.c2.persona import PersonaLearner, run_review_learning


class TestModels:
    """Test c2 Pydantic models."""

    def test_source_creation(self):
        src = Source(id="smith2024", title="A Study on X")
        assert src.id == "smith2024"
        assert src.relevance == Relevance.MEDIUM
        assert src.status == SourceStatus.DISCOVERED

    def test_reading_note(self):
        note = ReadingNote(
            source_id="smith2024",
            passes=[
                ReadingPass(
                    pass_number=1,
                    summary="Interesting approach",
                    claims=["Claim A", "Claim B"],
                )
            ],
        )
        assert note.max_pass == 1
        assert len(note.passes[0].claims) == 2

    def test_workspace_state(self):
        ws = WorkspaceState(
            project_name="test_project",
            project_type=ProjectType.ACADEMIC_PAPER,
            goal="Test goal",
        )
        assert ws.project_name == "test_project"
        assert ws.sources == []
        assert ws.sections == []

    def test_edit_pattern(self):
        pat = EditPattern(category="tone", description="Softened language")
        assert pat.frequency == 1

    def test_profile_diff(self):
        diff = ProfileDiff(
            tone_updates=["softer tone"],
            summary="1 tone pattern",
        )
        assert len(diff.tone_updates) == 1

    def test_artifact(self):
        art = Artifact(name="draft_v1.pdf", type="pdf")
        assert art.version == "v1"


class TestWorkspace:
    """Test workspace management."""

    def test_create_workspace_academic(self):
        ws = create_workspace(
            project_name="my_paper",
            project_type=ProjectType.ACADEMIC_PAPER,
            goal="Write a paper on X",
        )
        assert ws.project_name == "my_paper"
        section_names = [s.name for s in ws.sections]
        assert "abstract" in section_names
        assert "method" in section_names
        assert len(ws.sections) == 7

    def test_create_workspace_proposal(self):
        ws = create_workspace(
            project_name="my_proposal",
            project_type=ProjectType.PROPOSAL,
            goal="Propose project Y",
        )
        section_names = [s.name for s in ws.sections]
        assert "executive_summary" in section_names
        assert "budget" in section_names

    def test_render_workspace(self):
        ws = create_workspace(
            project_name="test",
            project_type=ProjectType.ACADEMIC_PAPER,
            goal="Test",
        )
        ws.sources.append(
            Source(id="s1", title="Paper A", relevance=Relevance.HIGH)
        )
        ws.open_questions.append("Is X valid?")

        md = render_workspace(ws)
        assert "# c2 Workspace - test" in md
        assert "Paper A" in md
        assert "Is X valid?" in md
        assert "academic_paper" in md

    def test_save_workspace(self, tmp_path):
        ws = create_workspace("test", ProjectType.PROPOSAL, "Test")
        from c4.c2.workspace import save_workspace

        path = save_workspace(ws, tmp_path / "projects" / "test")
        assert path.exists()
        content = path.read_text()
        assert "proposal" in content


class TestProfile:
    """Test profile management."""

    def test_load_save_profile(self, tmp_path):
        profile_path = tmp_path / "profile.yaml"
        data = {
            "user": {"name": "Test User"},
            "preferences": {"discover": {"max_queue_size": 10}},
        }
        save_profile(data, profile_path)
        loaded = load_profile(profile_path)
        assert loaded["user"]["name"] == "Test User"

    def test_get_preference(self):
        profile = {
            "preferences": {
                "discover": {"max_queue_size": 15},
                "write": {"language": "english"},
            }
        }
        assert get_preference(profile, "discover", "max_queue_size") == 15
        assert get_preference(profile, "write", "language") == "english"
        assert get_preference(profile, "read", "missing", "default") == "default"

    def test_update_learned_patterns(self):
        profile = {"learned_patterns": {"tone_preferences": ["existing"]}}
        updated = update_learned_patterns(
            profile,
            tone_preferences=["new_tone"],
            structure_preferences=["new_struct"],
        )
        patterns = updated["learned_patterns"]
        assert "existing" in patterns["tone_preferences"]
        assert "new_tone" in patterns["tone_preferences"]
        assert "new_struct" in patterns["structure_preferences"]
        assert patterns["last_updated"] == str(date.today())

    def test_load_missing_profile(self, tmp_path):
        missing = tmp_path / "nonexistent.yaml"
        result = load_profile(missing)
        assert result == {}


class TestPersona:
    """Test persona learning."""

    def test_analyze_edits_tone(self):
        original = "이 수식에 오류가 있습니다. 잘못된 부분을 수정해야 합니다."
        edited = "이 수식에 대해 확인이 필요합니다. 확인 바랍니다."

        patterns = PersonaLearner.analyze_edits(original, edited)
        assert len(patterns) > 0
        categories = [p.category for p in patterns]
        assert "tone" in categories or "wording" in categories

    def test_analyze_edits_conciseness(self):
        original = "A " * 100
        edited = "A " * 20

        patterns = PersonaLearner.analyze_edits(original, edited)
        has_structure = any(p.category == "structure" for p in patterns)
        assert has_structure

    def test_suggest_profile_updates(self):
        patterns = [
            EditPattern(category="tone", description="Softened language"),
            EditPattern(category="structure", description="Shortened text"),
        ]
        diff = PersonaLearner.suggest_profile_updates(patterns)
        assert len(diff.tone_updates) == 1
        assert len(diff.structure_updates) == 1
        assert "톤 패턴" in diff.summary

    def test_apply_profile_diff(self, tmp_path):
        profile_path = tmp_path / "profile.yaml"
        save_profile({"learned_patterns": {}}, profile_path)

        diff = ProfileDiff(
            tone_updates=["use questioning tone"],
            structure_updates=["shorter minor comments"],
            new_patterns=[],
            summary="test",
        )
        PersonaLearner.apply_profile_diff(profile_path, diff)

        updated = load_profile(profile_path)
        patterns = updated.get("learned_patterns", {})
        assert "use questioning tone" in patterns.get("tone_preferences", [])


class TestParseWorkspace:
    """Test parse_workspace() — markdown → WorkspaceState."""

    def test_roundtrip_empty_workspace(self):
        """create→render→parse→render should produce identical output."""
        ws = create_workspace("empty_test", ProjectType.ACADEMIC_PAPER, "Test goal")
        md1 = render_workspace(ws)
        parsed = parse_workspace(md1)
        md2 = render_workspace(parsed)
        assert md1 == md2

    def test_roundtrip_with_sources(self):
        """Sources survive a render→parse→render roundtrip (lossy id ok)."""
        ws = create_workspace("src_test", ProjectType.PROPOSAL, "Source test")
        ws.sources.append(
            Source(id="smith2024", title="Study on X", type=SourceType.PAPER,
                   relevance=Relevance.HIGH, status=SourceStatus.COMPLETED, notes="Good paper")
        )
        ws.sources.append(
            Source(id="jones2023", title="Survey Y", type=SourceType.WEB,
                   relevance=Relevance.LOW, status=SourceStatus.READING, notes="Skimmed")
        )
        md = render_workspace(ws)
        parsed = parse_workspace(md)

        assert len(parsed.sources) == 2
        assert parsed.sources[0].title == "Study on X"
        assert parsed.sources[0].relevance == Relevance.HIGH
        assert parsed.sources[0].status == SourceStatus.COMPLETED
        assert parsed.sources[1].type == SourceType.WEB
        assert parsed.sources[1].status == SourceStatus.READING

    def test_roundtrip_with_reviews(self):
        """ReviewRecord enum values survive roundtrip."""
        ws = create_workspace("rev_test", ProjectType.ACADEMIC_PAPER, "Review test")
        ws.reviews.append(
            ReviewRecord(
                date=date(2026, 2, 8),
                reviewer="self",
                type=ReviewType.EXTERNAL,
                summary="Major 5건",
                reflection_status=ReviewReflectionStatus.DONE,
            )
        )
        md = render_workspace(ws)
        parsed = parse_workspace(md)

        assert len(parsed.reviews) == 1
        assert parsed.reviews[0].type == ReviewType.EXTERNAL
        assert parsed.reviews[0].reflection_status == ReviewReflectionStatus.DONE
        assert parsed.reviews[0].date == date(2026, 2, 8)
        assert parsed.reviews[0].summary == "Major 5건"

    def test_parse_real_workspace(self):
        """Parse the actual 25-TIE-6582 workspace file."""
        ws_path = Path(__file__).parent.parent / "projects" / "25-TIE-6582" / "c2_workspace.md"
        if not ws_path.exists():
            pytest.skip("25-TIE-6582 workspace not available")

        md = ws_path.read_text(encoding="utf-8")
        ws = parse_workspace(md)

        assert ws.project_name == "25-TIE-6582"
        assert ws.project_type == ProjectType.ACADEMIC_PAPER
        assert "리뷰" in ws.goal
        assert len(ws.sources) == 4
        assert len(ws.reading_notes) == 1
        assert len(ws.reviews) == 1
        assert len(ws.claim_evidence) == 5
        assert len(ws.changelog) == 3
        assert ws.created_at == date(2026, 2, 8)

    def test_parse_empty_tables(self):
        """Empty tables parse to empty lists."""
        ws = create_workspace("empty_tbl", ProjectType.REPORT, "Empty test")
        md = render_workspace(ws)
        parsed = parse_workspace(md)

        assert parsed.sources == []
        assert parsed.reading_notes == []
        assert parsed.reviews == []
        assert parsed.claim_evidence == []
        assert parsed.changelog == []

    def test_parse_open_questions_empty(self):
        """A single '-' in open questions → empty list."""
        ws = create_workspace("oq_test", ProjectType.ACADEMIC_PAPER, "OQ test")
        # Default: no open_questions → renders as "-"
        md = render_workspace(ws)
        parsed = parse_workspace(md)
        assert parsed.open_questions == []

    def test_parse_open_questions_with_items(self):
        """Multiple bullet items are parsed correctly."""
        ws = create_workspace("oq2", ProjectType.ACADEMIC_PAPER, "OQ test")
        ws.open_questions = ["Is X valid?", "Check Y"]
        md = render_workspace(ws)
        parsed = parse_workspace(md)
        assert parsed.open_questions == ["Is X valid?", "Check Y"]

    def test_enum_by_value(self):
        """Korean enum reverse mapping works."""
        assert _enum_by_value(SourceStatus, "발견") == SourceStatus.DISCOVERED
        assert _enum_by_value(SourceStatus, "읽기중") == SourceStatus.READING
        assert _enum_by_value(SourceStatus, "완료") == SourceStatus.COMPLETED
        assert _enum_by_value(SectionStatus, "미시작") == SectionStatus.NOT_STARTED
        assert _enum_by_value(SectionStatus, "초안") == SectionStatus.DRAFTING
        assert _enum_by_value(ReviewReflectionStatus, "미반영") == ReviewReflectionStatus.PENDING
        assert _enum_by_value(ReviewReflectionStatus, "반영완료") == ReviewReflectionStatus.DONE
        # Unknown value → default
        assert _enum_by_value(SourceStatus, "unknown", SourceStatus.DISCOVERED) == SourceStatus.DISCOVERED


class TestReviewLearning:
    """Test run_review_learning() convenience function."""

    def test_run_review_learning_basic(self, tmp_path):
        """Detect tone changes between draft and final."""
        draft = tmp_path / ".draft.md"
        final = tmp_path / "리뷰의견.md"
        draft.write_text("이 수식에 오류가 있습니다. 잘못된 접근입니다.\n", encoding="utf-8")
        final.write_text("이 수식에 대해 확인이 필요합니다. 확인 바랍니다.\n", encoding="utf-8")

        diff = run_review_learning(draft, final)
        assert isinstance(diff, ProfileDiff)
        categories = [p.category for p in diff.new_patterns]
        assert "tone" in categories or "wording" in categories

    def test_run_review_learning_no_changes(self, tmp_path):
        """Identical texts produce no patterns."""
        text = "# A. 주요 의견\n\n내용 동일\n"
        draft = tmp_path / ".draft.md"
        final = tmp_path / "리뷰의견.md"
        draft.write_text(text, encoding="utf-8")
        final.write_text(text, encoding="utf-8")

        diff = run_review_learning(draft, final)
        assert diff.new_patterns == []
        assert diff.summary == "변경 없음"

    def test_run_review_learning_auto_apply(self, tmp_path):
        """auto_apply=True updates profile.yaml."""
        profile_path = tmp_path / "profile.yaml"
        save_profile({"learned_patterns": {}}, profile_path)

        draft = tmp_path / ".draft.md"
        final = tmp_path / "리뷰의견.md"
        draft.write_text("이 수식에 오류가 있습니다. 잘못된 부분입니다.\n", encoding="utf-8")
        final.write_text("이 수식에 대해 확인이 필요합니다. 확인 바랍니다.\n", encoding="utf-8")

        diff = run_review_learning(draft, final, profile_path=profile_path, auto_apply=True)
        assert len(diff.tone_updates) > 0

        updated = load_profile(profile_path)
        patterns = updated.get("learned_patterns", {})
        assert len(patterns.get("tone_preferences", [])) > 0
