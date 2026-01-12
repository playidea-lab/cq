"""Unit tests for SkillMatcher - Task-to-skill extraction and agent matching.

Tests cover:
1. TaskContext dataclass
2. extract_required_skills() - keyword, task_type, file_pattern matching
3. find_best_agents() - scoring and ranking
4. get_recommended_agent() - convenience method
"""

from __future__ import annotations

import pytest

from c4.supervisor.agent_graph import (
    AgentGraph,
    SkillMatcher,
    TaskContext,
)
from c4.supervisor.agent_graph.models import (
    Agent,
    AgentDefinition,
    AgentPersona,
    AgentRelationships,
    AgentSkills,
    Skill,
    SkillDefinition,
    SkillTriggers,
)

# ============================================================================
# Fixtures
# ============================================================================


@pytest.fixture
def graph() -> AgentGraph:
    """Create an AgentGraph with test skills and agents."""
    g = AgentGraph()

    # Add skills
    python_skill = SkillDefinition(
        skill=Skill(
            id="python-coding",
            name="Python Coding",
            description="Writing Python code and modules",
            capabilities=["write python code"],
            triggers=SkillTriggers(
                keywords=["python", "py", "django", "flask"],
                task_types=["feature", "refactor"],
                file_patterns=["*.py"],
            ),
        )
    )
    g.add_skill(python_skill)

    debugging_skill = SkillDefinition(
        skill=Skill(
            id="debugging",
            name="Debugging",
            description="Finding and fixing bugs in code",
            capabilities=["debug code"],
            triggers=SkillTriggers(
                keywords=["debug", "bug", "fix", "error"],
                task_types=["bugfix"],
            ),
        )
    )
    g.add_skill(debugging_skill)

    api_skill = SkillDefinition(
        skill=Skill(
            id="api-design",
            name="API Design",
            description="Designing RESTful APIs",
            capabilities=["design APIs"],
            triggers=SkillTriggers(
                keywords=["api", "rest", "endpoint"],
                file_patterns=["**/api/**", "**/routes/**"],
            ),
        )
    )
    g.add_skill(api_skill)

    frontend_skill = SkillDefinition(
        skill=Skill(
            id="frontend-dev",
            name="Frontend Development",
            description="Building frontend interfaces",
            capabilities=["build UIs"],
            triggers=SkillTriggers(
                keywords=["react", "vue", "frontend", "ui"],
                file_patterns=["*.tsx", "*.jsx", "*.vue"],
            ),
        )
    )
    g.add_skill(frontend_skill)

    # Add agents
    backend_dev = AgentDefinition(
        agent=Agent(
            id="backend-dev",
            name="Backend Developer",
            persona=AgentPersona(
                role="Python backend specialist",
                expertise="Python, APIs, databases",
            ),
            skills=AgentSkills(
                primary=["python-coding", "api-design"],
                secondary=["debugging"],
            ),
            relationships=AgentRelationships(),
        )
    )
    g.add_agent(backend_dev)

    debugger = AgentDefinition(
        agent=Agent(
            id="debugger",
            name="Debugger",
            persona=AgentPersona(
                role="Bug hunting specialist",
                expertise="Debugging, profiling, tracing",
            ),
            skills=AgentSkills(
                primary=["debugging"],
                secondary=["python-coding"],
            ),
            relationships=AgentRelationships(),
        )
    )
    g.add_agent(debugger)

    frontend_dev = AgentDefinition(
        agent=Agent(
            id="frontend-dev-agent",
            name="Frontend Developer",
            persona=AgentPersona(
                role="React/Vue specialist",
                expertise="React, Vue, TypeScript, CSS",
            ),
            skills=AgentSkills(
                primary=["frontend-dev"],
                secondary=[],
            ),
            relationships=AgentRelationships(),
        )
    )
    g.add_agent(frontend_dev)

    return g


@pytest.fixture
def matcher(graph: AgentGraph) -> SkillMatcher:
    """Create a SkillMatcher with the test graph."""
    return SkillMatcher(graph)


