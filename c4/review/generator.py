"""
Review document generator - creates formatted Markdown review documents.
"""

from pathlib import Path

from c4.review.llm import LLMClient, create_llm
from c4.review.models import OverallAssessment, ReviewConfig, ReviewDocument
from c4.review.prompts import build_review_generation_prompt, build_translation_prompt


class ReviewGenerator:
    """Generates formatted review documents in Markdown."""

    def __init__(
        self,
        api_key: str | None = None,
        model_name: str = "claude-opus-4",
        max_tokens: int = 8192,
        llm: LLMClient | None = None,
    ):
        """
        Initialize the review generator.

        Args:
            api_key: API key (if None, uses env var)
            model_name: Model to use
            max_tokens: Maximum tokens for API response
            llm: Optional pre-built LLMClient (overrides api_key/model_name)
        """
        self.llm = llm or create_llm(api_key=api_key, model=model_name)
        self.max_tokens = max_tokens

    def generate_review(
        self,
        assessment: OverallAssessment,
        user_inputs: list[str] | None = None,
        config: ReviewConfig | None = None,
    ) -> ReviewDocument:
        """
        Generate a complete review document.

        Args:
            assessment: OverallAssessment from Pass 3
            user_inputs: Optional user discussion points
            config: Optional ReviewConfig for customization

        Returns:
            ReviewDocument with formatted content

        Raises:
            ValueError: If assessment is invalid
        """
        if config is None:
            config = ReviewConfig()

        # Build the review generation prompt
        prompt = build_review_generation_prompt(assessment, user_inputs, config)

        response = self.llm.chat(
            messages=[{"role": "user", "content": prompt}],
            max_tokens=self.max_tokens,
        )
        korean_review = response.text

        # Translate to English
        english_review = self._translate_to_english(korean_review)

        # Create ReviewDocument (placeholder - actual implementation will parse structure)
        # For now, we store the formatted review text
        review_doc = ReviewDocument(
            metadata=None,  # Will be filled in integrated pipeline
            structure=None,  # Will be filled in integrated pipeline
            overall_assessment=assessment,
            reviewer_notes=f"{korean_review}\n\n---\n\n[English Translation]\n\n{english_review}",
        )

        return review_doc

    def _translate_to_english(self, korean_text: str) -> str:
        """
        Translate Korean review to English.

        Args:
            korean_text: Korean review text

        Returns:
            English translation
        """
        prompt = build_translation_prompt(korean_text)

        response = self.llm.chat(
            messages=[{"role": "user", "content": prompt}],
            max_tokens=self.max_tokens,
        )
        return response.text

    def save_review(self, review: ReviewDocument, output_dir: Path) -> Path:
        """
        Save review document to Markdown file.

        Args:
            review: ReviewDocument to save
            output_dir: Directory to save the review

        Returns:
            Path to the saved review file

        Raises:
            ValueError: If output_dir is invalid or not writable
        """
        if not output_dir.exists():
            raise ValueError(f"Output directory does not exist: {output_dir}")

        if not output_dir.is_dir():
            raise ValueError(f"Output path is not a directory: {output_dir}")

        # Generate filename
        output_file = output_dir / "review.md"

        # Write the review
        try:
            output_file.write_text(review.reviewer_notes, encoding="utf-8")
        except Exception as e:
            raise ValueError(f"Failed to write review file: {e}")

        return output_file
