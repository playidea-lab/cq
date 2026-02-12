"""
Review profile management - tracks reviewer patterns and preferences.
"""

import json
from pathlib import Path

import yaml
from anthropic import Anthropic

from c4.review.models import ReviewDocument, ReviewerProfile, ReviewPoint, StyleGuide


class ReviewProfile:
    """Manages reviewer profiles with pattern tracking and learning."""

    def __init__(self, api_key: str | None = None):
        """
        Initialize the profile manager.

        Args:
            api_key: Anthropic API key for bootstrap functionality
        """
        self.client = Anthropic(api_key=api_key) if api_key else None

    def load_profile(self, profile_path: Path) -> ReviewerProfile:
        """
        Load reviewer profile from YAML file.

        Args:
            profile_path: Path to profile.yaml

        Returns:
            ReviewerProfile object

        Raises:
            ValueError: If profile file is invalid
        """
        if not profile_path.exists():
            # Return empty profile if file doesn't exist
            return ReviewerProfile()

        try:
            with open(profile_path, encoding="utf-8") as f:
                data = yaml.safe_load(f)

            if data is None:
                return ReviewerProfile()

            return ReviewerProfile(**data)
        except Exception as e:
            raise ValueError(f"Failed to load profile: {e}")

    def save_profile(self, profile: ReviewerProfile, profile_path: Path) -> None:
        """
        Save reviewer profile to YAML file.

        Args:
            profile: ReviewerProfile to save
            profile_path: Path to save profile.yaml

        Raises:
            ValueError: If save fails
        """
        try:
            # Ensure directory exists
            profile_path.parent.mkdir(parents=True, exist_ok=True)

            # Convert to dict and save as YAML
            with open(profile_path, "w", encoding="utf-8") as f:
                yaml.dump(profile.model_dump(), f, allow_unicode=True, sort_keys=False)
        except Exception as e:
            raise ValueError(f"Failed to save profile: {e}")

    def update_profile(
        self, profile: ReviewerProfile, review_doc: ReviewDocument
    ) -> ReviewerProfile:
        """
        Update profile based on a completed review.

        Increments frequency of review points found in the review.

        Args:
            profile: Current ReviewerProfile
            review_doc: Completed ReviewDocument

        Returns:
            Updated ReviewerProfile
        """
        # Update review points frequency
        # For now, we'll extract key weaknesses/strengths as review points
        if review_doc.overall_assessment:
            for weakness in review_doc.overall_assessment.key_weaknesses:
                self._add_or_increment_point(profile, weakness, "weakness")

            for strength in review_doc.overall_assessment.key_strengths:
                self._add_or_increment_point(profile, strength, "strength")

        return profile

    def _add_or_increment_point(
        self, profile: ReviewerProfile, point_text: str, category: str
    ) -> None:
        """
        Add a new review point or increment frequency if it exists.

        Args:
            profile: ReviewerProfile to update
            point_text: Text of the review point
            category: Category of the point
        """
        # Check if point already exists
        for point in profile.review_points:
            if point.point.lower() == point_text.lower():
                point.frequency += 1
                return

        # Add new point
        profile.review_points.append(
            ReviewPoint(point=point_text, frequency=1, category=category)
        )

    def get_emphasis_points(self, profile: ReviewerProfile) -> list[str]:
        """
        Get emphasis points from profile.

        Returns points that appear frequently (frequency >= 2).

        Args:
            profile: ReviewerProfile

        Returns:
            List of frequent review points
        """
        return [point.point for point in profile.review_points if point.frequency >= 2]

    def get_style_guide(self, profile: ReviewerProfile) -> StyleGuide:
        """
        Get style guide from profile.

        Args:
            profile: ReviewerProfile

        Returns:
            StyleGuide object
        """
        return profile.style_guide

    def bootstrap_from_existing(
        self, review_dir: Path, model_name: str = "claude-opus-4"
    ) -> ReviewerProfile:
        """
        Bootstrap a profile from existing review files.

        Analyzes existing review files to extract patterns.

        Args:
            review_dir: Directory containing existing review.md files
            model_name: Claude model to use for analysis

        Returns:
            Bootstrapped ReviewerProfile

        Raises:
            ValueError: If no review files found or API call fails
        """
        if not self.client:
            raise ValueError("API key required for bootstrapping")

        if not review_dir.exists() or not review_dir.is_dir():
            raise ValueError(f"Invalid review directory: {review_dir}")

        # Find review files
        review_files = list(review_dir.rglob("*.md"))
        if not review_files:
            raise ValueError(f"No review files found in {review_dir}")

        # Limit to first 5 reviews for analysis
        review_files = review_files[:5]

        # Read review contents
        reviews_text = ""
        for review_file in review_files:
            reviews_text += f"\n\n--- Review from {review_file.name} ---\n"
            reviews_text += review_file.read_text(encoding="utf-8")

        # Build analysis prompt
        prompt = f"""Analyze these academic paper reviews and extract the reviewer's patterns:

{reviews_text}

Identify:
1. **Expertise areas**: What domains/topics does the reviewer focus on?
2. **Review style**: strict/balanced/lenient based on tone
3. **Common points**: What criticisms/suggestions appear repeatedly?
4. **Emphasis**: What aspects do they emphasize (novelty, reproducibility, clarity)?

Return JSON:
```json
{{
  "expertise_areas": ["machine learning", "computer vision"],
  "review_style": "balanced",
  "focus_on_novelty": true,
  "focus_on_reproducibility": true,
  "review_points": [
    {{"point": "Lack of comparison with recent work", "frequency": 3, "category": "weakness"}},
    {{"point": "Clear presentation", "frequency": 2, "category": "strength"}}
  ],
  "style_guide": {{
    "tone": "professional",
    "emphasis_points": ["reproducibility", "novelty"],
    "common_phrases": ["needs clarification", "well-structured"]
  }}
}}
```
"""

        # Call API
        try:
            response = self.client.messages.create(
                model=model_name, max_tokens=4096, messages=[{"role": "user", "content": prompt}]
            )

            response_text = response.content[0].text

            # Parse JSON
            json_start = response_text.find("{")
            json_end = response_text.rfind("}") + 1
            if json_start == -1 or json_end == 0:
                raise ValueError("No JSON found in API response")

            json_str = response_text[json_start:json_end]
            data = json.loads(json_str)

            # Create profile
            profile = ReviewerProfile(**data)
            return profile

        except Exception as e:
            raise ValueError(f"Failed to bootstrap profile: {e}")
