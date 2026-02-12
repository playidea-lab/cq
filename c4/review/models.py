"""
Pydantic models for auto_review system.
"""


from pydantic import BaseModel, Field


class PageImage(BaseModel):
    """Represents a single page extracted from a PDF."""
    page_number: int = Field(..., description="Page number (1-indexed)")
    image_data: bytes = Field(..., description="PNG image data")
    width: int = Field(..., description="Image width in pixels")
    height: int = Field(..., description="Image height in pixels")


class PaperMetadata(BaseModel):
    """Metadata extracted from a paper."""
    title: str = Field(..., description="Paper title")
    authors: list[str] = Field(default_factory=list, description="List of authors")
    abstract: str = Field(default="", description="Paper abstract")
    keywords: list[str] = Field(default_factory=list, description="Paper keywords")
    publication_date: str | None = Field(None, description="Publication date if available")
    page_count: int = Field(0, description="Total number of pages")


class Section(BaseModel):
    """Represents a section in the paper."""
    title: str = Field(..., description="Section title")
    page_number: int = Field(..., description="Page number where section starts")
    level: int = Field(1, description="Section level (1=main, 2=subsection, etc.)")


class Figure(BaseModel):
    """Represents a figure in the paper."""
    caption: str = Field(..., description="Figure caption")
    page_number: int = Field(..., description="Page number where figure appears")
    figure_number: str = Field(..., description="Figure number (e.g., 'Figure 1', 'Fig. 2')")


class Table(BaseModel):
    """Represents a table in the paper."""
    caption: str = Field(..., description="Table caption")
    page_number: int = Field(..., description="Page number where table appears")
    table_number: str = Field(..., description="Table number (e.g., 'Table 1')")


class PaperStructure(BaseModel):
    """Structural analysis of the paper."""
    title: str = Field(..., description="Paper title")
    authors: list[str] = Field(default_factory=list, description="List of authors")
    sections: list[Section] = Field(default_factory=list, description="Sections in order")
    figures: list[Figure] = Field(default_factory=list, description="Figures in the paper")
    tables: list[Table] = Field(default_factory=list, description="Tables in the paper")
    equations_count: int = Field(0, description="Number of equations")
    references_count: int = Field(0, description="Number of references")

    # Convenience properties
    @property
    def has_introduction(self) -> bool:
        """Check if paper has introduction section."""
        return any("introduction" in s.title.lower() for s in self.sections)

    @property
    def has_methodology(self) -> bool:
        """Check if paper has methodology section."""
        return any(
            any(keyword in s.title.lower() for keyword in ["method", "approach", "algorithm"])
            for s in self.sections
        )

    @property
    def has_results(self) -> bool:
        """Check if paper has results section."""
        return any(
            any(keyword in s.title.lower() for keyword in ["result", "experiment", "evaluation"])
            for s in self.sections
        )

    @property
    def has_conclusion(self) -> bool:
        """Check if paper has conclusion section."""
        return any("conclusion" in s.title.lower() for s in self.sections)

    @property
    def figure_count(self) -> int:
        """Number of figures."""
        return len(self.figures)

    @property
    def table_count(self) -> int:
        """Number of tables."""
        return len(self.tables)


class SectionEval(BaseModel):
    """Evaluation of a specific section."""
    section_name: str = Field(..., description="Name of the section")
    score: int = Field(..., ge=1, le=10, description="Section quality score (1-10)")
    strengths: list[str] = Field(default_factory=list, description="Identified strengths")
    weaknesses: list[str] = Field(default_factory=list, description="Identified weaknesses")
    specific_comments: list[str] = Field(
        default_factory=list, description="Specific comments and suggestions"
    )


class SectionAnalysis(BaseModel):
    """Analysis of all sections (Pass 2 output)."""
    sections: list[SectionEval] = Field(
        default_factory=list, description="Evaluations for each section"
    )
    overall_notes: str = Field(default="", description="Overall notes on section quality")


