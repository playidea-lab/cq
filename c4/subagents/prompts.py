"""Prompt loader for subagent templates."""

from pathlib import Path


def load_prompt(agent_type: str) -> str:
    """Load prompt template for a subagent type.

    Args:
        agent_type: Type of agent (scout, worker, reviewer, planner)

    Returns:
        Prompt template content

    Raises:
        FileNotFoundError: If prompt file doesn't exist
        ValueError: If agent_type is invalid
    """
    valid_types = ["scout", "worker", "reviewer", "planner"]
    if agent_type not in valid_types:
        raise ValueError(f"Invalid agent_type: {agent_type}. Must be one of {valid_types}")

    prompt_file = Path(__file__).parent / "prompts" / f"{agent_type}.md"

    if not prompt_file.exists():
        raise FileNotFoundError(f"Prompt file not found: {prompt_file}")

    return prompt_file.read_text()
