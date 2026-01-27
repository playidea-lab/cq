"""Context Loader - Loads dynamic context from .c4/standards."""

import os
from pathlib import Path

class ContextLoader:
    @staticmethod
    def load_standards(project_root: Path | None = None) -> str:
        """Load and merge all standards files from registry."""
        # 1. System Registry (Primary)
        system_standards = Path(__file__).parent.parent / "system" / "registry" / "standards"
        
        # 2. Project Override (Optional)
        project_standards = None
        if project_root is None:
            env_root = os.environ.get("C4_PROJECT_ROOT")
            if env_root:
                project_standards = Path(env_root) / ".c4" / "standards"
            else:
                project_standards = Path.cwd() / ".c4" / "standards"

        # Use project standards if they exist (overrides), else system
        target_dir = project_standards if project_standards and project_standards.exists() else system_standards

        if not target_dir.exists():
            return ""
            
        content = []
        content.append("# PROJECT STANDARDS & RULES")
        content.append("Follow these rules strictly when reviewing code:\n")
        
        for file in sorted(target_dir.glob("*.md")):
            try:
                text = file.read_text()
                content.append(f"## {file.name}\n{text}")
            except Exception:
                pass
                
        return "\n\n".join(content)
