
import json
import logging
from typing import Any, Optional

from c4.c2.models import ProfileDiff
from c4.c2.persona import PersonaLearner

logger = logging.getLogger(__name__)

class SemanticPersonaLearner(PersonaLearner):
    """Extends PersonaLearner with LLM-based semantic analysis."""

    def __init__(self, bridge_client: Any):
        self.bridge = bridge_client

    async def learn_from_edit(self, original: str, edited: str, context: Optional[str] = None) -> ProfileDiff:
        """Analyzes an edit using an LLM to extract high-level persona traits."""

        prompt = f"""
Analyze the following edit between an AI-generated draft and a user's final version.
Extract the user's underlying engineering preferences, tone, and structural choices.

[Context]
{context or "General engineering task"}

[Original Draft]
{original}

[User's Final Version]
{edited}

Return a JSON object with:
1. "tone": specific observations about language style.
2. "patterns": common coding or architectural patterns preferred.
3. "constraints": what the user seems to avoid or mandate.
"""
        # Call LLM via bridge (abstracted provider)
        response = await self.bridge.call_llm(prompt, model_hint="reasoning-high")

        try:
            analysis = json.loads(response)
            return ProfileDiff(
                tone_updates=analysis.get("tone", []),
                new_patterns=analysis.get("patterns", []),
                intent_insight=analysis.get("constraints", [])
            )
        except Exception as e:
            logger.error(f"Failed to parse semantic persona analysis: {e}")
            # Fallback to basic difflib analysis
            patterns = self.analyze_edits(original, edited)
            return ProfileDiff(new_patterns=patterns)
