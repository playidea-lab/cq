"""
Review orchestrator - manages multi-pass review workflow with user feedback.
"""

from datetime import UTC, datetime
from pathlib import Path

from c4.review.analyzer import PaperAnalyzer
from c4.review.converter import PDFConverter
from c4.review.generator import ReviewGenerator
from c4.review.models import (
    OverallAssessment,
    PaperStructure,
    ReviewConfig,
    ReviewDocument,
    ReviewerProfile,
    ReviewSession,
    SectionAnalysis,
)


class ReviewOrchestrator:
    """Orchestrates the multi-pass analysis workflow."""

    def __init__(
        self,
        api_key: str | None = None,
        converter: PDFConverter | None = None,
        analyzer: PaperAnalyzer | None = None,
        generator: ReviewGenerator | None = None,
        llm=None,
    ):
        """
        Initialize the orchestrator.

        Args:
            api_key: API key
            converter: Optional PDFConverter instance (for testing)
            analyzer: Optional PaperAnalyzer instance (for testing)
            generator: Optional ReviewGenerator instance (for testing)
            llm: Optional LLMClient (shared by analyzer and generator)
        """
        self.api_key = api_key
        self.converter = converter or PDFConverter()
        self.analyzer = analyzer or PaperAnalyzer(api_key=api_key, llm=llm)
        self.generator = generator or ReviewGenerator(api_key=api_key, llm=llm)

    def start_review(self, pdf_path: Path, config: ReviewConfig | None = None) -> ReviewSession:
        """
        Start a new review session.

        Args:
            pdf_path: Path to the PDF file
            config: Optional ReviewConfig

        Returns:
            ReviewSession with initialized state

        Raises:
            FileNotFoundError: If PDF does not exist
            ValueError: If PDF is invalid or corrupted
        """
        if not pdf_path.exists():
            raise FileNotFoundError(f"PDF file not found: {pdf_path}")

        if config is None:
            config = ReviewConfig()

        # Convert PDF to page images
        try:
            pages = self.converter.convert(pdf_path)
        except ValueError as e:
            raise ValueError(f"Failed to convert PDF: {e}")

        # Create session
        session = ReviewSession(
            pdf_path=str(pdf_path),
            config=config,
            pages=pages,
            created_at=datetime.now(UTC).isoformat(),
        )

        return session

    def run_pass1(self, session: ReviewSession) -> PaperStructure:
        """
        Run Pass 1: Structure analysis.

        Args:
            session: ReviewSession with pages loaded

        Returns:
            PaperStructure with extracted structural information

        Raises:
            ValueError: If session has no pages
        """
        if not session.pages:
            raise ValueError("Session has no pages loaded")

        # Run structure analysis
        structure = self.analyzer.analyze_structure(session.pages)

        # Update session
        session.structure = structure

        return structure

    def run_pass2(
        self, session: ReviewSession, user_feedback: str | None = None, profile: ReviewerProfile | None = None
    ) -> SectionAnalysis:
        """
        Run Pass 2: Section-level analysis.

        Args:
            session: ReviewSession with structure from Pass 1
            user_feedback: Optional user feedback from Pass 1
            profile: Optional ReviewerProfile for customization

        Returns:
            SectionAnalysis with detailed section evaluations

        Raises:
            ValueError: If session has no structure or pages
        """
        if not session.structure:
            raise ValueError("Session has no structure from Pass 1")

        if not session.pages:
            raise ValueError("Session has no pages loaded")

        # Store user feedback
        session.pass1_feedback = user_feedback

        # Run section analysis
        section_analysis = self.analyzer.analyze_sections(
            structure=session.structure,
            pages=session.pages,
            profile=profile,
        )

        # Update session
        session.section_analysis = section_analysis

        return section_analysis

    def run_pass3(self, session: ReviewSession, user_feedback: str | None = None) -> OverallAssessment:
        """
        Run Pass 3: Overall assessment.

        Args:
            session: ReviewSession with section_analysis from Pass 2
            user_feedback: Optional user feedback from Pass 2

        Returns:
            OverallAssessment with scores, recommendation, and summary

        Raises:
            ValueError: If session has no section_analysis
        """
        if not session.section_analysis:
            raise ValueError("Session has no section_analysis from Pass 2")

        # Store user feedback
        session.pass2_feedback = user_feedback

        # Collect all user inputs so far
        user_inputs = []
        if session.pass1_feedback:
            user_inputs.append(f"[After Pass 1] {session.pass1_feedback}")
        if user_feedback:
            user_inputs.append(f"[After Pass 2] {user_feedback}")

        # Run overall assessment
        overall_assessment = self.analyzer.generate_assessment(
            section_analysis=session.section_analysis,
            user_inputs=user_inputs if user_inputs else None,
        )

        # Update session
        session.overall_assessment = overall_assessment

        return overall_assessment

    def finalize(self, session: ReviewSession, user_feedback: str | None = None) -> ReviewDocument:
        """
        Finalize the review by generating the final document.

        Args:
            session: ReviewSession with overall_assessment from Pass 3
            user_feedback: Optional user feedback from Pass 3

        Returns:
            ReviewDocument with formatted review content

        Raises:
            ValueError: If session has no overall_assessment
        """
        if not session.overall_assessment:
            raise ValueError("Session has no overall_assessment from Pass 3")

        # Store user feedback
        session.pass3_feedback = user_feedback

        # Collect all user inputs
        user_inputs = []
        if session.pass1_feedback:
            user_inputs.append(f"[After Pass 1] {session.pass1_feedback}")
        if session.pass2_feedback:
            user_inputs.append(f"[After Pass 2] {session.pass2_feedback}")
        if user_feedback:
            user_inputs.append(f"[After Pass 3] {user_feedback}")

        # Generate review document
        review_doc = self.generator.generate_review(
            assessment=session.overall_assessment,
            user_inputs=user_inputs if user_inputs else None,
            config=session.config,
        )

        # Populate metadata and structure in the review document
        review_doc.structure = session.structure
        review_doc.section_analysis = session.section_analysis

        # Update session
        session.final_review = review_doc
        session.completed_at = datetime.now(UTC).isoformat()

        return review_doc
