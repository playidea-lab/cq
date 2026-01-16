"""Cloud Worker Process - Runs tasks in cloud environment."""

from __future__ import annotations

import os
import sys
from pathlib import Path


def main() -> int:
    """Main entry point for cloud worker.

    Environment Variables:
        C4_PROJECT_ID: Project to work on
        C4_TASK_ID: Specific task (optional)
        SUPABASE_URL: Supabase project URL
        SUPABASE_KEY: Supabase service key
        GITHUB_TOKEN: GitHub token for repo access

    Returns:
        Exit code (0 = success, 1 = error)
    """
    project_id = os.environ.get("C4_PROJECT_ID")
    task_id = os.environ.get("C4_TASK_ID")

    if not project_id:
        print("Error: C4_PROJECT_ID not set", file=sys.stderr)
        return 1

    print("C4 Cloud Worker starting")
    print(f"  Project: {project_id}")
    print(f"  Task: {task_id or 'any'}")

    # Clone or setup project
    workspace = Path("/home/worker/workspace")
    project_dir = workspace / project_id

    if not project_dir.exists():
        github_token = os.environ.get("GITHUB_TOKEN")
        repo_url = os.environ.get("C4_REPO_URL")

        if repo_url:
            import subprocess

            # Clone repository
            clone_url = repo_url
            if github_token and "github.com" in repo_url:
                clone_url = repo_url.replace(
                    "https://",
                    f"https://{github_token}@"
                )

            result = subprocess.run(
                ["git", "clone", "--depth", "1", clone_url, str(project_dir)],
                capture_output=True,
                text=True,
            )

            if result.returncode != 0:
                print(f"Error cloning: {result.stderr}", file=sys.stderr)
                return 1

            print(f"Cloned repository to {project_dir}")

    # Initialize C4 in project
    os.chdir(project_dir)
    os.environ["C4_PROJECT_ROOT"] = str(project_dir)

    # Run worker loop
    try:
        from c4.mcp_server import C4Daemon

        daemon = C4Daemon(project_root=project_dir)

        if not daemon.is_initialized():
            print("C4 not initialized in project", file=sys.stderr)
            return 1

        daemon.load()
        worker_id = f"cloud-{os.environ.get('FLY_MACHINE_ID', 'local')[:8]}"

        print(f"Starting worker loop as {worker_id}")

        # Simple worker loop
        while True:
            result = daemon.c4_get_task(worker_id)

            if result.get("error"):
                if "No tasks" in result["error"]:
                    print("No more tasks - worker complete")
                    return 0
                print(f"Error: {result['error']}", file=sys.stderr)
                return 1

            task = result
            print(f"Got task: {task['task_id']} - {task['title']}")

            # Execute task (simplified - in production, use full agent)
            # For now, just mark as needing manual implementation
            print(f"Task {task['task_id']} requires manual implementation")
            print(f"  DoD: {task['dod']}")
            print(f"  Scope: {task['scope']}")

            # In production, this would run the actual agent
            break

        return 0

    except KeyboardInterrupt:
        print("\nWorker interrupted")
        return 0
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
