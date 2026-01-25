"""Tests for template models."""

from datetime import datetime

import pytest

from c4.templates.models import (
    CheckpointInfo,
    DataConfig,
    DataSplit,
    ExperimentConfig,
    ExperimentConfigSchema,
    ExperimentResult,
    ExperimentResultSchema,
    HyperparameterRange,
    HyperparameterSpace,
    MetricDefinition,
    MetricDirection,
    MetricResult,
    ModelConfig,
    ParameterDistribution,
    SearchStrategy,
    TrainingConfig,
)


class TestSearchStrategy:
    """Tests for SearchStrategy enum."""

    def test_strategy_values(self):
        """Test all search strategies exist."""
        assert SearchStrategy.GRID == "grid"
        assert SearchStrategy.RANDOM == "random"
        assert SearchStrategy.BAYESIAN == "bayesian"
        assert SearchStrategy.OPTUNA == "optuna"
        assert SearchStrategy.MANUAL == "manual"


class TestParameterDistribution:
    """Tests for ParameterDistribution enum."""

    def test_distribution_values(self):
        """Test all distributions exist."""
        assert ParameterDistribution.UNIFORM == "uniform"
        assert ParameterDistribution.LOG_UNIFORM == "log_uniform"
        assert ParameterDistribution.NORMAL == "normal"
        assert ParameterDistribution.CATEGORICAL == "categorical"
        assert ParameterDistribution.INT_UNIFORM == "int_uniform"


class TestHyperparameterRange:
    """Tests for HyperparameterRange dataclass."""

    def test_continuous_range(self):
        """Test continuous hyperparameter range."""
        hp_range = HyperparameterRange(
            name="learning_rate",
            distribution=ParameterDistribution.LOG_UNIFORM,
            low=1e-5,
            high=1e-1,
            log_scale=True,
        )
        assert hp_range.name == "learning_rate"
        assert hp_range.low == 1e-5
        assert hp_range.high == 1e-1
        assert hp_range.log_scale is True

    def test_categorical_range(self):
        """Test categorical hyperparameter range."""
        hp_range = HyperparameterRange(
            name="optimizer",
            distribution=ParameterDistribution.CATEGORICAL,
            choices=["adam", "sgd", "adamw"],
        )
        assert hp_range.choices == ["adam", "sgd", "adamw"]

    def test_integer_range(self):
        """Test integer hyperparameter range."""
        hp_range = HyperparameterRange(
            name="batch_size",
            distribution=ParameterDistribution.INT_UNIFORM,
            low=8,
            high=128,
            step=8,
        )
        assert hp_range.step == 8


class TestHyperparameterSpace:
    """Tests for HyperparameterSpace dataclass."""

    def test_default_space(self):
        """Test default hyperparameter space."""
        space = HyperparameterSpace()
        assert space.strategy == SearchStrategy.RANDOM
        assert space.n_trials == 20
        assert space.early_stopping is True

    def test_custom_space(self):
        """Test custom hyperparameter space."""
        params = [
            HyperparameterRange(
                name="lr",
                distribution=ParameterDistribution.LOG_UNIFORM,
                low=1e-5,
                high=1e-2,
            ),
            HyperparameterRange(
                name="batch_size",
                distribution=ParameterDistribution.CATEGORICAL,
                choices=[16, 32, 64],
            ),
        ]
        space = HyperparameterSpace(
            strategy=SearchStrategy.OPTUNA,
            n_trials=100,
            timeout=3600,
            parameters=params,
            early_stopping=True,
        )
        assert space.strategy == SearchStrategy.OPTUNA
        assert space.n_trials == 100
        assert len(space.parameters) == 2


class TestMetricDefinition:
    """Tests for MetricDefinition dataclass."""

    def test_maximize_metric(self):
        """Test metric to maximize."""
        metric = MetricDefinition(
            name="accuracy",
            display_name="Accuracy",
            direction=MetricDirection.MAXIMIZE,
            primary=True,
            format=".2%",
            threshold=0.9,
        )
        assert metric.direction == MetricDirection.MAXIMIZE
        assert metric.primary is True
        assert metric.threshold == 0.9

    def test_minimize_metric(self):
        """Test metric to minimize."""
        metric = MetricDefinition(
            name="loss",
            display_name="Loss",
            direction=MetricDirection.MINIMIZE,
            format=".4f",
        )
        assert metric.direction == MetricDirection.MINIMIZE
        assert metric.primary is False


class TestDataConfig:
    """Tests for DataConfig dataclass."""

    def test_minimal_config(self):
        """Test minimal data configuration."""
        config = DataConfig(train_path="/data/train")
        assert config.train_path == "/data/train"
        assert config.batch_size == 32
        assert config.num_workers == 4

    def test_full_config(self):
        """Test full data configuration."""
        config = DataConfig(
            train_path="/data/train",
            val_path="/data/val",
            test_path="/data/test",
            data_format="parquet",
            preprocessing={"normalize": True},
            augmentation={"random_flip": True},
            batch_size=64,
            num_workers=8,
        )
        assert config.val_path == "/data/val"
        assert config.data_format == "parquet"
        assert config.preprocessing["normalize"] is True


class TestModelConfig:
    """Tests for ModelConfig dataclass."""

    def test_pretrained_config(self):
        """Test pretrained model configuration."""
        config = ModelConfig(
            architecture="resnet50",
            backbone="imagenet",
            pretrained=True,
            num_classes=10,
        )
        assert config.pretrained is True
        assert config.num_classes == 10

    def test_custom_head_config(self):
        """Test model with custom head."""
        config = ModelConfig(
            architecture="efficientnet_b0",
            num_classes=100,
            hidden_dims=[512, 256],
            dropout=0.5,
            custom_head={"type": "mlp", "layers": 2},
        )
        assert config.hidden_dims == [512, 256]
        assert config.dropout == 0.5


