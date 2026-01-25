"""Tests for Template base classes."""

from typing import Any

import pytest

from c4.templates.base import (
    ParameterType,
    Template,
    TemplateCategory,
    TemplateConfig,
    TemplateParameter,
    TemplateRegistry,
    TemplateValidation,
)


class TestTemplateCategory:
    """Tests for TemplateCategory enum."""

    def test_category_values(self):
        """Test that all expected categories exist."""
        assert TemplateCategory.CLASSIFICATION == "classification"
        assert TemplateCategory.DETECTION == "detection"
        assert TemplateCategory.LLM == "llm"
        assert TemplateCategory.NLP == "nlp"
        assert TemplateCategory.GENERATION == "generation"

    def test_category_is_string_enum(self):
        """Test that category can be used as string."""
        assert TemplateCategory.CLASSIFICATION.value == "classification"


class TestParameterType:
    """Tests for ParameterType enum."""

    def test_parameter_type_values(self):
        """Test that all expected parameter types exist."""
        assert ParameterType.STRING == "string"
        assert ParameterType.INTEGER == "integer"
        assert ParameterType.FLOAT == "float"
        assert ParameterType.BOOLEAN == "boolean"
        assert ParameterType.CHOICE == "choice"
        assert ParameterType.PATH == "path"
        assert ParameterType.MODEL == "model"
        assert ParameterType.DATASET == "dataset"


class TestTemplateParameter:
    """Tests for TemplateParameter dataclass."""

    def test_required_parameter(self):
        """Test creating a required parameter."""
        param = TemplateParameter(
            name="learning_rate",
            param_type=ParameterType.FLOAT,
            description="Learning rate for optimizer",
            required=True,
        )
        assert param.name == "learning_rate"
        assert param.param_type == ParameterType.FLOAT
        assert param.required is True
        assert param.default is None

    def test_optional_parameter_with_default(self):
        """Test creating an optional parameter with default."""
        param = TemplateParameter(
            name="batch_size",
            param_type=ParameterType.INTEGER,
            description="Batch size for training",
            default=32,
            required=False,
        )
        assert param.default == 32
        assert param.required is False

    def test_choice_parameter(self):
        """Test creating a choice parameter."""
        param = TemplateParameter(
            name="optimizer",
            param_type=ParameterType.CHOICE,
            description="Optimizer type",
            choices=["adam", "sgd", "adamw"],
            default="adam",
        )
        assert param.choices == ["adam", "sgd", "adamw"]
        assert param.default == "adam"

    def test_numeric_parameter_with_bounds(self):
        """Test parameter with min/max bounds."""
        param = TemplateParameter(
            name="dropout",
            param_type=ParameterType.FLOAT,
            description="Dropout rate",
            min_value=0.0,
            max_value=1.0,
            default=0.5,
        )
        assert param.min_value == 0.0
        assert param.max_value == 1.0

    def test_piq_knowledge_ref(self):
        """Test parameter with PIQ knowledge reference."""
        param = TemplateParameter(
            name="architecture",
            param_type=ParameterType.MODEL,
            description="Model architecture",
            piq_knowledge_ref="model-architectures.image-classification",
        )
        assert param.piq_knowledge_ref == "model-architectures.image-classification"


class TestTemplateValidation:
    """Tests for TemplateValidation dataclass."""

    def test_required_validation(self):
        """Test creating a required validation."""
        validation = TemplateValidation(
            name="lint",
            command="uv run ruff check .",
            required=True,
            description="Code linting",
        )
        assert validation.name == "lint"
        assert validation.command == "uv run ruff check ."
        assert validation.required is True

    def test_optional_validation(self):
        """Test creating an optional validation."""
        validation = TemplateValidation(
            name="e2e",
            command="uv run pytest tests/e2e/",
            required=False,
        )
        assert validation.required is False