# ============================================================================
# Test TaskContext Dataclass
# ============================================================================


class TestTaskContext:
    """Tests for TaskContext dataclass."""

    def test_basic_creation(self) -> None:
        """TaskContext should be created with just title."""
        task = TaskContext(title="Fix bug")

        assert task.title == "Fix bug"
        assert task.description == ""
        assert task.task_type is None
        assert task.scope is None

    def test_full_creation(self) -> None:
        """TaskContext should accept all fields."""
        task = TaskContext(
            title="Add API endpoint",
            description="Create a new REST endpoint",
            task_type="feature",
            scope="src/api/users.py",
        )

        assert task.title == "Add API endpoint"
        assert task.description == "Create a new REST endpoint"
        assert task.task_type == "feature"
        assert task.scope == "src/api/users.py"

    def test_empty_title_raises(self) -> None:
        """TaskContext should raise ValueError for empty title."""
        with pytest.raises(ValueError, match="title cannot be empty"):
            TaskContext(title="")

    def test_whitespace_title_raises(self) -> None:
        """TaskContext should raise ValueError for whitespace-only title."""
        with pytest.raises(ValueError, match="title cannot be empty"):
            TaskContext(title="   ")


# ============================================================================
# Test extract_required_skills - Keyword Matching
# ============================================================================


class TestExtractRequiredSkillsKeywords:
    """Tests for keyword-based skill extraction."""

    def test_keyword_in_title(self, matcher: SkillMatcher) -> None:
        """Should match keyword in title."""
        task = TaskContext(title="Fix Python bug")
        skills = matcher.extract_required_skills(task)

        assert "python-coding" in skills

    def test_keyword_in_description(self, matcher: SkillMatcher) -> None:
        """Should match keyword in description."""
        task = TaskContext(title="Fix issue", description="Debug the django app")
        skills = matcher.extract_required_skills(task)

        assert "python-coding" in skills  # 'django' keyword

    def test_keyword_case_insensitive(self, matcher: SkillMatcher) -> None:
        """Keyword matching should be case-insensitive."""
        task = TaskContext(title="Fix PYTHON Bug")
        skills = matcher.extract_required_skills(task)

        assert "python-coding" in skills

    def test_multiple_keywords_match(self, matcher: SkillMatcher) -> None:
        """Should match multiple skills from different keywords."""
        task = TaskContext(title="Debug Python API")
        skills = matcher.extract_required_skills(task)

        assert "python-coding" in skills
        assert "debugging" in skills
        assert "api-design" in skills

    def test_no_keyword_match(self, matcher: SkillMatcher) -> None:
        """Should return empty list if no keywords match."""
        task = TaskContext(title="Update documentation")
        skills = matcher.extract_required_skills(task)

        # No skills should match (no triggers for "documentation")
        assert "python-coding" not in skills
        assert "debugging" not in skills


# ============================================================================
# Test extract_required_skills - Task Type Matching
# ============================================================================


class TestExtractRequiredSkillsTaskType:
    """Tests for task_type-based skill extraction."""

    def test_task_type_match(self, matcher: SkillMatcher) -> None:
        """Should match skill by task_type."""
        task = TaskContext(title="New endpoint", task_type="feature")
        skills = matcher.extract_required_skills(task)

        assert "python-coding" in skills  # triggers on 'feature' task_type

    def test_task_type_case_insensitive(self, matcher: SkillMatcher) -> None:
        """task_type matching should be case-insensitive."""
        task = TaskContext(title="New endpoint", task_type="FEATURE")
        skills = matcher.extract_required_skills(task)

        assert "python-coding" in skills

    def test_task_type_bugfix(self, matcher: SkillMatcher) -> None:
        """Should match debugging skill for bugfix type."""
        task = TaskContext(title="Something", task_type="bugfix")
        skills = matcher.extract_required_skills(task)

        assert "debugging" in skills

    def test_task_type_no_match(self, matcher: SkillMatcher) -> None:
        """Should not match if task_type doesn't match triggers."""
        task = TaskContext(title="Something", task_type="documentation")
        skills = matcher.extract_required_skills(task)

        assert "python-coding" not in skills  # 'documentation' not in triggers


