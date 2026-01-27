import os
import logging
from pathlib import Path

logger = logging.getLogger(__name__)

class ContextLoader:
    MAX_CONTEXT_CHARS = 30000  # Threshold for slimming (approx 7.5k tokens)

    @staticmethod
    def load_standards(project_root: Path | None = None, slim: bool = True) -> str:
        """Load and merge all standards files from registry.
        
        Args:
            project_root: Optional project root path
            slim: If True, applies context slimming when content is too large
        """
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
            
        files = sorted(target_dir.glob("*.md"))
        if not files:
            return ""

        content_parts = []
        total_chars = 0
        
        # Initial pass to check size
        temp_contents = []
        for file in files:
            try:
                text = file.read_text()
                temp_contents.append((file.name, text))
                total_chars += len(text)
            except Exception:
                continue

        # Apply slimming if needed
        is_slimmed = False
        if slim and total_chars > ContextLoader.MAX_CONTEXT_CHARS:
            is_slimmed = True
            logger.info(f"Context slimming active: {total_chars} chars exceeded limit")
            
            # Slimming strategy: Take only the first 2000 chars of each file (Core sections)
            for name, text in temp_contents:
                if len(text) > 3000:
                    slimmed_text = text[:3000] + "\n\n... (content truncated for token slimming) ..."
                    content_parts.append(f"## {name} (SLIMMED)\n{slimmed_text}")
                else:
                    content_parts.append(f"## {name}\n{text}")
        else:
            for name, text in temp_contents:
                content_parts.append(f"## {name}\n{text}")
                
        header = "# PROJECT STANDARDS & RULES\n"
        if is_slimmed:
            header += "> NOTE: Some standards have been truncated to save tokens. Use read_file on full path if needed.\n"
        header += "Follow these rules strictly when reviewing code:\n\n"
        
        return header + "\n\n".join(content_parts)
