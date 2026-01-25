"""Object Detection Template.

Provides a complete template for object detection experiments
with YOLO, Faster R-CNN, and DETR architectures.
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
class ObjectDetectionTemplate(Template):
    """Object Detection experiment template.

    Supports:
    - YOLO family (YOLOv5, YOLOv8)
    - Faster R-CNN with various backbones
    - DETR and variants
    - COCO and custom dataset formats
    - Multi-GPU training
    """

    @property
    def config(self) -> TemplateConfig:
        """Get template configuration."""
        return TemplateConfig(
            id="object-detection",
            name="Object Detection",
            version="1.0.0",
            category=TemplateCategory.DETECTION,
            description="Train object detection models with YOLO, Faster R-CNN, or DETR",
            author="C4 Team",
            tags=["object", "detection", "yolo", "rcnn", "detr", "coco"],
            parameters=[
                # Data parameters
                TemplateParameter(
                    name="data_path",
                    param_type=ParameterType.PATH,
                    description="Path to dataset (COCO or YOLO format)",
                    required=True,
                ),
                TemplateParameter(
                    name="data_format",
                    param_type=ParameterType.CHOICE,
                    description="Dataset format",
                    default="coco",
                    choices=["coco", "yolo", "voc"],
                ),
                TemplateParameter(
                    name="image_size",
                    param_type=ParameterType.INTEGER,
                    description="Input image size",
                    default=640,
                    min_value=320,
                    max_value=1280,
                ),
                TemplateParameter(
                    name="num_classes",
                    param_type=ParameterType.INTEGER,
                    description="Number of object classes",
                    required=True,
                    min_value=1,
                ),
                # Model parameters
                TemplateParameter(
                    name="architecture",
                    param_type=ParameterType.CHOICE,
                    description="Detection architecture",
                    default="yolov8n",
                    choices=[
                        "yolov5n",
                        "yolov5s",
                        "yolov5m",
                        "yolov5l",
                        "yolov8n",
                        "yolov8s",
                        "yolov8m",
                        "yolov8l",
                        "fasterrcnn_resnet50",
                        "fasterrcnn_resnet101",
                        "detr_resnet50",
                        "detr_resnet101",
                    ],
                    piq_knowledge_ref="architecture",
                ),
                TemplateParameter(
                    name="pretrained",
                    param_type=ParameterType.BOOLEAN,
                    description="Use pretrained weights",
                    default=True,
                ),
                TemplateParameter(
                    name="anchor_sizes",
                    param_type=ParameterType.STRING,
                    description="Anchor sizes (comma-separated, optional)",
                    default="",
                    required=False,
                ),
                # Training parameters
                TemplateParameter(
                    name="batch_size",
                    param_type=ParameterType.INTEGER,
                    description="Training batch size",
                    default=16,
                    min_value=1,
                    max_value=128,
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
                    choices=["sgd", "adam", "adamw"],
                    piq_knowledge_ref="optimizer",
                ),
                TemplateParameter(
                    name="warmup_epochs",
                    param_type=ParameterType.INTEGER,
                    description="Learning rate warmup epochs",
                    default=3,
                    min_value=0,
                    max_value=20,
                ),
                # Augmentation
                TemplateParameter(
                    name="mosaic",
                    param_type=ParameterType.BOOLEAN,
                    description="Use mosaic augmentation",
                    default=True,
                ),
                TemplateParameter(
                    name="mixup",
                    param_type=ParameterType.FLOAT,
                    description="MixUp augmentation probability",
                    default=0.0,
                    min_value=0.0,
                    max_value=1.0,
                ),
                # Advanced
                TemplateParameter(
                    name="mixed_precision",
                    param_type=ParameterType.BOOLEAN,
                    description="Use mixed precision training (FP16)",
                    default=True,
                ),
                TemplateParameter(
                    name="nms_threshold",
                    param_type=ParameterType.FLOAT,
                    description="NMS IoU threshold",
                    default=0.45,
                    min_value=0.1,
                    max_value=0.9,
                ),
                TemplateParameter(
                    name="conf_threshold",
                    param_type=ParameterType.FLOAT,
                    description="Confidence threshold",
                    default=0.25,
                    min_value=0.01,
                    max_value=0.99,
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
            ],
            piq_knowledge_refs=[
                "object_detection",
                "yolo",
                "faster_rcnn",
                "anchor_free",
            ],
            checkpoints=[
                {
                    "id": "CP-DATA",
                    "name": "Data Pipeline Ready",
                    "description": "Data loading with annotations working",
                },
                {
                    "id": "CP-MODEL",
                    "name": "Model Architecture Ready",
                    "description": "Detection model configured",
                },
                {
                    "id": "CP-TRAIN",
                    "name": "Training Complete",
                    "description": "Model trained with mAP evaluation",
                },
            ],
            dependencies=[
                "torch>=2.0.0",
                "torchvision>=0.15.0",
                "ultralytics>=8.0.0",
                "pycocotools>=2.0.0",
                "albumentations>=1.3.0",
            ],
        )

    def generate_project(
        self,
        output_dir: str,
        params: dict[str, Any],
    ) -> dict[str, str]:
        """Generate project files from template."""
        files = {}

        files["train.py"] = self._generate_train_script(params)
        files["model.py"] = self._generate_model_script(params)
        files["data.py"] = self._generate_data_script(params)
        files["config.yaml"] = self._generate_config(params)
        files["requirements.txt"] = self._generate_requirements()
        files["README.md"] = self._generate_readme(params)

        return files

    def generate_config(
        self,
        params: dict[str, Any],
    ) -> dict[str, Any]:
        """Generate experiment configuration."""
        return {
            "experiment": {
                "name": f"object-detection-{params.get('architecture', 'yolov8n')}",
                "template_id": self.id,
            },
            "data": {
                "path": params.get("data_path", ""),
                "format": params.get("data_format", "coco"),
                "image_size": params.get("image_size", 640),
                "num_classes": params.get("num_classes", 80),
            },
            "model": {
                "architecture": params.get("architecture", "yolov8n"),
                "pretrained": params.get("pretrained", True),
                "num_classes": params.get("num_classes", 80),
                "nms_threshold": params.get("nms_threshold", 0.45),
                "conf_threshold": params.get("conf_threshold", 0.25),
            },
            "training": {
                "epochs": params.get("epochs", 100),
                "batch_size": params.get("batch_size", 16),
                "learning_rate": params.get("learning_rate", 1e-3),
                "optimizer": params.get("optimizer", "adamw"),
                "warmup_epochs": params.get("warmup_epochs", 3),
                "mixed_precision": params.get("mixed_precision", True),
            },
            "augmentation": {
                "mosaic": params.get("mosaic", True),
                "mixup": params.get("mixup", 0.0),
            },
        }

    def generate_tasks(
        self,
        params: dict[str, Any],
    ) -> list[dict[str, Any]]:
        """Generate C4 tasks from template."""
        arch = params.get("architecture", "yolov8n")

        return [
            {
                "id": "T-OD-001",
                "title": "Setup Project Structure",
                "scope": [".", "requirements.txt", "config.yaml"],
                "dod": "Project directory created with dependencies installed",
                "dependencies": [],
            },
            {
                "id": "T-OD-002",
                "title": "Prepare Dataset",
                "scope": ["data.py", "tests/test_data.py"],
                "dod": (
                    "1) Dataset loaded in correct format, "
                    "2) Bounding boxes verified, "
                    "3) Train/val split working"
                ),
                "dependencies": ["T-OD-001"],
            },
            {
                "id": "T-OD-003",
                "title": f"Configure {arch} Model",
                "scope": ["model.py", "tests/test_model.py"],
                "dod": (
                    f"1) {arch} model initialized, "
                    "2) Forward pass returns predictions, "
                    "3) NMS working correctly"
                ),
                "dependencies": ["T-OD-001"],
            },
            {
                "id": "T-OD-004",
                "title": "Implement Training Pipeline",
                "scope": ["train.py"],
                "dod": (
                    "1) Training loop runs, "
                    "2) Loss decreases, "
                    "3) Augmentations applied"
                ),
                "dependencies": ["T-OD-002", "T-OD-003"],
            },
            {
                "id": "T-OD-005",
                "title": "Implement mAP Evaluation",
                "scope": ["evaluate.py", "utils/metrics.py"],
                "dod": (
                    "1) COCO mAP computed correctly, "
                    "2) Per-class AP available, "
                    "3) Results logged"
                ),
                "dependencies": ["T-OD-004"],
            },
            {
                "id": "T-OD-006",
                "title": "Run Full Training",
                "scope": ["experiments/"],
                "dod": (
                    f"1) Model trained for {params.get('epochs', 100)} epochs, "
                    "2) mAP@0.5 and mAP@0.5:0.95 reported, "
                    "3) Best model saved"
                ),
                "dependencies": ["T-OD-005"],
            },
        ]

    def generate_checkpoints(
        self,
        params: dict[str, Any],
    ) -> list[dict[str, Any]]:
        """Generate C4 checkpoints from template."""
        return [
            {
                "id": "CP-OD-DATA",
                "name": "Data Pipeline Ready",
                "description": "Dataset loaded with annotations",
                "required_tasks": ["T-OD-001", "T-OD-002"],
                "required_validations": ["lint", "unit"],
            },
            {
                "id": "CP-OD-MODEL",
                "name": "Model Ready",
                "description": "Detection model configured",
                "required_tasks": ["T-OD-003"],
                "required_validations": ["lint", "unit"],
            },
            {
                "id": "CP-OD-TRAIN",
                "name": "Training Complete",
                "description": "Model trained with mAP evaluation",
                "required_tasks": ["T-OD-004", "T-OD-005", "T-OD-006"],
                "required_validations": ["lint", "unit"],
            },
        ]

    # =========================================================================
    # Private Methods - File Generation
    # =========================================================================

    def _generate_train_script(self, params: dict[str, Any]) -> str:
        """Generate training script."""
        arch = params.get("architecture", "yolov8n")

        if arch.startswith("yolov8"):
            return self._generate_yolov8_train(params)
        else:
            return self._generate_torchvision_train(params)

    def _generate_yolov8_train(self, params: dict[str, Any]) -> str:
        """Generate YOLOv8 training script."""
        return f'''"""Training script for YOLOv8 Object Detection."""

from ultralytics import YOLO


def train(config: dict) -> None:
    """Train YOLOv8 model."""
    # Load model
    model = YOLO("{params.get("architecture", "yolov8n")}.pt")

    # Train
    results = model.train(
        data=config["data"]["path"],
        epochs={params.get("epochs", 100)},
        imgsz={params.get("image_size", 640)},
        batch={params.get("batch_size", 16)},
        lr0={params.get("learning_rate", 1e-3)},
        warmup_epochs={params.get("warmup_epochs", 3)},
        mosaic={1.0 if params.get("mosaic", True) else 0.0},
        mixup={params.get("mixup", 0.0)},
        amp={params.get("mixed_precision", True)},
    )

    print(f"Training complete! mAP@0.5: {{results.box.map50:.4f}}")


if __name__ == "__main__":
    import yaml
    with open("config.yaml") as f:
        config = yaml.safe_load(f)
    train(config)
'''

    def _generate_torchvision_train(self, params: dict[str, Any]) -> str:
        """Generate torchvision training script."""
        return f'''"""Training script for Faster R-CNN/DETR Object Detection."""

import torch
import torchvision
from torch.utils.data import DataLoader

from data import create_dataloaders
from model import create_model


def train(config: dict) -> None:
    """Train detection model."""
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")

    # Data
    train_loader, val_loader = create_dataloaders(config)

    # Model
    model = create_model(config).to(device)

    # Optimizer
    optimizer = torch.optim.AdamW(
        model.parameters(),
        lr={params.get("learning_rate", 1e-3)},
    )

    # Training
    for epoch in range({params.get("epochs", 100)}):
        model.train()
        total_loss = 0

        for images, targets in train_loader:
            images = [img.to(device) for img in images]
            targets = [{{k: v.to(device) for k, v in t.items()}} for t in targets]

            optimizer.zero_grad()
            loss_dict = model(images, targets)
            losses = sum(loss for loss in loss_dict.values())
            losses.backward()
            optimizer.step()

            total_loss += losses.item()

        print(f"Epoch {{epoch+1}}: Loss = {{total_loss/len(train_loader):.4f}}")

        # Save checkpoint
        torch.save(model.state_dict(), f"checkpoint_epoch_{{epoch+1}}.pth")


if __name__ == "__main__":
    import yaml
    with open("config.yaml") as f:
        config = yaml.safe_load(f)
    train(config)
'''

    def _generate_model_script(self, params: dict[str, Any]) -> str:
        """Generate model script."""
        arch = params.get("architecture", "yolov8n")
        num_classes = params.get("num_classes", 80)

        if arch.startswith("yolov"):
            return f'''"""Model definition for YOLO Object Detection."""

from ultralytics import YOLO


def create_model(config: dict) -> YOLO:
    """Create YOLO model."""
    model = YOLO("{arch}.pt")
    return model
'''
        else:
            return f'''"""Model definition for Object Detection."""

import torch
import torchvision
from torchvision.models.detection import (
    fasterrcnn_resnet50_fpn,
    fasterrcnn_resnet50_fpn_v2,
)
from torchvision.models.detection.faster_rcnn import FastRCNNPredictor


def create_model(config: dict) -> torch.nn.Module:
    """Create detection model."""
    num_classes = config.get("model", {{}}).get("num_classes", {num_classes})

    # Load pretrained Faster R-CNN
    model = fasterrcnn_resnet50_fpn(pretrained=True)

    # Replace head for custom classes
    in_features = model.roi_heads.box_predictor.cls_score.in_features
    model.roi_heads.box_predictor = FastRCNNPredictor(in_features, num_classes + 1)

    return model
'''

    def _generate_data_script(self, params: dict[str, Any]) -> str:
        """Generate data loading script."""
        return '''"""Data loading for Object Detection."""

import torch
from torch.utils.data import DataLoader, Dataset
from torchvision import transforms
from pathlib import Path
import json


class COCODataset(Dataset):
    """COCO format dataset."""

    def __init__(self, root: str, ann_file: str, transforms=None):
        self.root = Path(root)
        self.transforms = transforms

        with open(ann_file) as f:
            self.coco = json.load(f)

        self.images = self.coco["images"]
        self.annotations = self._load_annotations()

    def _load_annotations(self):
        """Load annotations indexed by image_id."""
        anns = {}
        for ann in self.coco["annotations"]:
            img_id = ann["image_id"]
            if img_id not in anns:
                anns[img_id] = []
            anns[img_id].append(ann)
        return anns

    def __len__(self):
        return len(self.images)

    def __getitem__(self, idx):
        img_info = self.images[idx]
        img_id = img_info["id"]

        # Load image
        from PIL import Image
        img_path = self.root / img_info["file_name"]
        img = Image.open(img_path).convert("RGB")

        # Load annotations
        anns = self.annotations.get(img_id, [])

        boxes = []
        labels = []
        for ann in anns:
            x, y, w, h = ann["bbox"]
            boxes.append([x, y, x + w, y + h])
            labels.append(ann["category_id"])

        target = {
            "boxes": torch.tensor(boxes, dtype=torch.float32),
            "labels": torch.tensor(labels, dtype=torch.int64),
        }

        if self.transforms:
            img = self.transforms(img)

        return img, target


def collate_fn(batch):
    """Custom collate function for detection."""
    return tuple(zip(*batch))


def create_dataloaders(config: dict) -> tuple[DataLoader, DataLoader]:
    """Create train and validation dataloaders."""
    data_path = Path(config.get("data", {}).get("path", "."))
    batch_size = config.get("training", {}).get("batch_size", 16)

    transform = transforms.Compose([
        transforms.ToTensor(),
    ])

    train_dataset = COCODataset(
        root=data_path / "train",
        ann_file=data_path / "annotations" / "train.json",
        transforms=transform,
    )

    val_dataset = COCODataset(
        root=data_path / "val",
        ann_file=data_path / "annotations" / "val.json",
        transforms=transform,
    )

    train_loader = DataLoader(
        train_dataset,
        batch_size=batch_size,
        shuffle=True,
        num_workers=4,
        collate_fn=collate_fn,
    )

    val_loader = DataLoader(
        val_dataset,
        batch_size=batch_size,
        shuffle=False,
        num_workers=4,
        collate_fn=collate_fn,
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
        arch = params.get("architecture", "yolov8n")

        return f'''# Object Detection with {arch}

This project was generated using the C4 Object Detection template.

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
