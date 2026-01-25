"""LLM Fine-tuning Template.

Provides a complete template for fine-tuning Large Language Models
with LoRA, QLoRA, and full fine-tuning support.
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
class LLMFinetuningTemplate(Template):
    """LLM Fine-tuning experiment template.

    Supports:
    - LoRA and QLoRA fine-tuning
    - Full fine-tuning (for smaller models)
    - Instruction tuning datasets
    - PEFT (Parameter-Efficient Fine-Tuning)
    - DeepSpeed and FSDP for distributed training
    - Quantization (4-bit, 8-bit)
    """

    @property
    def config(self) -> TemplateConfig:
        """Get template configuration."""
        return TemplateConfig(
            id="llm-finetuning",
            name="LLM Fine-tuning",
            version="1.0.0",
            category=TemplateCategory.LLM,
            description="Fine-tune Large Language Models with LoRA, QLoRA, or full fine-tuning",
            author="C4 Team",
            tags=["llm", "finetuning", "lora", "qlora", "instruction-tuning", "peft"],
            parameters=[
                # Model parameters
                TemplateParameter(
                    name="base_model",
                    param_type=ParameterType.MODEL,
                    description="Base model to fine-tune",
                    default="meta-llama/Llama-2-7b-hf",
                    choices=[
                        "meta-llama/Llama-2-7b-hf",
                        "meta-llama/Llama-2-13b-hf",
                        "meta-llama/Llama-3-8B",
                        "mistralai/Mistral-7B-v0.1",
                        "mistralai/Mixtral-8x7B-v0.1",
                        "google/gemma-7b",
                        "google/gemma-2b",
                        "Qwen/Qwen2-7B",
                        "microsoft/phi-2",
                    ],
                    piq_knowledge_ref="backbone",
                ),
                TemplateParameter(
                    name="finetuning_method",
                    param_type=ParameterType.CHOICE,
                    description="Fine-tuning method",
                    default="lora",
                    choices=["full", "lora", "qlora"],
                    piq_knowledge_ref="technique",
                ),
                # Data parameters
                TemplateParameter(
                    name="dataset_path",
                    param_type=ParameterType.PATH,
                    description="Path to dataset (JSONL format)",
                    required=True,
                ),
                TemplateParameter(
                    name="dataset_format",
                    param_type=ParameterType.CHOICE,
                    description="Dataset format",
                    default="alpaca",
                    choices=["alpaca", "sharegpt", "completion", "custom"],
                ),
                TemplateParameter(
                    name="max_seq_length",
                    param_type=ParameterType.INTEGER,
                    description="Maximum sequence length",
                    default=2048,
                    min_value=256,
                    max_value=32768,
                ),
                # LoRA parameters
                TemplateParameter(
                    name="lora_r",
                    param_type=ParameterType.INTEGER,
                    description="LoRA rank",
                    default=16,
                    min_value=4,
                    max_value=256,
                    piq_knowledge_ref="hyperparameter",
                ),
                TemplateParameter(
                    name="lora_alpha",
                    param_type=ParameterType.INTEGER,
                    description="LoRA alpha (scaling factor)",
                    default=32,
                    min_value=8,
                    max_value=512,
                ),
                TemplateParameter(
                    name="lora_dropout",
                    param_type=ParameterType.FLOAT,
                    description="LoRA dropout",
                    default=0.05,
                    min_value=0.0,
                    max_value=0.5,
                ),
                TemplateParameter(
                    name="target_modules",
                    param_type=ParameterType.STRING,
                    description="Target modules for LoRA (comma-separated)",
                    default="q_proj,k_proj,v_proj,o_proj",
                ),
                # Training parameters
                TemplateParameter(
                    name="batch_size",
                    param_type=ParameterType.INTEGER,
                    description="Per-device batch size",
                    default=4,
                    min_value=1,
                    max_value=64,
                    piq_knowledge_ref="batch_size",
                ),
                TemplateParameter(
                    name="gradient_accumulation",
                    param_type=ParameterType.INTEGER,
                    description="Gradient accumulation steps",
                    default=4,
                    min_value=1,
                    max_value=64,
                ),
                TemplateParameter(
                    name="epochs",
                    param_type=ParameterType.INTEGER,
                    description="Number of training epochs",
                    default=3,
                    min_value=1,
                    max_value=100,
                ),
                TemplateParameter(
                    name="learning_rate",
                    param_type=ParameterType.FLOAT,
                    description="Learning rate",
                    default=2e-4,
                    min_value=1e-7,
                    max_value=1e-2,
                    piq_knowledge_ref="learning_rate",
                ),
                TemplateParameter(
                    name="optimizer",
                    param_type=ParameterType.CHOICE,
                    description="Optimizer",
                    default="adamw_8bit",
                    choices=["adamw", "adamw_8bit", "sgd", "adafactor"],
                    piq_knowledge_ref="optimizer",
                ),
                TemplateParameter(
                    name="scheduler",
                    param_type=ParameterType.CHOICE,
                    description="Learning rate scheduler",
                    default="cosine",
                    choices=["constant", "linear", "cosine", "polynomial"],
                    piq_knowledge_ref="scheduler",
                ),
                TemplateParameter(
                    name="warmup_ratio",
                    param_type=ParameterType.FLOAT,
                    description="Warmup ratio",
                    default=0.03,
                    min_value=0.0,
                    max_value=0.5,
                ),
                # Quantization
                TemplateParameter(
                    name="load_in_4bit",
                    param_type=ParameterType.BOOLEAN,
                    description="Load model in 4-bit (QLoRA)",
                    default=True,
                ),
                TemplateParameter(
                    name="bnb_4bit_quant_type",
                    param_type=ParameterType.CHOICE,
                    description="4-bit quantization type",
                    default="nf4",
                    choices=["nf4", "fp4"],
                ),
                TemplateParameter(
                    name="use_flash_attention",
                    param_type=ParameterType.BOOLEAN,
                    description="Use Flash Attention 2",
                    default=True,
                ),
                # Output
                TemplateParameter(
                    name="output_dir",
                    param_type=ParameterType.PATH,
                    description="Output directory for checkpoints",
                    default="./output",
                ),
                TemplateParameter(
                    name="merge_and_save",
                    param_type=ParameterType.BOOLEAN,
                    description="Merge LoRA weights and save full model",
                    default=True,
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
                "llm",
                "lora",
                "qlora",
                "instruction_tuning",
                "peft",
            ],
            checkpoints=[
                {
                    "id": "CP-DATA",
                    "name": "Data Pipeline Ready",
                    "description": "Dataset loaded and tokenized",
                },
                {
                    "id": "CP-MODEL",
                    "name": "Model Ready",
                    "description": "Base model loaded with LoRA adapters",
                },
                {
                    "id": "CP-TRAIN",
                    "name": "Training Complete",
                    "description": "Fine-tuning finished with evaluation",
                },
            ],
            dependencies=[
                "torch>=2.0.0",
                "transformers>=4.36.0",
                "peft>=0.7.0",
                "bitsandbytes>=0.41.0",
                "accelerate>=0.25.0",
                "trl>=0.7.0",
                "datasets>=2.14.0",
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
        files["inference.py"] = self._generate_inference_script(params)

        return files

    def generate_config(
        self,
        params: dict[str, Any],
    ) -> dict[str, Any]:
        """Generate experiment configuration."""
        base_model = params.get("base_model", "meta-llama/Llama-2-7b-hf")
        method = params.get("finetuning_method", "lora")

        return {
            "experiment": {
                "name": f"llm-finetune-{base_model.split('/')[-1]}-{method}",
                "template_id": self.id,
            },
            "model": {
                "base_model": base_model,
                "finetuning_method": method,
                "load_in_4bit": params.get("load_in_4bit", True),
                "bnb_4bit_quant_type": params.get("bnb_4bit_quant_type", "nf4"),
                "use_flash_attention": params.get("use_flash_attention", True),
            },
            "lora": {
                "r": params.get("lora_r", 16),
                "alpha": params.get("lora_alpha", 32),
                "dropout": params.get("lora_dropout", 0.05),
                "target_modules": params.get(
                    "target_modules", "q_proj,k_proj,v_proj,o_proj"
                ).split(","),
            },
            "data": {
                "dataset_path": params.get("dataset_path", ""),
                "dataset_format": params.get("dataset_format", "alpaca"),
                "max_seq_length": params.get("max_seq_length", 2048),
            },
            "training": {
                "batch_size": params.get("batch_size", 4),
                "gradient_accumulation": params.get("gradient_accumulation", 4),
                "epochs": params.get("epochs", 3),
                "learning_rate": params.get("learning_rate", 2e-4),
                "optimizer": params.get("optimizer", "adamw_8bit"),
                "scheduler": params.get("scheduler", "cosine"),
                "warmup_ratio": params.get("warmup_ratio", 0.03),
            },
            "output": {
                "output_dir": params.get("output_dir", "./output"),
                "merge_and_save": params.get("merge_and_save", True),
            },
        }

    def generate_tasks(
        self,
        params: dict[str, Any],
    ) -> list[dict[str, Any]]:
        """Generate C4 tasks from template."""
        model_name = params.get("base_model", "Llama-2-7b").split("/")[-1]
        method = params.get("finetuning_method", "lora")

        return [
            {
                "id": "T-LLM-001",
                "title": "Setup Project Structure",
                "scope": [".", "requirements.txt", "config.yaml"],
                "dod": "Project created with dependencies installed, GPU verified",
                "dependencies": [],
            },
            {
                "id": "T-LLM-002",
                "title": "Prepare Dataset",
                "scope": ["data.py", "tests/test_data.py"],
                "dod": (
                    "1) Dataset loaded from path, "
                    "2) Tokenization working, "
                    "3) Data collator configured"
                ),
                "dependencies": ["T-LLM-001"],
            },
            {
                "id": "T-LLM-003",
                "title": f"Load {model_name} with {method.upper()}",
                "scope": ["model.py", "tests/test_model.py"],
                "dod": (
                    f"1) {model_name} loaded in quantized mode, "
                    f"2) {method.upper()} adapters attached, "
                    "3) Forward pass successful"
                ),
                "dependencies": ["T-LLM-001"],
            },
            {
                "id": "T-LLM-004",
                "title": "Implement Training Pipeline",
                "scope": ["train.py"],
                "dod": (
                    "1) SFTTrainer configured, "
                    "2) Training starts without OOM, "
                    "3) Loss decreasing"
                ),
                "dependencies": ["T-LLM-002", "T-LLM-003"],
            },
            {
                "id": "T-LLM-005",
                "title": "Run Fine-tuning",
                "scope": ["experiments/", "output/"],
                "dod": (
                    f"1) Model fine-tuned for {params.get('epochs', 3)} epochs, "
                    "2) Checkpoints saved, "
                    "3) Training metrics logged"
                ),
                "dependencies": ["T-LLM-004"],
            },
            {
                "id": "T-LLM-006",
                "title": "Merge and Evaluate",
                "scope": ["inference.py", "output/merged"],
                "dod": (
                    "1) LoRA weights merged, "
                    "2) Inference test passed, "
                    "3) Quality samples generated"
                ),
                "dependencies": ["T-LLM-005"],
            },
        ]

    def generate_checkpoints(
        self,
        params: dict[str, Any],
    ) -> list[dict[str, Any]]:
        """Generate C4 checkpoints from template."""
        return [
            {
                "id": "CP-LLM-DATA",
                "name": "Data Pipeline Ready",
                "description": "Dataset loaded and tokenized",
                "required_tasks": ["T-LLM-001", "T-LLM-002"],
                "required_validations": ["lint", "unit"],
            },
            {
                "id": "CP-LLM-MODEL",
                "name": "Model Ready",
                "description": "Base model with LoRA adapters",
                "required_tasks": ["T-LLM-003"],
                "required_validations": ["lint", "unit"],
            },
            {
                "id": "CP-LLM-TRAIN",
                "name": "Fine-tuning Complete",
                "description": "Training finished and model merged",
                "required_tasks": ["T-LLM-004", "T-LLM-005", "T-LLM-006"],
                "required_validations": ["lint", "unit"],
            },
        ]

    # =========================================================================
    # Private Methods - File Generation
    # =========================================================================

    def _generate_train_script(self, params: dict[str, Any]) -> str:
        """Generate training script."""
        return f'''"""Training script for LLM Fine-tuning."""

import torch
from transformers import (
    AutoModelForCausalLM,
    AutoTokenizer,
    BitsAndBytesConfig,
    TrainingArguments,
)
from peft import LoraConfig, get_peft_model, prepare_model_for_kbit_training
from trl import SFTTrainer
from datasets import load_dataset

from data import load_and_prepare_dataset
from model import load_model_and_tokenizer


def train(config: dict) -> None:
    """Fine-tune the LLM."""
    # Load model and tokenizer
    model, tokenizer = load_model_and_tokenizer(config)

    # Prepare for k-bit training
    model = prepare_model_for_kbit_training(model)

    # Add LoRA adapters
    lora_config = LoraConfig(
        r={params.get("lora_r", 16)},
        lora_alpha={params.get("lora_alpha", 32)},
        lora_dropout={params.get("lora_dropout", 0.05)},
        target_modules={params.get("target_modules", "q_proj,k_proj,v_proj,o_proj").split(",")},
        bias="none",
        task_type="CAUSAL_LM",
    )
    model = get_peft_model(model, lora_config)

    # Load dataset
    dataset = load_and_prepare_dataset(config, tokenizer)

    # Training arguments
    training_args = TrainingArguments(
        output_dir=config.get("output", {{}}).get("output_dir", "./output"),
        num_train_epochs={params.get("epochs", 3)},
        per_device_train_batch_size={params.get("batch_size", 4)},
        gradient_accumulation_steps={params.get("gradient_accumulation", 4)},
        learning_rate={params.get("learning_rate", 2e-4)},
        lr_scheduler_type="{params.get("scheduler", "cosine")}",
        warmup_ratio={params.get("warmup_ratio", 0.03)},
        optim="{params.get("optimizer", "adamw_8bit")}",
        fp16=True,
        logging_steps=10,
        save_strategy="epoch",
        report_to="none",
    )

    # Initialize trainer
    trainer = SFTTrainer(
        model=model,
        args=training_args,
        train_dataset=dataset["train"],
        tokenizer=tokenizer,
        max_seq_length={params.get("max_seq_length", 2048)},
    )

    # Train
    trainer.train()

    # Save
    trainer.save_model()

    # Merge and save if configured
    if config.get("output", {{}}).get("merge_and_save", True):
        from peft import PeftModel

        merged_model = model.merge_and_unload()
        merged_model.save_pretrained(
            config["output"]["output_dir"] + "/merged"
        )
        tokenizer.save_pretrained(
            config["output"]["output_dir"] + "/merged"
        )
        print("Merged model saved!")


if __name__ == "__main__":
    import yaml
    with open("config.yaml") as f:
        config = yaml.safe_load(f)
    train(config)
'''

    def _generate_model_script(self, params: dict[str, Any]) -> str:
        """Generate model script."""
        return f'''"""Model loading for LLM Fine-tuning."""

import torch
from transformers import (
    AutoModelForCausalLM,
    AutoTokenizer,
    BitsAndBytesConfig,
)


def load_model_and_tokenizer(config: dict):
    """Load base model and tokenizer."""
    model_name = config.get("model", {{}}).get(
        "base_model", "{params.get("base_model", "meta-llama/Llama-2-7b-hf")}"
    )

    # Quantization config
    bnb_config = None
    if config.get("model", {{}}).get("load_in_4bit", {params.get("load_in_4bit", True)}):
        bnb_config = BitsAndBytesConfig(
            load_in_4bit=True,
            bnb_4bit_quant_type=config.get("model", {{}}).get(
                "bnb_4bit_quant_type", "{params.get("bnb_4bit_quant_type", "nf4")}"
            ),
            bnb_4bit_compute_dtype=torch.bfloat16,
            bnb_4bit_use_double_quant=True,
        )

    # Load model
    model = AutoModelForCausalLM.from_pretrained(
        model_name,
        quantization_config=bnb_config,
        device_map="auto",
        trust_remote_code=True,
        attn_implementation="flash_attention_2" if config.get("model", {{}}).get(
            "use_flash_attention", {params.get("use_flash_attention", True)}
        ) else None,
    )

    # Load tokenizer
    tokenizer = AutoTokenizer.from_pretrained(
        model_name,
        trust_remote_code=True,
    )
    tokenizer.pad_token = tokenizer.eos_token
    tokenizer.padding_side = "right"

    return model, tokenizer
'''

    def _generate_data_script(self, params: dict[str, Any]) -> str:
        """Generate data loading script."""
        return f'''"""Data loading for LLM Fine-tuning."""

from datasets import load_dataset
from pathlib import Path


# Prompt templates
ALPACA_TEMPLATE = """Below is an instruction that describes a task. Write a response that appropriately completes the request.