class TestTemplateConfig:
    """Tests for TemplateConfig dataclass."""

    def test_minimal_config(self):
        """Test creating a minimal config."""
        config = TemplateConfig(
            id="test-template",
            name="Test Template",
            version="1.0.0",
            category=TemplateCategory.CLASSIFICATION,
        )
        assert config.id == "test-template"
        assert config.name == "Test Template"
        assert config.version == "1.0.0"
        assert config.category == TemplateCategory.CLASSIFICATION
        assert config.parameters == []
        assert config.tags == []

    def test_full_config(self):
        """Test creating a full config with all fields."""
        params = [
            TemplateParameter(
                name="batch_size",
                param_type=ParameterType.INTEGER,
                description="Batch size",
                default=32,
            )
        ]
        validations = [
            TemplateValidation(name="unit", command="uv run pytest", required=True)
        ]
        config = TemplateConfig(
            id="full-template",
            name="Full Template",
            version="2.0.0",
            category=TemplateCategory.LLM,
            description="A full template with all fields",
            author="C4 Team",
            tags=["llm", "fine-tuning"],
            parameters=params,
            validations=validations,
            piq_knowledge_refs=["llm.fine-tuning"],
            dependencies=["torch", "transformers"],
        )
        assert config.description == "A full template with all fields"
        assert config.author == "C4 Team"
        assert "llm" in config.tags
        assert len(config.parameters) == 1
        assert len(config.validations) == 1
        assert "llm.fine-tuning" in config.piq_knowledge_refs
        assert "torch" in config.dependencies


class MockTemplate(Template):
    """Mock template for testing."""

    def __init__(self, template_id: str = "mock-template"):
        self._template_id = template_id

    @property
    def config(self) -> TemplateConfig:
        return TemplateConfig(
            id=self._template_id,
            name="Mock Template",
            version="1.0.0",
            category=TemplateCategory.CLASSIFICATION,
            description="A mock template for testing",
            parameters=[
                TemplateParameter(
                    name="epochs",
                    param_type=ParameterType.INTEGER,
                    description="Number of epochs",
                    default=10,
                    min_value=1,
                    max_value=1000,
                ),
                TemplateParameter(
                    name="learning_rate",
                    param_type=ParameterType.FLOAT,
                    description="Learning rate",
                    default=0.001,
                    min_value=0.0,
                    max_value=1.0,
                ),
                TemplateParameter(
                    name="optimizer",
                    param_type=ParameterType.CHOICE,
                    description="Optimizer",
                    choices=["adam", "sgd"],
                    default="adam",
                ),
                TemplateParameter(
                    name="use_pretrained",
                    param_type=ParameterType.BOOLEAN,
                    description="Use pretrained weights",
                    default=True,
                    required=False,
                ),
            ],
        )

    def generate_project(
        self, output_dir: str, params: dict[str, Any]
    ) -> dict[str, str]:
        return {"train.py": "# Mock training script"}

    def generate_config(self, params: dict[str, Any]) -> dict[str, Any]:
        return {"epochs": params.get("epochs", 10)}

    def generate_tasks(self, params: dict[str, Any]) -> list[dict[str, Any]]:
        return [{"id": "T-001", "title": "Setup environment"}]

    def generate_checkpoints(self, params: dict[str, Any]) -> list[dict[str, Any]]:
        return [{"id": "CP-001", "description": "Training complete"}]


class TestTemplate:
    """Tests for Template abstract class."""

    def test_template_properties(self):
        """Test template property accessors."""
        template = MockTemplate()
        assert template.id == "mock-template"
        assert template.name == "Mock Template"
        assert template.version == "1.0.0"
        assert template.category == TemplateCategory.CLASSIFICATION

    def test_get_info(self):
        """Test get_info returns TemplateInfo."""
        template = MockTemplate()
        info = template.get_info()

        assert info.id == "mock-template"
        assert info.name == "Mock Template"
        assert info.category == "classification"
        assert len(info.parameters) == 4

    def test_validate_params_valid(self):
        """Test parameter validation with valid params."""
        template = MockTemplate()
        errors = template.validate_params(
            {
                "epochs": 50,
                "learning_rate": 0.01,
                "optimizer": "adam",
            }
        )
        assert errors == []

    def test_validate_params_missing_required(self):
        """Test validation fails for missing required params."""
        template = MockTemplate()
        errors = template.validate_params({})
        assert any("epochs" in e for e in errors)
        assert any("learning_rate" in e for e in errors)
        assert any("optimizer" in e for e in errors)

    def test_validate_params_invalid_integer(self):
        """Test validation fails for invalid integer type."""
        template = MockTemplate()
        errors = template.validate_params(
            {
                "epochs": "not_an_int",
                "learning_rate": 0.01,
                "optimizer": "adam",
            }
        )
        assert any("integer" in e.lower() for e in errors)

    def test_validate_params_out_of_range(self):
        """Test validation fails for out of range values."""
        template = MockTemplate()
        errors = template.validate_params(
            {
                "epochs": 2000,  # max is 1000
                "learning_rate": 0.01,
                "optimizer": "adam",
            }
        )
        assert any("<=" in e for e in errors)

    def test_validate_params_invalid_choice(self):
        """Test validation fails for invalid choice."""
        template = MockTemplate()
        errors = template.validate_params(
            {
                "epochs": 10,
                "learning_rate": 0.01,
                "optimizer": "rmsprop",  # not in choices
            }
        )
        assert any("must be one of" in e.lower() for e in errors)

    def test_validate_params_invalid_boolean(self):
        """Test validation fails for invalid boolean."""
        template = MockTemplate()
        errors = template.validate_params(
            {
                "epochs": 10,
                "learning_rate": 0.01,
                "optimizer": "adam",
                "use_pretrained": "yes",  # should be bool
            }
        )
        assert any("boolean" in e.lower() for e in errors)

    def test_generate_project(self):
        """Test project generation."""
        template = MockTemplate()
        files = template.generate_project("/tmp/test", {"epochs": 10})
        assert "train.py" in files

    def test_generate_tasks(self):
        """Test task generation."""
        template = MockTemplate()
        tasks = template.generate_tasks({"epochs": 10})
        assert len(tasks) >= 1
        assert tasks[0]["id"] == "T-001"

    def test_generate_checkpoints(self):
        """Test checkpoint generation."""
        template = MockTemplate()
        checkpoints = template.generate_checkpoints({"epochs": 10})
        assert len(checkpoints) >= 1
        assert checkpoints[0]["id"] == "CP-001"