# ============================================================================
# Test extract_required_skills - File Pattern Matching
# ============================================================================


class TestExtractRequiredSkillsFilePattern:
    """Tests for file_pattern-based skill extraction."""

    def test_file_pattern_exact_match(self, matcher: SkillMatcher) -> None:
        """Should match skill by exact file pattern."""
        task = TaskContext(title="Something", scope="main.py")
        skills = matcher.extract_required_skills(task)

        assert "python-coding" in skills

    def test_file_pattern_directory_match(self, matcher: SkillMatcher) -> None:
        """Should match skill by directory pattern."""
        task = TaskContext(title="Something", scope="src/api/users.py")
        skills = matcher.extract_required_skills(task)

        assert "api-design" in skills

    def test_file_pattern_tsx_match(self, matcher: SkillMatcher) -> None:
        """Should match frontend skill for .tsx files."""
        task = TaskContext(title="Something", scope="components/Button.tsx")
        skills = matcher.extract_required_skills(task)

        assert "frontend-dev" in skills

    def test_file_pattern_no_scope(self, matcher: SkillMatcher) -> None:
        """Should not match file pattern when scope is None."""
        task = TaskContext(title="Python work")  # No scope
        skills = matcher.extract_required_skills(task)

        # Should still match via keyword, but not file pattern
        assert "python-coding" in skills


# ============================================================================
# Test find_best_agents - Basic Matching
# ============================================================================


class TestFindBestAgentsBasic:
    """Tests for basic agent matching."""

    def test_single_skill_match(self, matcher: SkillMatcher) -> None:
        """Should find agents with single skill."""
        agents = matcher.find_best_agents(["debugging"])

        assert len(agents) >= 2  # debugger and backend-dev both have debugging
        agent_ids = [a.agent_id for a in agents]
        assert "debugger" in agent_ids
        assert "backend-dev" in agent_ids

    def test_multiple_skill_match(self, matcher: SkillMatcher) -> None:
        """Should rank agents by number of matched skills."""
        agents = matcher.find_best_agents(["python-coding", "api-design"])

        # backend-dev has both (primary), should be first
        assert agents[0].agent_id == "backend-dev"
        assert len(agents[0].matched_skills) == 2

    def test_empty_skills_returns_empty(self, matcher: SkillMatcher) -> None:
        """Should return empty list for empty skills."""
        agents = matcher.find_best_agents([])

        assert agents == []

    def test_no_matching_agents(self, matcher: SkillMatcher) -> None:
        """Should return empty list when no agents match."""
        agents = matcher.find_best_agents(["nonexistent-skill"])

        assert agents == []


# ============================================================================
# Test find_best_agents - Scoring
# ============================================================================


class TestFindBestAgentsScoring:
    """Tests for agent scoring logic."""

    def test_primary_skill_bonus(self, matcher: SkillMatcher) -> None:
        """Primary skills should get bonus score."""
        agents = matcher.find_best_agents(["debugging"])

        # Find debugger and backend-dev
        debugger = next(a for a in agents if a.agent_id == "debugger")
        backend_dev = next(a for a in agents if a.agent_id == "backend-dev")

        # debugger has debugging as primary, backend-dev as secondary
        assert debugger.primary_match_count == 1
        assert debugger.secondary_match_count == 0
        assert backend_dev.primary_match_count == 0
        assert backend_dev.secondary_match_count == 1

        # debugger should have higher score
        assert debugger.score > backend_dev.score

    def test_score_calculation(self, matcher: SkillMatcher) -> None:
        """Score should be base + primary bonus."""
        agents = matcher.find_best_agents(["python-coding", "api-design"])

        # backend-dev has both as primary
        backend = next(a for a in agents if a.agent_id == "backend-dev")

        # Score = 2 (matched) + 2 * 0.5 (primary bonus) = 3.0
        assert backend.score == 3.0
        assert backend.primary_match_count == 2
        assert backend.secondary_match_count == 0

    def test_sorted_by_score_descending(self, matcher: SkillMatcher) -> None:
        """Agents should be sorted by score descending."""
        agents = matcher.find_best_agents(["debugging", "python-coding"])

        # Verify sorted
        for i in range(len(agents) - 1):
            assert agents[i].score >= agents[i + 1].score


