"""Tests for c4-scout agent definition file."""

import re
from pathlib import Path


def test_c4_scout_agent_file_exists():
    """Test that c4-scout agent file exists."""
    agent_file = Path(".claude/agents/c4-scout.md")
    assert agent_file.exists(), "c4-scout.md should exist in .claude/agents/"


def test_c4_scout_agent_frontmatter():
    """Test that c4-scout has correct frontmatter configuration."""
    agent_file = Path(".claude/agents/c4-scout.md")
    content = agent_file.read_text()

    # Check frontmatter exists
    assert content.startswith("---"), "Agent file should start with frontmatter"

    # Extract frontmatter
    frontmatter_match = re.search(r"^---\n(.*?)\n---", content, re.DOTALL)
    assert frontmatter_match, "Could not parse frontmatter"

    frontmatter = frontmatter_match.group(1)

    # Check required fields
    assert "name: c4-scout" in frontmatter, "Should have name: c4-scout"
    assert (
        "model: haiku" in frontmatter
    ), "Should use haiku model for cost efficiency"
    assert "description:" in frontmatter, "Should have description"
    assert "color:" in frontmatter, "Should have color"

    # Check tool restrictions
    assert "tools:" in frontmatter, "Should have tools section"
    assert "- Glob" in frontmatter, "Should allow Glob"
    assert "- Grep" in frontmatter, "Should allow Grep"
    assert "- Read" in frontmatter, "Should allow Read"

    # Check that only allowed tools are listed
    tools_section = re.search(r"tools:\n(.*?)---", content, re.DOTALL)
    if tools_section:
        tools_text = tools_section.group(1)
        disallowed_tools = ["Write", "Edit", "Bash", "WebFetch", "WebSearch"]
        for tool in disallowed_tools:
            assert (
                f"- {tool}" not in tools_text
            ), f"Should not allow {tool} tool (context compression only)"


def test_c4_scout_agent_instructions():
    """Test that c4-scout has proper instructions."""
    agent_file = Path(".claude/agents/c4-scout.md")
    content = agent_file.read_text()

    # Check for key instruction sections
    assert (
        "context compression" in content.lower()
    ), "Should mention context compression"
    assert "500 tokens" in content.lower(), "Should mention 500 token limit"
    assert "haiku" in content.lower(), "Should mention Haiku model"

    # Check for workflow guidance
    assert "Glob" in content, "Should guide on using Glob"
    assert "Grep" in content, "Should guide on using Grep"
    assert "Read" in content, "Should guide on using Read"

    # Check for compression guidance
    assert (
        "compress" in content.lower() or "summary" in content.lower()
    ), "Should provide compression guidance"

    # Check for anti-patterns
    assert (
        "never" in content.lower() or "don't" in content.lower()
    ), "Should include anti-patterns"


def test_c4_scout_token_limit_mentioned():
    """Test that 500 token limit is clearly mentioned."""
    agent_file = Path(".claude/agents/c4-scout.md")
    content = agent_file.read_text()

    # Should mention 500 tokens multiple times
    token_mentions = content.lower().count("500")
    assert token_mentions >= 3, "Should mention 500 token limit multiple times"


def test_c4_scout_examples_provided():
    """Test that c4-scout includes usage examples."""
    agent_file = Path(".claude/agents/c4-scout.md")
    content = agent_file.read_text()

    # Check for example patterns
    example_indicators = [
        "example",
        "pattern",
        "request:",
        "response:",
        "good:",
        "bad:",
    ]

    found_examples = sum(
        1 for indicator in example_indicators if indicator in content.lower()
    )
    assert found_examples >= 4, "Should include multiple examples"


def test_c4_scout_structure_guidelines():
    """Test that c4-scout provides output structure guidelines."""
    agent_file = Path(".claude/agents/c4-scout.md")
    content = agent_file.read_text()

    # Check for structure-related guidance
    structure_keywords = ["format", "structure", "output", "template", "checklist"]

    found_keywords = sum(
        1 for keyword in structure_keywords if keyword in content.lower()
    )
    assert found_keywords >= 3, "Should provide structure guidelines"


def test_readme_exists():
    """Test that agents README exists."""
    readme_file = Path(".claude/agents/README.md")
    assert readme_file.exists(), "README.md should exist in .claude/agents/"


def test_readme_installation_instructions():
    """Test that README includes installation instructions."""
    readme_file = Path(".claude/agents/README.md")
    content = readme_file.read_text()

    # Check for installation instructions
    assert "installation" in content.lower(), "Should have installation section"
    assert "~/.claude/agents" in content, "Should mention user directory"
    assert "mkdir" in content or "copy" in content, "Should have copy instructions"


def test_readme_usage_examples():
    """Test that README includes usage examples."""
    readme_file = Path(".claude/agents/README.md")
    content = readme_file.read_text()

    # Check for usage examples
    assert "@c4-scout" in content, "Should show how to invoke the agent"
    assert "example" in content.lower(), "Should include examples"


def test_c4_scout_search_strategy_documented():
    """Test that c4-scout documents appropriate search strategies."""
    agent_file = Path(".claude/agents/c4-scout.md")
    content = agent_file.read_text()

    # Check that search strategies are documented
    assert "Glob" in content, "Should document using Glob"
    assert "Grep" in content, "Should document using Grep"


def test_c4_scout_cost_efficiency_emphasized():
    """Test that cost efficiency is emphasized."""
    agent_file = Path(".claude/agents/c4-scout.md")
    content = agent_file.read_text()

    cost_keywords = ["cost", "efficient", "haiku", "compress", "lightweight"]

    found_keywords = sum(1 for keyword in cost_keywords if keyword in content.lower())
    assert found_keywords >= 4, "Should emphasize cost efficiency"