class TestTemplateRegistry:
    """Tests for TemplateRegistry."""

    @pytest.fixture(autouse=True)
    def clear_registry(self):
        """Clear registry before each test."""
        # Save original templates
        original = TemplateRegistry._templates.copy()
        yield
        # Restore original templates
        TemplateRegistry._templates = original

    def test_register_template(self):
        """Test registering a template."""

        @TemplateRegistry.register
        class TestRegisterTemplate(MockTemplate):
            @property
            def config(self) -> TemplateConfig:
                cfg = super().config
                return TemplateConfig(
                    id="test-register",
                    name=cfg.name,
                    version=cfg.version,
                    category=cfg.category,
                    parameters=cfg.parameters,
                )

        template = TemplateRegistry.get("test-register")
        assert template is not None
        assert template.id == "test-register"

    def test_get_nonexistent_template(self):
        """Test getting a template that doesn't exist."""
        template = TemplateRegistry.get("nonexistent-template-xyz")
        assert template is None

    def test_list_all_templates(self):
        """Test listing all templates."""
        # Clear and register test template
        TemplateRegistry._templates = {}

        @TemplateRegistry.register
        class ListTestTemplate(MockTemplate):
            @property
            def config(self) -> TemplateConfig:
                cfg = super().config
                return TemplateConfig(
                    id="list-test",
                    name=cfg.name,
                    version=cfg.version,
                    category=cfg.category,
                )

        templates = TemplateRegistry.list_all()
        assert len(templates) >= 1
        assert any(t.id == "list-test" for t in templates)

    def test_list_by_category(self):
        """Test listing templates by category."""
        TemplateRegistry._templates = {}

        @TemplateRegistry.register
        class CategoryTestTemplate(MockTemplate):
            @property
            def config(self) -> TemplateConfig:
                return TemplateConfig(
                    id="category-test",
                    name="Category Test",
                    version="1.0.0",
                    category=TemplateCategory.DETECTION,
                )

        detection_templates = TemplateRegistry.list_by_category(
            TemplateCategory.DETECTION
        )
        assert any(t.id == "category-test" for t in detection_templates)

        llm_templates = TemplateRegistry.list_by_category(TemplateCategory.LLM)
        assert not any(t.id == "category-test" for t in llm_templates)

    def test_search_templates(self):
        """Test searching templates."""
        TemplateRegistry._templates = {}

        @TemplateRegistry.register
        class SearchTestTemplate(MockTemplate):
            @property
            def config(self) -> TemplateConfig:
                return TemplateConfig(
                    id="search-test",
                    name="PyTorch Image Classifier",
                    version="1.0.0",
                    category=TemplateCategory.CLASSIFICATION,
                    description="A classifier for images",
                    tags=["pytorch", "cnn", "vision"],
                )

        # Search by name
        results = TemplateRegistry.search("pytorch")
        assert any(t.id == "search-test" for t in results)

        # Search by description
        results = TemplateRegistry.search("classifier")
        assert any(t.id == "search-test" for t in results)

        # Search by tag
        results = TemplateRegistry.search("cnn")
        assert any(t.id == "search-test" for t in results)

        # No results
        results = TemplateRegistry.search("nonexistent-xyz-123")
        assert not any(t.id == "search-test" for t in results)