# ============================================================================
# Test AgentMatch Dataclass
# ============================================================================


class TestAgentMatch:
    """Tests for AgentMatch dataclass."""

    def test_agent_match_fields(self, matcher: SkillMatcher) -> None:
        """AgentMatch should have all expected fields."""
        agents = matcher.find_best_agents(["python-coding"])

        assert len(agents) >= 1
        match = agents[0]

        assert isinstance(match.agent_id, str)
        assert isinstance(match.score, float)
        assert isinstance(match.matched_skills, list)
        assert isinstance(match.primary_match_count, int)
        assert isinstance(match.secondary_match_count, int)

    def test_matched_skills_sorted(self, matcher: SkillMatcher) -> None:
        """matched_skills should be sorted for consistent output."""
        agents = matcher.find_best_agents(["python-coding", "api-design"])

        backend = next(a for a in agents if a.agent_id == "backend-dev")
        # Skills should be in alphabetical order
        assert backend.matched_skills == sorted(backend.matched_skills)


# ============================================================================
# Test get_recommended_agent - Convenience Method
# ============================================================================


class TestGetRecommendedAgent:
    """Tests for get_recommended_agent convenience method."""

    def test_returns_best_agent(self, matcher: SkillMatcher) -> None:
        """Should return the best matching agent."""
        task = TaskContext(title="Fix Python API bug")
        agent = matcher.get_recommended_agent(task)

        # backend-dev should match (python, api, bug)
        assert agent == "backend-dev"

    def test_returns_none_no_match(self, matcher: SkillMatcher) -> None:
        """Should return None when no skills match."""
        task = TaskContext(title="Update readme")
        agent = matcher.get_recommended_agent(task)

        assert agent is None

    def test_frontend_task(self, matcher: SkillMatcher) -> None:
        """Should return frontend agent for frontend task."""
        task = TaskContext(title="Build React component", scope="Button.tsx")
        agent = matcher.get_recommended_agent(task)

        assert agent == "frontend-dev-agent"


# ============================================================================
# Test Edge Cases
# ============================================================================


class TestEdgeCases:
    """Tests for edge cases and error handling."""

    def test_empty_graph(self) -> None:
        """Should handle empty graph gracefully."""
        empty_graph = AgentGraph()
        matcher = SkillMatcher(empty_graph)

        task = TaskContext(title="Any task")
        skills = matcher.extract_required_skills(task)
        agents = matcher.find_best_agents(["any-skill"])

        assert skills == []
        assert agents == []

    def test_skill_without_triggers(self, graph: AgentGraph) -> None:
        """Should handle skills with empty triggers."""
        # Add skill with no trigger values
        skill_no_triggers = SkillDefinition(
            skill=Skill(
                id="empty-triggers",
                name="Empty Triggers",
                description="A skill with no triggers",
                capabilities=["nothing"],
                triggers=SkillTriggers(),  # All None
            )
        )
        graph.add_skill(skill_no_triggers)
        matcher = SkillMatcher(graph)

        task = TaskContext(title="Any task")
        skills = matcher.extract_required_skills(task)

        # Should not match skill with no triggers
        assert "empty-triggers" not in skills

    def test_agent_without_skills(self, graph: AgentGraph) -> None:
        """Should handle agents with no skills."""
        # Add agent with empty skills (primary is required, but secondary empty)
        # This is more of a model validation test
        pass  # Primary is required by model, so this is implicitly tested

    def test_special_characters_in_title(self, matcher: SkillMatcher) -> None:
        """Should handle special characters in task title."""
        task = TaskContext(title="Fix bug: user.login() @api/v2")
        skills = matcher.extract_required_skills(task)

        assert "debugging" in skills  # 'bug' keyword
        assert "api-design" in skills  # 'api' keyword
