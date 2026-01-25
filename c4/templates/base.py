"""Template Base Classes.

Defines the abstract interface for ML/DL experiment templates.
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from enum import Enum
from typing import Any, TypeVar

from pydantic import BaseModel


class TemplateCategory(str, Enum):
    """Categories of ML/DL templates."""

    CLASSIFICATION = "classification"  # Image, text classification
    DETECTION = "detection"  # Object detection, anomaly detection
    SEGMENTATION = "segmentation"  # Semantic, instance segmentation
    GENERATION = "generation"  # Image, text generation
    NLP = "nlp"  # NLP tasks (NER, QA, summarization)
    LLM = "llm"  # LLM fine-tuning, RLHF
    REGRESSION = "regression"  # Prediction, forecasting
    CLUSTERING = "clustering"  # Unsupervised learning
    RECOMMENDATION = "recommendation"  # Recommender systems


class ParameterType(str, Enum):
    """Types of template parameters."""

    STRING = "string"
    INTEGER = "integer"
    FLOAT = "float"
    BOOLEAN = "boolean"
    CHOICE = "choice"  # Enumerated choice
    PATH = "path"  # File/directory path
    MODEL = "model"  # Model architecture name
    DATASET = "dataset"  # Dataset path or identifier


@dataclass
class TemplateParameter:
    """Template parameter definition.

    Attributes:
        name: Parameter name (e.g., 'learning_rate', 'batch_size')
        param_type: Type of the parameter
        description: Human-readable description
        default: Default value
        required: Whether this parameter is required
        choices: Valid choices for CHOICE type
        min_value: Minimum value for numeric types
        max_value: Maximum value for numeric types
        piq_knowledge_ref: Optional PIQ knowledge reference for suggestions
    """

    name: str
    param_type: ParameterType
    description: str
    default: Any = None
    required: bool = True
    choices: list[Any] | None = None
    min_value: float | None = None
    max_value: float | None = None
    piq_knowledge_ref: str | None = None  # Reference to PIQ knowledge for suggestions


@dataclass
class TemplateValidation:
    """Validation rule for template.

    Attributes:
        name: Validation name
        command: Shell command or validation function
        required: Whether this validation must pass
        description: What this validation checks
    """

    name: str
    command: str
    required: bool = True
    description: str = ""


@dataclass
class TemplateConfig:
    """Template configuration.

    Attributes:
        id: Unique template identifier
        name: Human-readable name
        version: Template version
        category: Template category
        description: Template description
        author: Template author
        tags: Searchable tags
        parameters: Template parameters
        validations: Validation rules
        piq_knowledge_refs: PIQ knowledge references for this template
        checkpoints: Checkpoint definitions
        dependencies: Required Python packages
    """

    id: str
    name: str
    version: str
    category: TemplateCategory
    description: str = ""
    author: str = ""
    tags: list[str] = field(default_factory=list)
    parameters: list[TemplateParameter] = field(default_factory=list)
    validations: list[TemplateValidation] = field(default_factory=list)
    piq_knowledge_refs: list[str] = field(default_factory=list)
    checkpoints: list[dict[str, Any]] = field(default_factory=list)
    dependencies: list[str] = field(default_factory=list)


class TemplateInfo(BaseModel):
    """Serializable template information."""

    id: str
    name: str
    version: str
    category: str
    description: str
    author: str
    tags: list[str]
    parameters: list[dict[str, Any]]
    piq_knowledge_refs: list[str]


T = TypeVar("T", bound="Template")


class Template(ABC):
    """Abstract base class for ML/DL experiment templates.

    Each template (Image Classification, Object Detection, etc.) must implement
    this interface to be usable in the C4 template system.

    Example:
        @TemplateRegistry.register
        class ImageClassificationTemplate(Template):
            @property
            def config(self) -> TemplateConfig:
                return TemplateConfig(
                    id="image-classification",
                    name="Image Classification",
                    version="1.0.0",
                    category=TemplateCategory.CLASSIFICATION,
                    # ...
                )
    """

    # =========================================================================
    # Template Identity
    # =========================================================================

    @property
    @abstractmethod
    def config(self) -> TemplateConfig:
        """Get template configuration."""
        pass

    @property
    def id(self) -> str:
        """Template ID."""
        return self.config.id

    @property
    def name(self) -> str:
        """Template name."""
        return self.config.name

    @property
    def version(self) -> str:
        """Template version."""
        return self.config.version

    @property
    def category(self) -> TemplateCategory:
        """Template category."""
        return self.config.category

    def get_info(self) -> TemplateInfo:
        """Get serializable template information."""
        return TemplateInfo(
            id=self.config.id,
            name=self.config.name,
            version=self.config.version,
            category=self.config.category.value,
            description=self.config.description,
            author=self.config.author,
            tags=self.config.tags,
            parameters=[
                {
                    "name": p.name,
                    "type": p.param_type.value,
                    "description": p.description,
                    "default": p.default,
                    "required": p.required,
                    "choices": p.choices,
                }
                for p in self.config.parameters
            ],
            piq_knowledge_refs=self.config.piq_knowledge_refs,
        )

    # =========================================================================
    # Project Generation
    # =========================================================================

    @abstractmethod
    def generate_project(
        self,
        output_dir: str,
        params: dict[str, Any],
    ) -> dict[str, str]:
        """Generate project files from template.

        Args:
            output_dir: Directory to write generated files
            params: Parameter values from user

        Returns:
            Dict mapping file paths to their contents
        """
        pass

    @abstractmethod
    def generate_config(
        self,
        params: dict[str, Any],
    ) -> dict[str, Any]:
        """Generate experiment configuration.

        Args:
            params: Parameter values from user

        Returns:
            Experiment configuration dict
        """
        pass

    # =========================================================================
    # Task Generation
    # =========================================================================

    @abstractmethod
    def generate_tasks(
        self,
        params: dict[str, Any],
    ) -> list[dict[str, Any]]:
        """Generate C4 tasks from template.

        Args:
            params: Parameter values from user

        Returns:
            List of task definitions for C4
        """
        pass

    @abstractmethod
    def generate_checkpoints(
        self,
        params: dict[str, Any],
    ) -> list[dict[str, Any]]:
        """Generate C4 checkpoints from template.

        Args:
            params: Parameter values from user

        Returns:
            List of checkpoint definitions
        """
        pass

    # =========================================================================
    # Validation
    # =========================================================================

    def validate_params(
        self,
        params: dict[str, Any],
    ) -> list[str]:
        """Validate parameter values.

        Args:
            params: Parameter values to validate

        Returns:
            List of validation error messages (empty if valid)
        """
        errors: list[str] = []

        for param in self.config.parameters:
            value = params.get(param.name)

            # Check required
            if param.required and value is None:
                errors.append(f"Missing required parameter: {param.name}")
                continue

            if value is None:
                continue

            # Check type
            if param.param_type == ParameterType.INTEGER:
                if not isinstance(value, int):
                    errors.append(f"{param.name} must be an integer")
                elif param.min_value is not None and value < param.min_value:
                    errors.append(f"{param.name} must be >= {param.min_value}")
                elif param.max_value is not None and value > param.max_value:
                    errors.append(f"{param.name} must be <= {param.max_value}")

            elif param.param_type == ParameterType.FLOAT:
                if not isinstance(value, (int, float)):
                    errors.append(f"{param.name} must be a number")
                elif param.min_value is not None and value < param.min_value:
                    errors.append(f"{param.name} must be >= {param.min_value}")
                elif param.max_value is not None and value > param.max_value:
                    errors.append(f"{param.name} must be <= {param.max_value}")

            elif param.param_type == ParameterType.CHOICE:
                if param.choices and value not in param.choices:
                    errors.append(
                        f"{param.name} must be one of: {', '.join(map(str, param.choices))}"
                    )

            elif param.param_type == ParameterType.BOOLEAN:
                if not isinstance(value, bool):
                    errors.append(f"{param.name} must be a boolean")

        return errors

    # =========================================================================
    # PIQ Integration
    # =========================================================================

    async def get_piq_suggestions(
        self,
        piq_client: Any,  # PIQClient
        param_name: str,
    ) -> list[Any]:
        """Get PIQ knowledge suggestions for a parameter.

        Args:
            piq_client: PIQ client instance
            param_name: Parameter name to get suggestions for

        Returns:
            List of suggested values from PIQ knowledge
        """
        param = next(
            (p for p in self.config.parameters if p.name == param_name),
            None,
        )

        if not param or not param.piq_knowledge_ref:
            return []

        # Query PIQ for suggestions
        result = await piq_client.query(
            knowledge_type=param.piq_knowledge_ref,
            context={"template_id": self.id, "param_name": param_name},
        )

        return result.suggestions if result else []


class TemplateRegistry:
    """Template registry (singleton).

    Manages registration and lookup of templates.
    """

    _templates: dict[str, type[Template]] = {}

    @classmethod
    def register(cls, template_class: type[T]) -> type[T]:
        """Register a template class.

        Usage:
            @TemplateRegistry.register
            class MyTemplate(Template):
                pass
        """
        # Create instance to get ID
        instance = template_class()
        cls._templates[instance.id] = template_class
        return template_class

    @classmethod
    def get(cls, template_id: str) -> Template | None:
        """Get template instance by ID."""
        template_class = cls._templates.get(template_id)
        return template_class() if template_class else None

    @classmethod
    def list_all(cls) -> list[TemplateInfo]:
        """List all registered templates."""
        return [t().get_info() for t in cls._templates.values()]

    @classmethod
    def list_by_category(cls, category: TemplateCategory) -> list[TemplateInfo]:
        """List templates by category."""
        return [
            t().get_info()
            for t in cls._templates.values()
            if t().category == category
        ]

    @classmethod
    def search(cls, query: str) -> list[TemplateInfo]:
        """Search templates by name, description, or tags."""
        query_lower = query.lower()
        results = []

        for template_class in cls._templates.values():
            template = template_class()
            config = template.config

            # Search in name, description, and tags
            if (
                query_lower in config.name.lower()
                or query_lower in config.description.lower()
                or any(query_lower in tag.lower() for tag in config.tags)
            ):
                results.append(template.get_info())

        return results