### Instruction:
{{instruction}}

### Response:
{{response}}"""

SHAREGPT_TEMPLATE = """{{conversations}}"""


def format_alpaca(example):
    """Format Alpaca-style example."""
    instruction = example.get("instruction", "")
    input_text = example.get("input", "")
    output = example.get("output", "")

    if input_text:
        instruction = f"{{instruction}}\\n\\nInput: {{input_text}}"

    return {{
        "text": ALPACA_TEMPLATE.format(
            instruction=instruction,
            response=output,
        )
    }}


def format_sharegpt(example):
    """Format ShareGPT-style example."""
    conversations = example.get("conversations", [])
    text = ""
    for turn in conversations:
        role = turn.get("from", "human")
        content = turn.get("value", "")
        if role == "human":
            text += f"### Human: {{content}}\\n\\n"
        else:
            text += f"### Assistant: {{content}}\\n\\n"
    return {{"text": text.strip()}}


def load_and_prepare_dataset(config: dict, tokenizer):
    """Load and prepare dataset for training."""
    data_path = config.get("data", {{}}).get("dataset_path", "")
    data_format = config.get("data", {{}}).get("dataset_format", "alpaca")
    max_seq_length = config.get("data", {{}}).get("max_seq_length", {params.get("max_seq_length", 2048)})

    # Load dataset
    if Path(data_path).suffix == ".jsonl":
        dataset = load_dataset("json", data_files=data_path)
    else:
        dataset = load_dataset(data_path)

    # Format based on type
    if data_format == "alpaca":
        dataset = dataset.map(format_alpaca)
    elif data_format == "sharegpt":
        dataset = dataset.map(format_sharegpt)

    return dataset
'''

    def _generate_inference_script(self, params: dict[str, Any]) -> str:
        """Generate inference script."""
        return f'''"""Inference script for fine-tuned LLM."""

import torch
from transformers import AutoModelForCausalLM, AutoTokenizer, pipeline


def load_merged_model(model_path: str):
    """Load the merged model for inference."""
    tokenizer = AutoTokenizer.from_pretrained(model_path)
    model = AutoModelForCausalLM.from_pretrained(
        model_path,
        torch_dtype=torch.bfloat16,
        device_map="auto",
    )
    return model, tokenizer


def generate_response(model, tokenizer, prompt: str, max_new_tokens: int = 512):
    """Generate a response from the model."""
    inputs = tokenizer(prompt, return_tensors="pt").to(model.device)

    with torch.no_grad():
        outputs = model.generate(
            **inputs,
            max_new_tokens=max_new_tokens,
            do_sample=True,
            temperature=0.7,
            top_p=0.9,
            repetition_penalty=1.1,
        )

    response = tokenizer.decode(outputs[0], skip_special_tokens=True)
    return response[len(prompt):]


if __name__ == "__main__":
    import yaml
    with open("config.yaml") as f:
        config = yaml.safe_load(f)

    model_path = config["output"]["output_dir"] + "/merged"
    model, tokenizer = load_merged_model(model_path)

    # Test prompts
    test_prompts = [
        "What is machine learning?",
        "Write a short poem about coding.",
        "Explain quantum computing in simple terms.",
    ]

    for prompt in test_prompts:
        formatted_prompt = f"""Below is an instruction that describes a task. Write a response that appropriately completes the request.

### Instruction:
{{prompt}}

### Response:
"""
        response = generate_response(model, tokenizer, formatted_prompt)
        print(f"Prompt: {{prompt}}")
        print(f"Response: {{response}}")
        print("-" * 50)
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
        model = params.get("base_model", "Llama-2-7b").split("/")[-1]
        method = params.get("finetuning_method", "lora").upper()

        return f'''# {model} Fine-tuning with {method}

This project was generated using the C4 LLM Fine-tuning template.

## Requirements

- NVIDIA GPU with at least 16GB VRAM (for 7B models with QLoRA)
- CUDA 11.8+
- Python 3.10+

## Setup

```bash
pip install -r requirements.txt
```

## Training

```bash
python train.py
```

## Inference

After training, run inference with:

```bash
python inference.py
```

## Configuration

Edit `config.yaml` to modify training parameters.

## Generated by C4 Template Library
'''
