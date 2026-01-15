"""Tests for Cursor command files."""

from pathlib import Path


def test_cursor_commands_exist() -> None:
    """Ensure Cursor command files are present."""
    repo_root = Path(__file__).resolve().parents[2]
    commands_dir = repo_root / ".cursor" / "commands"

    required_files = [
        "c4-status.md",
        "c4-init.md",
        "c4-stop.md",
        "c4-clear.md",
        "c4-validate.md",
        "c4-add-task.md",
        "c4-plan.md",
        "c4-run.md",
        "c4-checkpoint.md",
        "c4-submit.md",
    ]

    missing = [name for name in required_files if not (commands_dir / name).exists()]
    assert not missing, f"Missing Cursor command files: {missing}"
