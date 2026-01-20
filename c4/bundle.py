"""C4D Bundle Creator - Package checkpoint data for Supervisor review"""

import json
import subprocess
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path


@dataclass
class BundleSummary:
    """Summary of a checkpoint bundle"""

    checkpoint_id: str
    timestamp: datetime
    tasks_completed: list[str]
    files_changed: int
    lines_added: int
    lines_removed: int

    def to_dict(self) -> dict:
        return {
            "checkpoint_id": self.checkpoint_id,
            "timestamp": self.timestamp.isoformat(),
            "tasks_completed": self.tasks_completed,
            "files_changed": self.files_changed,
            "lines_added": self.lines_added,
            "lines_removed": self.lines_removed,
        }


class BundleCreator:
    """Create bundles for Supervisor review at checkpoints"""

    def __init__(self, project_root: Path, c4_dir: Path):
        self.root = project_root
        self.c4_dir = c4_dir
        self.bundles_dir = c4_dir / "bundles"

    def create_bundle(
        self,
        checkpoint_id: str,
        tasks_completed: list[str],
        validation_results: list[dict],
        base_branch: str = "main",
    ) -> Path:
        """
        Create a bundle for Supervisor review.

        Args:
            checkpoint_id: ID of the checkpoint (e.g., "CP1")
            tasks_completed: List of completed task IDs
            validation_results: List of validation results
            base_branch: Branch to diff against

        Returns:
            Path to the created bundle directory
        """
        # Create bundle directory
        ts = datetime.now().strftime("%Y%m%dT%H%M%S")
        bundle_name = f"cp-{checkpoint_id}-{ts}"
        bundle_dir = self.bundles_dir / bundle_name
        bundle_dir.mkdir(parents=True, exist_ok=True)

        # Get git diff stats
        diff_stats = self._get_diff_stats(base_branch)

        # Create summary
        summary = BundleSummary(
            checkpoint_id=checkpoint_id,
            timestamp=datetime.now(),
            tasks_completed=tasks_completed,
            files_changed=diff_stats["files_changed"],
            lines_added=diff_stats["lines_added"],
            lines_removed=diff_stats["lines_removed"],
        )

        # Save summary.json
        (bundle_dir / "summary.json").write_text(json.dumps(summary.to_dict(), indent=2))

        # Save changes.diff
        diff_content = self._get_diff_content(base_branch)
        (bundle_dir / "changes.diff").write_text(diff_content)

        # Save test_results.json
        (bundle_dir / "test_results.json").write_text(json.dumps(validation_results, indent=2))

        return bundle_dir

    def _get_diff_stats(self, base_branch: str) -> dict:
        """Get git diff statistics"""
        stats = {
            "files_changed": 0,
            "lines_added": 0,
            "lines_removed": 0,
        }

        try:
            # Check if we're in a git repo
            result = subprocess.run(
                ["git", "rev-parse", "--git-dir"],
                cwd=self.root,
                capture_output=True,
                text=True,
            )
            if result.returncode != 0:
                return stats

            # Get diff stats
            result = subprocess.run(
                ["git", "diff", "--stat", base_branch, "--", "."],
                cwd=self.root,
                capture_output=True,
                text=True,
            )

            if result.returncode == 0 and result.stdout.strip():
                # Parse the last line for summary
                # e.g., " 5 files changed, 120 insertions(+), 30 deletions(-)"
                lines = result.stdout.strip().split("\n")
                if lines:
                    summary_line = lines[-1]
                    import re

                    # Extract files changed
                    match = re.search(r"(\d+) files? changed", summary_line)
                    if match:
                        stats["files_changed"] = int(match.group(1))

                    # Extract insertions
                    match = re.search(r"(\d+) insertions?\(\+\)", summary_line)
                    if match:
                        stats["lines_added"] = int(match.group(1))

                    # Extract deletions
                    match = re.search(r"(\d+) deletions?\(-\)", summary_line)
                    if match:
                        stats["lines_removed"] = int(match.group(1))

        except Exception:
            pass

        return stats

    def _get_diff_content(self, base_branch: str) -> str:
        """Get git diff content"""
        try:
            result = subprocess.run(
                ["git", "diff", base_branch, "--", "."],
                cwd=self.root,
                capture_output=True,
                text=True,
            )
            if result.returncode == 0:
                return result.stdout
        except Exception:
            pass

        return ""

    def get_latest_bundle(self, checkpoint_id: str | None = None) -> Path | None:
        """Get the latest bundle, optionally filtered by checkpoint ID"""
        if not self.bundles_dir.exists():
            return None

        bundles = sorted(self.bundles_dir.iterdir(), reverse=True)

        for bundle in bundles:
            if not bundle.is_dir():
                continue
            if checkpoint_id is None:
                return bundle
            if f"cp-{checkpoint_id}-" in bundle.name:
                return bundle

        return None

    def load_bundle_summary(self, bundle_dir: Path) -> BundleSummary | None:
        """Load summary from a bundle directory"""
        summary_file = bundle_dir / "summary.json"
        if not summary_file.exists():
            return None

        data = json.loads(summary_file.read_text())
        return BundleSummary(
            checkpoint_id=data["checkpoint_id"],
            timestamp=datetime.fromisoformat(data["timestamp"]),
            tasks_completed=data["tasks_completed"],
            files_changed=data["files_changed"],
            lines_added=data["lines_added"],
            lines_removed=data["lines_removed"],
        )