class TestTrainingConfig:
    """Tests for TrainingConfig dataclass."""

    def test_default_config(self):
        """Test default training configuration."""
        config = TrainingConfig()
        assert config.epochs == 100
        assert config.learning_rate == 1e-3
        assert config.optimizer == "adam"
        assert config.mixed_precision is False

    def test_advanced_config(self):
        """Test advanced training configuration."""
        config = TrainingConfig(
            epochs=50,
            learning_rate=3e-4,
            optimizer="adamw",
            optimizer_params={"weight_decay": 0.01},
            scheduler="cosine",
            scheduler_params={"T_max": 50},
            loss_function="focal",
            gradient_clip=1.0,
            mixed_precision=True,
            gradient_accumulation=4,
        )
        assert config.optimizer == "adamw"
        assert config.mixed_precision is True
        assert config.gradient_accumulation == 4


class TestExperimentConfig:
    """Tests for ExperimentConfig dataclass."""

    def test_minimal_config(self):
        """Test minimal experiment configuration."""
        config = ExperimentConfig(name="test-experiment")
        assert config.name == "test-experiment"
        assert config.seed == 42
        assert config.device == "auto"

    def test_full_config(self):
        """Test full experiment configuration."""
        data = DataConfig(train_path="/data/train")
        model = ModelConfig(architecture="resnet50", num_classes=10)
        training = TrainingConfig(epochs=100)
        metrics = [
            MetricDefinition(
                name="accuracy",
                display_name="Accuracy",
                direction=MetricDirection.MAXIMIZE,
                primary=True,
            )
        ]

        config = ExperimentConfig(
            name="full-experiment",
            description="A full experiment",
            template_id="image-classification",
            data=data,
            model=model,
            training=training,
            metrics=metrics,
            seed=123,
            device="cuda",
            output_dir="./experiments/full",
            tags=["test", "resnet"],
            piq_knowledge_refs=["arch.resnet"],
        )

        assert config.template_id == "image-classification"
        assert config.data.train_path == "/data/train"
        assert config.model.num_classes == 10
        assert len(config.metrics) == 1


class TestMetricResult:
    """Tests for MetricResult dataclass."""

    def test_metric_result(self):
        """Test metric result creation."""
        result = MetricResult(
            name="accuracy",
            value=0.95,
            split=DataSplit.VALIDATION,
            epoch=10,
            step=1000,
            timestamp=datetime.now(),
        )
        assert result.value == 0.95
        assert result.split == DataSplit.VALIDATION
        assert result.epoch == 10


class TestCheckpointInfo:
    """Tests for CheckpointInfo dataclass."""

    def test_checkpoint_info(self):
        """Test checkpoint info creation."""
        checkpoint = CheckpointInfo(
            path="/checkpoints/epoch_10.pt",
            epoch=10,
            metrics={"accuracy": 0.95, "loss": 0.1},
            is_best=True,
            created_at=datetime.now(),
        )
        assert checkpoint.epoch == 10
        assert checkpoint.is_best is True
        assert checkpoint.metrics["accuracy"] == 0.95


class TestExperimentResult:
    """Tests for ExperimentResult dataclass."""

    def test_pending_result(self):
        """Test pending experiment result."""
        config = ExperimentConfig(name="test")
        result = ExperimentResult(
            experiment_id="exp-001",
            config=config,
            status="pending",
        )
        assert result.status == "pending"
        assert result.started_at is None

    def test_completed_result(self):
        """Test completed experiment result."""
        config = ExperimentConfig(name="test")
        result = ExperimentResult(
            experiment_id="exp-002",
            config=config,
            status="completed",
            best_metrics={"accuracy": 0.98},
            final_metrics={"accuracy": 0.97},
            started_at=datetime.now(),
            finished_at=datetime.now(),
            piq_insights=["ResNet50 shows best performance"],
        )
        assert result.status == "completed"
        assert result.best_metrics["accuracy"] == 0.98
        assert len(result.piq_insights) == 1

    def test_failed_result(self):
        """Test failed experiment result."""
        config = ExperimentConfig(name="test")
        result = ExperimentResult(
            experiment_id="exp-003",
            config=config,
            status="failed",
            error="CUDA out of memory",
        )
        assert result.status == "failed"
        assert "memory" in result.error.lower()


class TestExperimentConfigSchema:
    """Tests for Pydantic ExperimentConfigSchema."""

    def test_schema_creation(self):
        """Test creating schema from dict."""
        schema = ExperimentConfigSchema(
            name="test-exp",
            description="Test experiment",
            seed=42,
            data={"train_path": "/data/train"},
            model={"architecture": "resnet50"},
            training={"epochs": 100},
        )
        assert schema.name == "test-exp"
        assert schema.data["train_path"] == "/data/train"

    def test_schema_serialization(self):
        """Test schema can be serialized to dict."""
        schema = ExperimentConfigSchema(
            name="test-exp",
            tags=["test"],
        )
        data = schema.model_dump()
        assert data["name"] == "test-exp"
        assert "tags" in data


class TestExperimentResultSchema:
    """Tests for Pydantic ExperimentResultSchema."""

    def test_schema_creation(self):
        """Test creating result schema."""
        schema = ExperimentResultSchema(
            experiment_id="exp-001",
            status="completed",
            best_metrics={"accuracy": 0.95},
            final_metrics={"accuracy": 0.94},
        )
        assert schema.experiment_id == "exp-001"
        assert schema.status == "completed"

    def test_schema_serialization(self):
        """Test schema can be serialized."""
        schema = ExperimentResultSchema(
            experiment_id="exp-001",
            status="running",
        )
        data = schema.model_dump()
        assert "experiment_id" in data
        assert "status" in data
