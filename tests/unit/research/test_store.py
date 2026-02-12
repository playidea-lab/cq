"""Tests for ResearchStore - SQLite-backed research project tracking."""

import pytest

from c4.research.models import IterationStatus, ProjectStatus
from c4.research.store import ResearchStore


@pytest.fixture
def store(tmp_path):
    """Create a ResearchStore in a temp directory."""
    return ResearchStore(base_path=tmp_path / "research")


class TestCreateProject:
    def test_create_and_get(self, store):
        pid = store.create_project("PPAD Paper 1", paper_path="/tmp/paper.pdf", repo_path="/tmp/repo")
        project = store.get_project(pid)
        assert project is not None
        assert project.name == "PPAD Paper 1"
        assert project.paper_path == "/tmp/paper.pdf"
        assert project.repo_path == "/tmp/repo"
        assert project.target_score == 7.0
        assert project.current_iteration == 0
        assert project.status == ProjectStatus.ACTIVE

    def test_create_with_custom_target(self, store):
        pid = store.create_project("Test", target_score=8.5)
        project = store.get_project(pid)
        assert project.target_score == 8.5

    def test_list_projects(self, store):
        store.create_project("P1")
        store.create_project("P2")
        projects = store.list_projects()
        assert len(projects) == 2

    def test_list_projects_by_status(self, store):
        pid = store.create_project("P1")
        store.create_project("P2")
        store.update_project(pid, status="paused")
        active = store.list_projects(status="active")
        assert len(active) == 1
        assert active[0].name == "P2"

    def test_get_nonexistent(self, store):
        assert store.get_project("nonexistent") is None

    def test_update_project(self, store):
        pid = store.create_project("Old Name")
        store.update_project(pid, name="New Name", target_score=9.0)
        project = store.get_project(pid)
        assert project.name == "New Name"
        assert project.target_score == 9.0


class TestCreateIteration:
    def test_create_iteration(self, store):
        pid = store.create_project("P1")
        iid = store.create_iteration(pid)
        iteration = store.get_iteration(iid)
        assert iteration is not None
        assert iteration.project_id == pid
        assert iteration.iteration_num == 1
        assert iteration.status == IterationStatus.REVIEWING

    def test_iteration_increments(self, store):
        pid = store.create_project("P1")
        iid1 = store.create_iteration(pid)
        iid2 = store.create_iteration(pid)
        i1 = store.get_iteration(iid1)
        i2 = store.get_iteration(iid2)
        assert i1.iteration_num == 1
        assert i2.iteration_num == 2

    def test_current_iteration(self, store):
        pid = store.create_project("P1")
        store.create_iteration(pid)
        store.create_iteration(pid)
        current = store.get_current_iteration(pid)
        assert current.iteration_num == 2

    def test_list_iterations(self, store):
        pid = store.create_project("P1")
        store.create_iteration(pid)
        store.create_iteration(pid)
        iterations = store.list_iterations(pid)
        assert len(iterations) == 2
        assert iterations[0].iteration_num == 1
        assert iterations[1].iteration_num == 2

    def test_project_current_iteration_updated(self, store):
        pid = store.create_project("P1")
        store.create_iteration(pid)
        project = store.get_project(pid)
        assert project.current_iteration == 1


class TestUpdateIteration:
    def test_update_score(self, store):
        pid = store.create_project("P1")
        iid = store.create_iteration(pid)
        store.update_iteration(iid, review_score=6.5)
        iteration = store.get_iteration(iid)
        assert iteration.review_score == 6.5

    def test_update_axis_scores(self, store):
        pid = store.create_project("P1")
        iid = store.create_iteration(pid)
        scores = {"quality": 7.0, "novelty": 5.0, "significance": 6.0}
        store.update_iteration(iid, axis_scores=scores)
        iteration = store.get_iteration(iid)
        assert iteration.axis_scores == scores

    def test_update_gaps(self, store):
        pid = store.create_project("P1")
        iid = store.create_iteration(pid)
        gaps = [{"type": "experiment", "desc": "Add LOSO CV", "priority": "A1"}]
        store.update_iteration(iid, gaps=gaps)
        iteration = store.get_iteration(iid)
        assert iteration.gaps == gaps

    def test_update_experiments(self, store):
        pid = store.create_project("P1")
        iid = store.create_iteration(pid)
        exps = [{"name": "LOSO CV", "status": "planned", "job_id": "job-123"}]
        store.update_iteration(iid, experiments=exps)
        iteration = store.get_iteration(iid)
        assert iteration.experiments == exps

    def test_update_status(self, store):
        pid = store.create_project("P1")
        iid = store.create_iteration(pid)
        store.update_iteration(iid, status="planning")
        iteration = store.get_iteration(iid)
        assert iteration.status == IterationStatus.PLANNING


class TestSuggestNext:
    def test_no_iterations_suggests_review(self, store):
        pid = store.create_project("P1")
        result = store.suggest_next(pid)
        assert result["action"] == "review"

    def test_reviewing_suggests_continue_review(self, store):
        pid = store.create_project("P1")
        store.create_iteration(pid)
        result = store.suggest_next(pid)
        assert result["action"] == "review"
        assert "in progress" in result["reason"].lower()

    def test_score_above_target_suggests_complete(self, store):
        pid = store.create_project("P1", target_score=7.0)
        iid = store.create_iteration(pid)
        store.update_iteration(iid, review_score=8.0, status="planning")
        result = store.suggest_next(pid)
        assert result["action"] == "complete"

    def test_score_above_target_with_experiment_gaps_not_complete(self, store):
        pid = store.create_project("P1", target_score=7.0)
        iid = store.create_iteration(pid)
        gaps = [{"type": "experiment", "desc": "Add baseline", "status": "planned"}]
        store.update_iteration(iid, review_score=8.0, gaps=gaps, status="planning")
        result = store.suggest_next(pid)
        assert result["action"] == "run_experiments"

    def test_done_iteration_suggests_new_review(self, store):
        pid = store.create_project("P1")
        iid = store.create_iteration(pid)
        store.update_iteration(iid, review_score=5.0, status="done")
        result = store.suggest_next(pid)
        assert result["action"] == "review"
        assert "previous iteration complete" in result["reason"].lower()

    def test_gaps_with_pending_experiments(self, store):
        pid = store.create_project("P1")
        iid = store.create_iteration(pid)
        gaps = [
            {"type": "experiment", "desc": "Exp A", "status": "planned"},
            {"type": "experiment", "desc": "Exp B", "status": "completed"},
        ]
        store.update_iteration(iid, review_score=5.0, gaps=gaps, status="experimenting")
        result = store.suggest_next(pid)
        assert result["action"] == "run_experiments"
        assert "1 experiments remaining" in result["reason"]

    def test_plan_experiments_fallback(self, store):
        pid = store.create_project("P1")
        iid = store.create_iteration(pid)
        store.update_iteration(iid, review_score=5.0, status="planning")
        result = store.suggest_next(pid)
        assert result["action"] == "plan_experiments"

    def test_paused_project(self, store):
        pid = store.create_project("P1")
        store.update_project(pid, status="paused")
        result = store.suggest_next(pid)
        assert result["action"] == "none"

    def test_nonexistent_project(self, store):
        result = store.suggest_next("nonexistent")
        assert result["action"] == "none"
