"""Template Models.

Data models for ML/DL experiment configuration and results.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Any

from pydantic import BaseModel, Field

# =============================================================================
# Hyperparameter Space
# =============================================================================


class SearchStrategy(str, Enum):
    """Hyperparameter search strategy."""

    GRID = "grid"  # Grid search
    RANDOM = "random"  # Random search
    BAYESIAN = "bayesian"  # Bayesian optimization
    OPTUNA = "optuna"  # Optuna-based
    MANUAL = "manual"  # Manual selection


class ParameterDistribution(str, Enum):
    """Parameter distribution for search."""

    UNIFORM = "uniform"
    LOG_UNIFORM = "log_uniform"
    NORMAL = "normal"
    LOG_NORMAL = "log_normal"
    CATEGORICAL = "categorical"
    INT_UNIFORM = "int_uniform"


@dataclass
class HyperparameterRange:
    """Range definition for a hyperparameter.

    Attributes:
        name: Parameter name
        distribution: Distribution type
        low: Minimum value (for continuous/int)
        high: Maximum value (for continuous/int)
        choices: Possible values (for categorical)
        log_scale: Whether to use log scale
        step: Step size for grid search
    """

    name: str
    distribution: ParameterDistribution
    low: float | None = None
    high: float | None = None
    choices: list[Any] | None = None
    log_scale: bool = False
    step: float | None = None


@dataclass
class HyperparameterSpace:
    """Hyperparameter search space.

    Attributes:
        strategy: Search strategy to use
        n_trials: Number of trials for search
        timeout: Maximum time in seconds
        parameters: List of parameter ranges
        constraints: Constraints between parameters
        early_stopping: Whether to use early stopping
        pruner: Pruner configuration (for Optuna)
    """

    strategy: SearchStrategy = SearchStrategy.RANDOM
    n_trials: int = 20
    timeout: int | None = None
    parameters: list[HyperparameterRange] = field(default_factory=list)
    constraints: list[str] = field(default_factory=list)
    early_stopping: bool = True
    pruner: dict[str, Any] | None = None


# =============================================================================
# Metric Definitions
# =============================================================================


class MetricDirection(str, Enum):
    """Optimization direction for a metric."""

    MAXIMIZE = "maximize"
    MINIMIZE = "minimize"


@dataclass
class MetricDefinition:
    """Definition of an evaluation metric.

    Attributes:
        name: Metric name (e.g., 'accuracy', 'f1_score')
        display_name: Human-readable name
        direction: Optimization direction
        primary: Whether this is the primary metric
        format: Display format (e.g., '.2f', '.4f', '.2%')
        threshold: Success threshold
        compute_fn: Optional computation function name
    """

    name: str
    display_name: str
    direction: MetricDirection = MetricDirection.MAXIMIZE
    primary: bool = False
    format: str = ".4f"
    threshold: float | None = None
    compute_fn: str | None = None


# =============================================================================
# Experiment Configuration
# =============================================================================


class DataSplit(str, Enum):
    """Data split types."""

    TRAIN = "train"
    VALIDATION = "validation"
    TEST = "test"


@dataclass
class DataConfig:
    """Data configuration for experiment.

    Attributes:
        train_path: Path to training data
        val_path: Path to validation data
        test_path: Path to test data
        data_format: Data format (csv, parquet, images, etc.)
        preprocessing: Preprocessing configuration
        augmentation: Augmentation configuration
        batch_size: Batch size
        num_workers: DataLoader workers
    """

    train_path: str
    val_path: str | None = None
    test_path: str | None = None
    data_format: str = "auto"
    preprocessing: dict[str, Any] = field(default_factory=dict)
    augmentation: dict[str, Any] = field(default_factory=dict)
    batch_size: int = 32
    num_workers: int = 4


@dataclass
class ModelConfig:
    """Model configuration for experiment.

    Attributes:
        architecture: Model architecture name
        backbone: Backbone network (for transfer learning)
        pretrained: Whether to use pretrained weights
        num_classes: Number of output classes
        hidden_dims: Hidden layer dimensions
        dropout: Dropout rate
        custom_head: Custom head configuration
    """

    architecture: str
    backbone: str | None = None
    pretrained: bool = True
    num_classes: int | None = None
    hidden_dims: list[int] = field(default_factory=list)
    dropout: float = 0.0
    custom_head: dict[str, Any] | None = None


@dataclass
class TrainingConfig:
    """Training configuration for experiment.

    Attributes:
        epochs: Number of training epochs
        learning_rate: Initial learning rate
        optimizer: Optimizer name
        optimizer_params: Additional optimizer parameters
        scheduler: LR scheduler name
        scheduler_params: Scheduler parameters
        loss_function: Loss function name
        loss_params: Loss function parameters
        gradient_clip: Gradient clipping value
        mixed_precision: Whether to use mixed precision
        gradient_accumulation: Gradient accumulation steps
    """

    epochs: int = 100
    learning_rate: float = 1e-3
    optimizer: str = "adam"
    optimizer_params: dict[str, Any] = field(default_factory=dict)
    scheduler: str | None = None
    scheduler_params: dict[str, Any] = field(default_factory=dict)
    loss_function: str = "cross_entropy"
    loss_params: dict[str, Any] = field(default_factory=dict)
    gradient_clip: float | None = None
    mixed_precision: bool = False
    gradient_accumulation: int = 1


@dataclass
class ExperimentConfig:
    """Complete experiment configuration.

    Attributes:
        name: Experiment name
        description: Experiment description
        template_id: Source template ID
        data: Data configuration
        model: Model configuration
        training: Training configuration
        metrics: Metric definitions
        hyperparameter_space: Hyperparameter search space
        seed: Random seed for reproducibility
        device: Device to use (cuda, cpu)
        output_dir: Output directory
        tags: Experiment tags
        piq_knowledge_refs: PIQ knowledge references
    """

    name: str
    description: str = ""
    template_id: str | None = None
    data: DataConfig | None = None
    model: ModelConfig | None = None
    training: TrainingConfig | None = None
    metrics: list[MetricDefinition] = field(default_factory=list)
    hyperparameter_space: HyperparameterSpace | None = None
    seed: int = 42
    device: str = "auto"
    output_dir: str = "./experiments"
    tags: list[str] = field(default_factory=list)
    piq_knowledge_refs: list[str] = field(default_factory=list)


# =============================================================================
# Experiment Results
# =============================================================================


@dataclass
class MetricResult:
    """Result for a single metric.

    Attributes:
        name: Metric name
        value: Metric value
        split: Data split (train/val/test)
        epoch: Epoch number (if applicable)
        step: Step number (if applicable)
        timestamp: Measurement timestamp
    """

    name: str
    value: float
    split: DataSplit = DataSplit.VALIDATION
    epoch: int | None = None
    step: int | None = None
    timestamp: datetime | None = None


@dataclass
class CheckpointInfo:
    """Information about a model checkpoint.

    Attributes:
        path: Checkpoint file path
        epoch: Epoch number
        metrics: Metrics at this checkpoint
        is_best: Whether this is the best checkpoint
        created_at: Creation timestamp
    """

    path: str
    epoch: int
    metrics: dict[str, float] = field(default_factory=dict)
    is_best: bool = False
    created_at: datetime | None = None


@dataclass
class ExperimentResult:
    """Complete experiment results.

    Attributes:
        experiment_id: Unique experiment ID
        config: Experiment configuration
        status: Experiment status
        best_metrics: Best metric values achieved
        final_metrics: Final metric values
        training_history: Training history
        checkpoints: Model checkpoints
        artifacts: Generated artifacts (paths)
        started_at: Experiment start time
        finished_at: Experiment finish time
        error: Error message if failed
        piq_insights: Insights from PIQ analysis
    """

    experiment_id: str
    config: ExperimentConfig
    status: str = "pending"  # pending, running, completed, failed
    best_metrics: dict[str, float] = field(default_factory=dict)
    final_metrics: dict[str, float] = field(default_factory=dict)
    training_history: list[MetricResult] = field(default_factory=list)
    checkpoints: list[CheckpointInfo] = field(default_factory=list)
    artifacts: dict[str, str] = field(default_factory=dict)
    started_at: datetime | None = None
    finished_at: datetime | None = None
    error: str | None = None
    piq_insights: list[str] = field(default_factory=list)


# =============================================================================
# Pydantic Models for API
# =============================================================================


class ExperimentConfigSchema(BaseModel):
    """Pydantic schema for experiment configuration."""

    name: str
    description: str = ""
    template_id: str | None = None
    seed: int = 42
    device: str = "auto"
    output_dir: str = "./experiments"
    tags: list[str] = Field(default_factory=list)

    # Nested configs as dicts for flexibility
    data: dict[str, Any] = Field(default_factory=dict)
    model: dict[str, Any] = Field(default_factory=dict)
    training: dict[str, Any] = Field(default_factory=dict)
    metrics: list[dict[str, Any]] = Field(default_factory=list)
    hyperparameter_space: dict[str, Any] | None = None


class ExperimentResultSchema(BaseModel):
    """Pydantic schema for experiment results."""

    experiment_id: str
    status: str
    best_metrics: dict[str, float] = Field(default_factory=dict)
    final_metrics: dict[str, float] = Field(default_factory=dict)
    started_at: datetime | None = None
    finished_at: datetime | None = None
    error: str | None = None
