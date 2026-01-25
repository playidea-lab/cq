"""Image Classification Template.

Provides a complete template for image classification experiments
with CNN/ViT architectures and transfer learning support.
"""

from __future__ import annotations

from typing import Any

from c4.templates.base import (
    ParameterType,
    Template,
    TemplateCategory,
    TemplateConfig,
    TemplateParameter,
    TemplateRegistry,
    TemplateValidation,
)


@TemplateRegistry.register
class ImageClassificationTemplate(Template):
    """Image Classification experiment template.

    Supports:
    - CNN architectures (ResNet, EfficientNet, etc.)
    - ViT architectures (ViT, DeiT, Swin, etc.)
    - Transfer learning with pretrained backbones
    - Data augmentation strategies
    - Learning rate schedulers
    - Mixed precision training
    """

    @property
    def config(self) -> TemplateConfig:
        """Get template configuration."""
        return TemplateConfig(
            id="image-classification",
            name="Image Classification",
            version="1.0.0",
            category=TemplateCategory.CLASSIFICATION,
            description="Train image classification models with CNN or ViT architectures",
            author="C4 Team",
            tags=["image", "classification", "cnn", "vit", "transfer-learning"],
            parameters=[
                # Data parameters
                TemplateParameter(
                    name="data_path",
                    param_type=ParameterType.PATH,
                    description="Path to image dataset (ImageFolder format)",
                    required=True,
                ),
                TemplateParameter(
                    name="image_size",
                    param_type=ParameterType.INTEGER,
                    description="Input image size (square)",
                    default=224,
                    min_value=32,
                    max_value=1024,
                ),
                TemplateParameter(
                    name="num_classes",
                    param_type=ParameterType.INTEGER,
                    description="Number of output classes",
                    required=True,
                    min_value=2,
                ),
                # Model parameters
                TemplateParameter(
                    name="backbone",
                    param_type=ParameterType.MODEL,
                    description="Backbone architecture",
                    default="resnet50",
                    choices=[
                        "resnet18",
                        "resnet34",
                        "resnet50",
                        "resnet101",
                        "efficientnet_b0",
                        "efficientnet_b1",
                        "efficientnet_b2",
                        "efficientnet_b3",
                        "vit_base_patch16_224",
                        "vit_small_patch16_224",
                        "swin_tiny_patch4_window7_224",
                        "swin_small_patch4_window7_224",
                    ],
                    piq_knowledge_ref="backbone",
                ),
                TemplateParameter(
                    name="pretrained",
                    param_type=ParameterType.BOOLEAN,
                    description="Use pretrained weights",
                    default=True,
                ),
                TemplateParameter(
                    name="freeze_backbone",
                    param_type=ParameterType.BOOLEAN,
                    description="Freeze backbone weights initially",
                    default=False,
                ),
                # Training parameters
                TemplateParameter(
                    name="batch_size",
                    param_type=ParameterType.INTEGER,
                    description="Training batch size",
                    default=32,
                    min_value=1,
                    max_value=512,
                    piq_knowledge_ref="batch_size",
                ),
                TemplateParameter(
                    name="epochs",
                    param_type=ParameterType.INTEGER,
                    description="Number of training epochs",
                    default=100,
                    min_value=1,
                ),
                TemplateParameter(
                    name="learning_rate",
                    param_type=ParameterType.FLOAT,
                    description="Initial learning rate",
                    default=1e-3,
                    min_value=1e-7,
                    max_value=1.0,
                    piq_knowledge_ref="learning_rate",
                ),
                TemplateParameter(
                    name="optimizer",
                    param_type=ParameterType.CHOICE,
                    description="Optimizer",
                    default="adamw",
                    choices=["sgd", "adam", "adamw", "rmsprop"],
                    piq_knowledge_ref="optimizer",
                ),
                TemplateParameter(
                    name="scheduler",
                    param_type=ParameterType.CHOICE,
                    description="Learning rate scheduler",
                    default="cosine",
                    choices=["none", "step", "cosine", "onecycle", "plateau"],
                    piq_knowledge_ref="scheduler",
                ),
                TemplateParameter(
                    name="weight_decay",
                    param_type=ParameterType.FLOAT,
                    description="Weight decay (L2 regularization)",
                    default=1e-4,
                    min_value=0.0,
                    max_value=1.0,
                ),
                # Augmentation
                TemplateParameter(
                    name="augmentation",
                    param_type=ParameterType.CHOICE,
                    description="Data augmentation strategy",
                    default="standard",
                    choices=["none", "standard", "autoaugment", "randaugment", "trivialaugment"],
                    piq_knowledge_ref="augmentation",
                ),
                # Advanced
                TemplateParameter(
                    name="mixed_precision",
                    param_type=ParameterType.BOOLEAN,
                    description="Use mixed precision training (FP16)",
                    default=True,
                ),
                TemplateParameter(
                    name="label_smoothing",
                    param_type=ParameterType.FLOAT,
                    description="Label smoothing factor",
                    default=0.1,
                    min_value=0.0,
                    max_value=0.5,
                ),
            ],
            validations=[
                TemplateValidation(
                    name="lint",
                    command="uv run ruff check .",
                    required=True,
                    description="Code linting",
                ),
                TemplateValidation(
                    name="unit",
                    command="uv run pytest tests/ -v",
                    required=True,
                    description="Unit tests",
                ),
                TemplateValidation(
                    name="typecheck",
                    command="uv run mypy .",
                    required=False,
                    description="Type checking",
                ),
            ],
            piq_knowledge_refs=[
                "classification",
                "transfer_learning",
                "data_augmentation",
            ],
            checkpoints=[
                {
                    "id": "CP-DATA",
                    "name": "Data Pipeline Ready",
                    "description": "Data loading and augmentation working",
                },
                {
                    "id": "CP-MODEL",
                    "name": "Model Architecture Ready",
                    "description": "Model with pretrained backbone configured",
                },
                {
                    "id": "CP-TRAIN",
                    "name": "Training Complete",
                    "description": "Model training finished with evaluation",
                },
            ],
            dependencies=[
                "torch>=2.0.0",
                "torchvision>=0.15.0",
                "timm>=0.9.0",
                "albumentations>=1.3.0",
                "pytorch-lightning>=2.0.0",
            ],
        )

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
        files = {}

        # Main training script
        files["train.py"] = self._generate_train_script(params)

        # Model definition
        files["model.py"] = self._generate_model_script(params)

        # Data loading
        files["data.py"] = self._generate_data_script(params)

        # Configuration
        files["config.yaml"] = self._generate_config(params)

        # Requirements
        files["requirements.txt"] = self._generate_requirements()

        # README
        files["README.md"] = self._generate_readme(params)

        return files

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
        return {
            "experiment": {
                "name": f"image-classification-{params.get('backbone', 'resnet50')}",
                "template_id": self.id,
            },
            "data": {
                "path": params.get("data_path", ""),
                "image_size": params.get("image_size", 224),
                "num_classes": params.get("num_classes", 10),
                "batch_size": params.get("batch_size", 32),
            },
            "model": {
                "backbone": params.get("backbone", "resnet50"),
                "pretrained": params.get("pretrained", True),
                "freeze_backbone": params.get("freeze_backbone", False),
                "num_classes": params.get("num_classes", 10),
            },
            "training": {
                "epochs": params.get("epochs", 100),
                "learning_rate": params.get("learning_rate", 1e-3),
                "optimizer": params.get("optimizer", "adamw"),
                "scheduler": params.get("scheduler", "cosine"),
                "weight_decay": params.get("weight_decay", 1e-4),
                "mixed_precision": params.get("mixed_precision", True),
                "label_smoothing": params.get("label_smoothing", 0.1),
            },
            "augmentation": {
                "strategy": params.get("augmentation", "standard"),
            },
        }

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
        backbone = params.get("backbone", "resnet50")

        return [
            {
                "id": "T-IC-001",
                "title": "Setup Project Structure",
                "scope": [".", "requirements.txt", "config.yaml"],
                "dod": "Project directory created with dependencies installed",
                "dependencies": [],
            },
            {
                "id": "T-IC-002",
                "title": "Implement Data Pipeline",
                "scope": ["data.py", "tests/test_data.py"],
                "dod": (
                    "1) DataLoader returns batches of correct shape, "
                    "2) Augmentation transforms applied, "
                    "3) Train/val split working"
                ),
                "dependencies": ["T-IC-001"],
            },
            {
                "id": "T-IC-003",
                "title": f"Configure {backbone} Model",
                "scope": ["model.py", "tests/test_model.py"],
                "dod": (
                    f"1) {backbone} loaded with pretrained weights, "
                    "2) Classification head configured, "
                    "3) Forward pass returns correct shape"
                ),
                "dependencies": ["T-IC-001"],
            },
            {
                "id": "T-IC-004",
                "title": "Implement Training Loop",
                "scope": ["train.py"],
                "dod": (
                    "1) Training loop runs without errors, "
                    "2) Loss decreases over epochs, "
                    "3) Checkpoints saved"
                ),
                "dependencies": ["T-IC-002", "T-IC-003"],
            },
            {
                "id": "T-IC-005",
                "title": "Implement Evaluation",
                "scope": ["train.py", "evaluate.py"],
                "dod": (
                    "1) Accuracy/F1 metrics computed on validation set, "
                    "2) Confusion matrix generated, "
                    "3) Best model saved"
                ),
                "dependencies": ["T-IC-004"],
            },
            {
                "id": "T-IC-006",
                "title": "Run Full Training",
                "scope": ["experiments/"],
                "dod": (
                    f"1) Model trained for {params.get('epochs', 100)} epochs, "
                    "2) Training logs saved, "
                    "3) Final accuracy reported"
                ),
                "dependencies": ["T-IC-005"],
            },
        ]

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
        return [
            {
                "id": "CP-IC-DATA",
                "name": "Data Pipeline Ready",
                "description": "Data loading and augmentation verified",
                "required_tasks": ["T-IC-001", "T-IC-002"],
                "required_validations": ["lint", "unit"],
            },
            {
                "id": "CP-IC-MODEL",
                "name": "Model Ready",
                "description": "Model architecture configured and tested",
                "required_tasks": ["T-IC-003"],
                "required_validations": ["lint", "unit"],
            },
            {
                "id": "CP-IC-TRAIN",
                "name": "Training Complete",
                "description": "Model trained and evaluated",
                "required_tasks": ["T-IC-004", "T-IC-005", "T-IC-006"],
                "required_validations": ["lint", "unit"],
            },
        ]

    # =========================================================================
    # Private Methods - File Generation
    # =========================================================================

    def _generate_train_script(self, params: dict[str, Any]) -> str:
        """Generate training script."""
        return f'''"""Training script for Image Classification."""

import torch
import torch.nn as nn
from torch.utils.data import DataLoader
from pathlib import Path

from model import create_model
from data import create_dataloaders


def train(config: dict) -> None:
    """Train the model."""
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")

    # Data
    train_loader, val_loader = create_dataloaders(config)

    # Model
    model = create_model(config).to(device)

    # Loss
    criterion = nn.CrossEntropyLoss(
        label_smoothing={params.get("label_smoothing", 0.1)}
    )

    # Optimizer
    optimizer = torch.optim.AdamW(
        model.parameters(),
        lr={params.get("learning_rate", 1e-3)},
        weight_decay={params.get("weight_decay", 1e-4)},
    )

    # Training loop
    best_acc = 0.0
    for epoch in range({params.get("epochs", 100)}):
        model.train()
        for batch in train_loader:
            images, labels = batch
            images, labels = images.to(device), labels.to(device)

            optimizer.zero_grad()
            outputs = model(images)
            loss = criterion(outputs, labels)
            loss.backward()
            optimizer.step()

        # Validation
        model.eval()
        correct = 0
        total = 0
        with torch.no_grad():
            for images, labels in val_loader:
                images, labels = images.to(device), labels.to(device)
                outputs = model(images)
                _, predicted = outputs.max(1)
                total += labels.size(0)
                correct += predicted.eq(labels).sum().item()

        acc = 100.0 * correct / total
        print(f"Epoch {{epoch+1}}: Accuracy = {{acc:.2f}}%")

        if acc > best_acc:
            best_acc = acc
            torch.save(model.state_dict(), "best_model.pth")


if __name__ == "__main__":
    import yaml
    with open("config.yaml") as f:
        config = yaml.safe_load(f)
    train(config)
'''

    def _generate_model_script(self, params: dict[str, Any]) -> str:
        """Generate model script."""
        backbone = params.get("backbone", "resnet50")
        num_classes = params.get("num_classes", 10)

        return f'''"""Model definition for Image Classification."""

import torch
import torch.nn as nn
import timm


def create_model(config: dict) -> nn.Module:
    """Create the classification model.

    Args:
        config: Configuration dictionary

    Returns:
        PyTorch model
    """
    model = timm.create_model(
        "{backbone}",
        pretrained={params.get("pretrained", True)},
        num_classes={num_classes},
    )

    if config.get("model", {{}}).get("freeze_backbone", False):
        for param in model.parameters():
            param.requires_grad = False
        # Unfreeze classifier
        for param in model.get_classifier().parameters():
            param.requires_grad = True

    return model
'''

    def _generate_data_script(self, params: dict[str, Any]) -> str:
        """Generate data loading script."""
        image_size = params.get("image_size", 224)

        return f'''"""Data loading for Image Classification."""

import torch
from torch.utils.data import DataLoader
from torchvision import datasets, transforms
from pathlib import Path


def get_transforms(config: dict, is_train: bool = True):
    """Get data transforms."""
    image_size = config.get("data", {{}}).get("image_size", {image_size})

    if is_train:
        return transforms.Compose([
            transforms.RandomResizedCrop(image_size),
            transforms.RandomHorizontalFlip(),
            transforms.ToTensor(),
            transforms.Normalize(
                mean=[0.485, 0.456, 0.406],
                std=[0.229, 0.224, 0.225],
            ),
        ])
    else:
        return transforms.Compose([
            transforms.Resize(int(image_size * 1.14)),
            transforms.CenterCrop(image_size),
            transforms.ToTensor(),
            transforms.Normalize(
                mean=[0.485, 0.456, 0.406],
                std=[0.229, 0.224, 0.225],
            ),
        ])


def create_dataloaders(config: dict) -> tuple[DataLoader, DataLoader]:
    """Create train and validation dataloaders."""
    data_path = Path(config.get("data", {{}}).get("path", "."))
    batch_size = config.get("data", {{}}).get("batch_size", 32)

    train_dataset = datasets.ImageFolder(
        data_path / "train",
        transform=get_transforms(config, is_train=True),
    )

    val_dataset = datasets.ImageFolder(
        data_path / "val",
        transform=get_transforms(config, is_train=False),
    )

    train_loader = DataLoader(
        train_dataset,
        batch_size=batch_size,
        shuffle=True,
        num_workers=4,
        pin_memory=True,
    )

    val_loader = DataLoader(
        val_dataset,
        batch_size=batch_size,
        shuffle=False,
        num_workers=4,
        pin_memory=True,
    )

    return train_loader, val_loader
'''

    def _generate_config(self, params: dict[str, Any]) -> str:
        """Generate config YAML."""
        import yaml

        config = self.generate_config(params)
        return yaml.dump(config, default_flow_style=False, sort_keys=False)

    def _generate_requirements(self) -> str:
        """Generate requirements.txt."""
        return "\n".join(self.config.dependencies)

    def _generate_readme(self, params: dict[str, Any]) -> str:
        """Generate README.md."""
        backbone = params.get("backbone", "resnet50")

        return f'''# Image Classification with {backbone}

This project was generated using the C4 Image Classification template.

## Setup

```bash
pip install -r requirements.txt
```

## Training

```bash
python train.py
```

## Configuration

Edit `config.yaml` to modify training parameters.

## Generated by C4 Template Library
'''