class OverallAssessment(BaseModel):
    """Overall assessment of the paper (Pass 3 output)."""
    dimension_scores: dict = Field(
        default_factory=dict,
        description="Scores for different dimensions (novelty, clarity, rigor, significance, etc.)",
    )
    overall_score: float = Field(..., ge=1.0, le=10.0, description="Overall quality score (1-10)")
    recommendation: str = Field(
        ...,
        description="Overall recommendation: Accept, MinorRevision, MajorRevision, or Reject",
    )
    summary: str = Field(..., description="Overall summary of the paper")
    key_strengths: list[str] = Field(default_factory=list, description="Key strengths")
    key_weaknesses: list[str] = Field(default_factory=list, description="Key weaknesses")

    # Legacy fields for backward compatibility
    @property
    def novelty_score(self) -> int:
        """Legacy novelty score (1-5 scale)."""
        return int(self.dimension_scores.get("novelty", 3))

    @property
    def clarity_score(self) -> int:
        """Legacy clarity score (1-5 scale)."""
        return int(self.dimension_scores.get("clarity", 3))

    @property
    def rigor_score(self) -> int:
        """Legacy rigor score (1-5 scale)."""
        return int(self.dimension_scores.get("rigor", 3))

    @property
    def significance_score(self) -> int:
        """Legacy significance score (1-5 scale)."""
        return int(self.dimension_scores.get("significance", 3))

    @property
    def overall_recommendation(self) -> str:
        """Legacy recommendation field."""
        return self.recommendation.lower().replace(" ", "_")


class ReviewDocument(BaseModel):
    """Complete review document."""
    metadata: PaperMetadata | None = Field(None, description="Paper metadata")
    structure: PaperStructure | None = Field(None, description="Paper structure")
    section_analysis: SectionAnalysis | None = Field(None, description="Section-level analysis")
    overall_assessment: OverallAssessment
    reviewer_notes: str = Field(default="", description="Formatted review content (Korean + English)")


class ReviewConfig(BaseModel):
    """Configuration for the review process."""
    model_name: str = Field(default="claude-opus-4", description="Claude model to use")
    max_pages: int | None = Field(None, description="Maximum pages to process")
    focus_areas: list[str] = Field(
        default_factory=lambda: ["novelty", "methodology", "clarity", "significance"],
        description="Areas to focus on during review"
    )
    strictness: float = Field(default=1.0, ge=0.0, le=2.0, description="Review strictness (0-2)")


class ReviewPoint(BaseModel):
    """Represents a review point with frequency tracking."""
    point: str = Field(..., description="The review point text")
    frequency: int = Field(1, description="Number of times this point was made")
    category: str = Field(default="general", description="Category: methodology, clarity, novelty, etc.")


class StyleGuide(BaseModel):
    """Style guide for review generation."""
    tone: str = Field(default="professional", description="Review tone")
    emphasis_points: list[str] = Field(
        default_factory=list, description="Points to emphasize in reviews"
    )
    common_phrases: list[str] = Field(
        default_factory=list, description="Common phrases used in reviews"
    )


class ReviewerProfile(BaseModel):
    """Profile of the reviewer (for customizing review style and tracking patterns)."""
    expertise_areas: list[str] = Field(default_factory=list, description="Areas of expertise")
    review_style: str = Field(
        default="balanced", description="Review style: strict, balanced, lenient"
    )
    focus_on_novelty: bool = Field(True, description="Emphasize novelty in review")
    focus_on_reproducibility: bool = Field(True, description="Emphasize reproducibility")
    review_points: list[ReviewPoint] = Field(
        default_factory=list, description="Frequently made review points"
    )
    style_guide: StyleGuide = Field(
        default_factory=StyleGuide, description="Personal style guide"
    )


class ReviewSession(BaseModel):
    """Session state for a multi-pass review workflow."""
    pdf_path: str = Field(..., description="Path to the PDF being reviewed")
    config: ReviewConfig = Field(default_factory=ReviewConfig, description="Review configuration")

    # Results from each pass
    pages: list[PageImage] | None = Field(None, description="Converted page images")
    structure: PaperStructure | None = Field(None, description="Pass 1 result")
    section_analysis: SectionAnalysis | None = Field(None, description="Pass 2 result")
    overall_assessment: OverallAssessment | None = Field(None, description="Pass 3 result")
    final_review: ReviewDocument | None = Field(None, description="Final review document")

    # User feedback from interactive discussion
    pass1_feedback: str | None = Field(None, description="User feedback after Pass 1")
    pass2_feedback: str | None = Field(None, description="User feedback after Pass 2")
    pass3_feedback: str | None = Field(None, description="User feedback after Pass 3")

    # Metadata
    created_at: str | None = Field(None, description="Session creation timestamp")
    completed_at: str | None = Field(None, description="Session completion timestamp")
