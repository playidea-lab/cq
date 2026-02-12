"""
Paper analyzer using LLM Vision API (Anthropic or OpenAI).
"""

import base64
import json

from pydantic import ValidationError

from c4.review.llm import LLMClient, create_llm
from c4.review.models import (
    Figure,
    OverallAssessment,
    PageImage,
    PaperStructure,
    ReviewerProfile,
    Section,
    SectionAnalysis,
    SectionEval,
    Table,
)
from c4.review.prompts import (
    STRUCTURE_ANALYSIS_PROMPT,
    build_assessment_prompt,
    build_section_analysis_prompt,
)


class PaperAnalyzer:
    """Analyzes academic papers using LLM Vision API."""

    def __init__(
        self,
        api_key: str | None = None,
        model_name: str = "claude-opus-4",
        max_tokens: int = 4096,
        llm: LLMClient | None = None,
    ):
        """
        Initialize the paper analyzer.

        Args:
            api_key: API key (if None, uses env var)
            model_name: Model to use
            max_tokens: Maximum tokens for API response
            llm: Optional pre-built LLMClient (overrides api_key/model_name)
        """
        self.llm = llm or create_llm(api_key=api_key, model=model_name)
        self.max_tokens = max_tokens

    def analyze_structure(self, pages: list[PageImage]) -> PaperStructure:
        """
        Analyze paper structure from page images (Pass 1).

        Args:
            pages: List of PageImage objects

        Returns:
            PaperStructure object with extracted structural information

        Raises:
            ValueError: If pages list is empty or API response is invalid
        """
        if not pages:
            raise ValueError("Pages list cannot be empty")

        # Prepare image content for API
        content = []
        for page in pages:
            image_base64 = base64.b64encode(page.image_data).decode("utf-8")
            content.append(self.llm.format_image(image_base64))

        content.append(self.llm.format_text(STRUCTURE_ANALYSIS_PROMPT))

        response = self.llm.chat(
            messages=[{"role": "user", "content": content}],
            max_tokens=self.max_tokens,
        )
        response_text = response.text

        # Parse JSON response
        try:
            # Find JSON block in response
            json_start = response_text.find("{")
            json_end = response_text.rfind("}") + 1
            if json_start == -1 or json_end == 0:
                raise ValueError("No JSON found in API response")

            json_str = response_text[json_start:json_end]
            data = json.loads(json_str)
        except json.JSONDecodeError as e:
            raise ValueError(f"Invalid JSON in API response: {e}")

        # Convert to PaperStructure
        try:
            structure = PaperStructure(
                title=data.get("title", "Unknown Title"),
                authors=data.get("authors", []),
                sections=[Section(**s) for s in data.get("sections", [])],
                figures=[Figure(**f) for f in data.get("figures", [])],
                tables=[Table(**t) for t in data.get("tables", [])],
                equations_count=data.get("equations_count", 0),
                references_count=data.get("references_count", 0),
            )
            return structure
        except (ValidationError, TypeError) as e:
            raise ValueError(f"Failed to parse API response into PaperStructure: {e}")

    def analyze_sections(
        self,
        structure: PaperStructure,
        pages: list[PageImage],
        profile: ReviewerProfile | None = None,
    ) -> SectionAnalysis:
        """
        Analyze sections in detail (Pass 2).

        Args:
            structure: PaperStructure from Pass 1
            pages: List of PageImage objects
            profile: Optional ReviewerProfile to customize review

        Returns:
            SectionAnalysis object with detailed section evaluations

        Raises:
            ValueError: If structure has no sections or API response is invalid
        """
        if not structure.sections:
            raise ValueError("PaperStructure has no sections to analyze")

        if not pages:
            raise ValueError("Pages list cannot be empty")

        # Build prompt with profile
        prompt = build_section_analysis_prompt(profile)

        # Add section context
        section_context = "\n**Sections to analyze**:\n"
        for section in structure.sections:
            section_context += f"- {section.title} (page {section.page_number}, level {section.level})\n"

        full_prompt = prompt + "\n" + section_context

        # Prepare image content for API
        content = []
        for page in pages:
            image_base64 = base64.b64encode(page.image_data).decode("utf-8")
            content.append(self.llm.format_image(image_base64))

        content.append(self.llm.format_text(full_prompt))

        response = self.llm.chat(
            messages=[{"role": "user", "content": content}],
            max_tokens=self.max_tokens,
        )
        response_text = response.text

        # Parse JSON response
        try:
            # Find JSON block in response
            json_start = response_text.find("{")
            json_end = response_text.rfind("}") + 1
            if json_start == -1 or json_end == 0:
                raise ValueError("No JSON found in API response")

            json_str = response_text[json_start:json_end]
            data = json.loads(json_str)
        except json.JSONDecodeError as e:
            raise ValueError(f"Invalid JSON in API response: {e}")

        # Convert to SectionAnalysis
        try:
            analysis = SectionAnalysis(
                sections=[SectionEval(**s) for s in data.get("sections", [])],
                overall_notes=data.get("overall_notes", ""),
            )
            return analysis
        except (ValidationError, TypeError) as e:
            raise ValueError(f"Failed to parse API response into SectionAnalysis: {e}")

    def generate_assessment(
        self,
        section_analysis: SectionAnalysis,
        user_inputs: list[str] | None = None,
    ) -> OverallAssessment:
        """
        Generate overall assessment from section analysis (Pass 3).

        Args:
            section_analysis: SectionAnalysis from Pass 2
            user_inputs: Optional list of user discussion points/questions

        Returns:
            OverallAssessment object with scores, recommendation, and summary

        Raises:
            ValueError: If section_analysis is empty or API response is invalid
        """
        if not section_analysis.sections:
            raise ValueError("SectionAnalysis has no section evaluations")

        # Build prompt with user inputs
        prompt = build_assessment_prompt(user_inputs)

        # Add section analysis context
        context = "\n**Section Evaluations**:\n"
        for section in section_analysis.sections:
            context += f"\n- {section.section_name} (Score: {section.score}/10)\n"
            context += f"  Strengths: {', '.join(section.strengths) if section.strengths else 'None'}\n"
            context += f"  Weaknesses: {', '.join(section.weaknesses) if section.weaknesses else 'None'}\n"

        if section_analysis.overall_notes:
            context += f"\nOverall notes: {section_analysis.overall_notes}\n"

        full_prompt = prompt + "\n" + context

        response = self.llm.chat(
            messages=[{"role": "user", "content": full_prompt}],
            max_tokens=self.max_tokens,
        )
        response_text = response.text

        # Parse JSON response
        try:
            # Find JSON block in response
            json_start = response_text.find("{")
            json_end = response_text.rfind("}") + 1
            if json_start == -1 or json_end == 0:
                raise ValueError("No JSON found in API response")

            json_str = response_text[json_start:json_end]
            data = json.loads(json_str)
        except json.JSONDecodeError as e:
            raise ValueError(f"Invalid JSON in API response: {e}")

        # Convert to OverallAssessment
        try:
            assessment = OverallAssessment(
                dimension_scores=data.get("dimension_scores", {}),
                overall_score=data.get("overall_score", 5.0),
                recommendation=data.get("recommendation", "MinorRevision"),
                summary=data.get("summary", ""),
                key_strengths=data.get("key_strengths", []),
                key_weaknesses=data.get("key_weaknesses", []),
            )
            return assessment
        except (ValidationError, TypeError) as e:
            raise ValueError(f"Failed to parse API response into OverallAssessment: {e}")
