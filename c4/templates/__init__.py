"""C4 Template Library.

Provides ML/DL experiment templates with optional PIQ knowledge integration.
"""

from c4.templates.base import (
    ParameterType,
    Template,
    TemplateCategory,
    TemplateConfig,
    TemplateInfo,
    TemplateParameter,
    TemplateRegistry,
    TemplateValidation,
)

# Import templates to register them
from c4.templates.image_classification import ImageClassificationTemplate
from c4.templates.llm_finetuning import LLMFinetuningTemplate
from c4.templates.models import (
    DataConfig,
    ExperimentConfig,
    ExperimentResult,
    HyperparameterSpace,
    MetricDefinition,
    ModelConfig,
    TrainingConfig,
)
from c4.templates.object_detection import ObjectDetectionTemplate
from c4.templates.piq_protocol import (
    KnowledgeSource,
    KnowledgeType,
    PIQClient,
    PIQConfig,
    PIQKnowledge,
    PIQKnowledgeQuery,
    PIQKnowledgeResult,
    get_piq_client,
)

__all__ = [
    # Base
    "ParameterType",
    "Template",
    "TemplateCategory",
    "TemplateConfig",
    "TemplateInfo",
    "TemplateParameter",
    "TemplateRegistry",
    "TemplateValidation",
    # Models
    "DataConfig",
    "ExperimentConfig",
    "ExperimentResult",
    "HyperparameterSpace",
    "MetricDefinition",
    "ModelConfig",
    "TrainingConfig",
    # PIQ Protocol
    "KnowledgeSource",
    "KnowledgeType",
    "PIQClient",
    "PIQConfig",
    "PIQKnowledge",
    "PIQKnowledgeQuery",
    "PIQKnowledgeResult",
    "get_piq_client",
    # Templates
    "ImageClassificationTemplate",
    "LLMFinetuningTemplate",
    "ObjectDetectionTemplate",
]
