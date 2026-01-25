"""Domain detection based on project structure analysis."""

import logging
import re
from itertools import islice
from pathlib import Path
from typing import Optional

from c4.discovery.models import Domain, DomainDetectionResult, DomainSignal

logger = logging.getLogger(__name__)


class DomainDetector:
    """Detects project domain by analyzing file structure and configurations."""

    # Detection rules: (file_pattern, content_pattern, domain, confidence, reason)
    DETECTION_RULES: list[tuple[str, Optional[str], Domain, float, str]] = [
        # PiQ ML Experiment System (higher priority than generic ML/DL)
        ("pyproject.toml", r"piq", Domain.ML_DL, 0.95, "PiQ experiment system detected"),
        (".c4/config.yaml", r"piq", Domain.ML_DL, 0.95, "PiQ C4 config detected"),
        ("**/piqr.py", None, Domain.ML_DL, 0.9, "PiQ piqr module detected"),
        # Web Frontend
        ("package.json", r'"react"', Domain.WEB_FRONTEND, 0.9, "React detected"),
        ("package.json", r'"vue"', Domain.WEB_FRONTEND, 0.9, "Vue detected"),
        ("package.json", r'"angular"', Domain.WEB_FRONTEND, 0.9, "Angular detected"),
        ("package.json", r'"svelte"', Domain.WEB_FRONTEND, 0.9, "Svelte detected"),
        ("package.json", r'"next"', Domain.WEB_FRONTEND, 0.85, "Next.js detected"),
        ("index.html", None, Domain.WEB_FRONTEND, 0.5, "index.html found"),
        # Web Backend
        ("package.json", r'"express"', Domain.WEB_BACKEND, 0.9, "Express detected"),
        ("package.json", r'"fastify"', Domain.WEB_BACKEND, 0.9, "Fastify detected"),
        ("package.json", r'"nestjs"', Domain.WEB_BACKEND, 0.9, "NestJS detected"),
        ("package.json", r'"koa"', Domain.WEB_BACKEND, 0.85, "Koa detected"),
        ("pyproject.toml", r"fastapi", Domain.WEB_BACKEND, 0.9, "FastAPI detected"),
        ("pyproject.toml", r"flask", Domain.WEB_BACKEND, 0.9, "Flask detected"),
        ("pyproject.toml", r"django", Domain.WEB_BACKEND, 0.9, "Django detected"),
        ("requirements.txt", r"fastapi", Domain.WEB_BACKEND, 0.85, "FastAPI detected"),
        ("requirements.txt", r"flask", Domain.WEB_BACKEND, 0.85, "Flask detected"),
        ("requirements.txt", r"django", Domain.WEB_BACKEND, 0.85, "Django detected"),
        # ML/DL
        ("pyproject.toml", r"torch", Domain.ML_DL, 0.9, "PyTorch detected"),
        ("pyproject.toml", r"tensorflow", Domain.ML_DL, 0.9, "TensorFlow detected"),
        ("pyproject.toml", r"jax", Domain.ML_DL, 0.9, "JAX detected"),
        ("pyproject.toml", r"scikit-learn", Domain.ML_DL, 0.8, "scikit-learn detected"),
        ("pyproject.toml", r"transformers", Domain.ML_DL, 0.85, "HuggingFace detected"),
        ("requirements.txt", r"torch", Domain.ML_DL, 0.85, "PyTorch detected"),
        ("requirements.txt", r"tensorflow", Domain.ML_DL, 0.85, "TensorFlow detected"),
        ("requirements.txt", r"xgboost", Domain.ML_DL, 0.8, "XGBoost detected"),
        # Mobile App
        ("pubspec.yaml", None, Domain.MOBILE_APP, 0.95, "Flutter project detected"),
        ("package.json", r'"react-native"', Domain.MOBILE_APP, 0.95, "React Native detected"),
        ("android/build.gradle", None, Domain.MOBILE_APP, 0.8, "Android project detected"),
        # Infra
        ("*.tf", None, Domain.INFRA, 0.95, "Terraform files detected"),
        ("docker-compose.yml", None, Domain.INFRA, 0.7, "Docker Compose detected"),
        ("docker-compose.yaml", None, Domain.INFRA, 0.7, "Docker Compose detected"),
        ("Dockerfile", None, Domain.INFRA, 0.5, "Dockerfile detected"),
        (".github/workflows/*.yml", None, Domain.INFRA, 0.6, "GitHub Actions detected"),
        ("Pulumi.yaml", None, Domain.INFRA, 0.95, "Pulumi detected"),
        # Library
        ("setup.py", None, Domain.LIBRARY, 0.7, "Python package detected"),
        (
            "pyproject.toml",
            r"\[build-system\]",
            Domain.LIBRARY,
            0.75,
            "Python package with build system",
        ),
        ("package.json", r'"main":', Domain.LIBRARY, 0.6, "npm package detected"),
        ("package.json", r'"exports":', Domain.LIBRARY, 0.7, "npm package with exports"),
        # Firmware
        ("platformio.ini", None, Domain.FIRMWARE, 0.95, "PlatformIO detected"),
        ("Makefile", r"arm-none-eabi", Domain.FIRMWARE, 0.9, "ARM toolchain detected"),
        ("CMakeLists.txt", r"stm32", Domain.FIRMWARE, 0.9, "STM32 project detected"),
    ]

    # Files that indicate jupyter notebooks (ML/DL signal)
    NOTEBOOK_PATTERN = "*.ipynb"

    # Framework detection for domain-specific tooling
    # Returns framework name if detected (e.g., "piq", "mlflow", "wandb")
    FRAMEWORK_RULES: list[tuple[str, Optional[str], str, float, str]] = [
        # PiQ ML Experiment System
        ("pyproject.toml", r"piq", "piq", 0.95, "PiQ dependency detected"),
        (".c4/config.yaml", r"piq", "piq", 0.95, "PiQ C4 config detected"),
        ("**/piqr.py", None, "piq", 0.9, "PiQ piqr module detected"),
        ("packages/piq/**", None, "piq", 0.95, "PiQ monorepo structure"),
        # MLflow
        ("mlruns/**", None, "mlflow", 0.9, "MLflow runs directory"),
        ("mlflow.db", None, "mlflow", 0.9, "MLflow database"),
        # Weights & Biases
        ("wandb/**", None, "wandb", 0.85, "W&B directory"),
        # Hydra
        ("config/**/*.yaml", r"defaults:", "hydra", 0.8, "Hydra config"),
        ("outputs/**/.hydra", None, "hydra", 0.9, "Hydra outputs"),
    ]

    def __init__(self, project_root: Path):
        self.project_root = project_root

    def detect(self) -> DomainDetectionResult:
        """Analyze project structure and detect domains."""
        signals: list[DomainSignal] = []

        # Check if project is empty
        if self._is_empty_project():
            return DomainDetectionResult(
                primary_domain=Domain.UNKNOWN,
                confidence=0.0,
                signals=[],
                detected_domains=[],
                is_empty_project=True,
            )

        # Apply detection rules
        for file_pattern, content_pattern, domain, confidence, reason in self.DETECTION_RULES:
            matched_files = self._check_rule(file_pattern, content_pattern)
            if matched_files:
                signals.append(
                    DomainSignal(
                        domain=domain,
                        confidence=confidence,
                        reason=reason,
                        files_matched=matched_files,
                    )
                )

        # Check for jupyter notebooks (use islice to avoid collecting all files)
        notebook_iter = self.project_root.glob(f"**/{self.NOTEBOOK_PATTERN}")
        notebooks = list(islice(notebook_iter, 5))  # Only collect up to 5
        if notebooks:
            signals.append(
                DomainSignal(
                    domain=Domain.ML_DL,
                    confidence=0.7,
                    reason="Jupyter notebooks found",
                    files_matched=[str(nb.relative_to(self.project_root)) for nb in notebooks],
                )
            )

        # Aggregate signals by domain
        domain_scores: dict[Domain, float] = {}
        for signal in signals:
            current = domain_scores.get(signal.domain, 0.0)
            # Use max confidence (not sum) to avoid over-counting
            domain_scores[signal.domain] = max(current, signal.confidence)

        if not domain_scores:
            return DomainDetectionResult(
                primary_domain=Domain.UNKNOWN,
                confidence=0.0,
                signals=signals,
                detected_domains=[],
                is_empty_project=False,
            )

        # Sort by confidence
        sorted_domains = sorted(domain_scores.items(), key=lambda x: x[1], reverse=True)
        primary_domain, primary_confidence = sorted_domains[0]

        # Collect all domains with reasonable confidence (>0.5)
        detected_domains = [d for d, c in sorted_domains if c >= 0.5]

        # Check for fullstack
        if Domain.WEB_FRONTEND in detected_domains and Domain.WEB_BACKEND in detected_domains:
            # If both frontend and backend are detected, consider fullstack
            frontend_score = domain_scores.get(Domain.WEB_FRONTEND, 0)
            backend_score = domain_scores.get(Domain.WEB_BACKEND, 0)
            if frontend_score >= 0.7 and backend_score >= 0.7:
                primary_domain = Domain.FULLSTACK
                primary_confidence = min(
                    domain_scores[Domain.WEB_FRONTEND], domain_scores[Domain.WEB_BACKEND]
                )

        # Detect frameworks for domain-specific tooling
        detected_frameworks = self.detect_frameworks()

        return DomainDetectionResult(
            primary_domain=primary_domain,
            confidence=primary_confidence,
            signals=signals,
            detected_domains=detected_domains,
            is_empty_project=False,
            detected_frameworks=detected_frameworks,
        )

    def detect_frameworks(self) -> list[str]:
        """Detect ML/DL frameworks and tools used in the project.

        Returns list of framework names like ["piq", "hydra", "mlflow"].
        """
        frameworks: dict[str, float] = {}

        for file_pattern, content_pattern, framework, confidence, _reason in self.FRAMEWORK_RULES:
            matched = self._check_rule(file_pattern, content_pattern)
            if matched:
                current = frameworks.get(framework, 0.0)
                frameworks[framework] = max(current, confidence)

        # Return frameworks with confidence >= 0.7
        return [f for f, c in sorted(frameworks.items(), key=lambda x: -x[1]) if c >= 0.7]

    def _is_empty_project(self) -> bool:
        """Check if project directory is essentially empty."""
        # Allow common config files
        ignore_patterns = {
            ".git",
            ".gitignore",
            ".c4",
            ".mcp.json",
            ".claude",
            "README.md",
            "LICENSE",
        }

        for item in self.project_root.iterdir():
            if item.name not in ignore_patterns and not item.name.startswith("."):
                if item.is_file():
                    return False
                if item.is_dir() and any(item.iterdir()):
                    return False
        return True

    def _check_rule(self, file_pattern: str, content_pattern: Optional[str]) -> list[str]:
        """Check if a rule matches any files in the project."""
        matched_files: list[str] = []

        # Handle glob patterns
        if "*" in file_pattern:
            files = list(self.project_root.glob(file_pattern))
            files.extend(self.project_root.glob(f"**/{file_pattern}"))
        else:
            file_path = self.project_root / file_pattern
            files = [file_path] if file_path.exists() else []

        for file_path in files:
            if not file_path.exists() or not file_path.is_file():
                continue

            if content_pattern is None:
                # Just file existence check
                matched_files.append(str(file_path.relative_to(self.project_root)))
            else:
                # Check file content
                try:
                    content = file_path.read_text(encoding="utf-8", errors="ignore")
                    if re.search(content_pattern, content, re.IGNORECASE):
                        matched_files.append(str(file_path.relative_to(self.project_root)))
                except Exception as e:
                    logger.debug(f"Failed to read {file_path}: {e}")

        return matched_files

    def infer_domain_from_description(self, description: str) -> Domain:
        """Infer domain from user's project description."""
        desc_lower = description.lower()

        # Keyword patterns for each domain
        patterns: dict[Domain, list[str]] = {
            Domain.WEB_FRONTEND: [
                "website",
                "web app",
                "웹앱",
                "웹사이트",
                "react",
                "vue",
                "frontend",
                "프론트엔드",
                "ui",
                "사용자 인터페이스",
                "landing page",
                "랜딩",
                "dashboard",
                "대시보드",
                "spa",
                "웹 페이지",
            ],
            Domain.WEB_BACKEND: [
                "api",
                "server",
                "서버",
                "backend",
                "백엔드",
                "rest",
                "graphql",
                "microservice",
                "마이크로서비스",
                "endpoint",
                "database",
            ],
            Domain.ML_DL: [
                "model",
                "모델",
                "machine learning",
                "머신러닝",
                "deep learning",
                "딥러닝",
                "neural",
                "뉴럴",
                "training",
                "학습",
                "prediction",
                "예측",
                "classification",
                "분류",
                "regression",
                "회귀",
                "nlp",
                "cv",
                "computer vision",
                "컴퓨터 비전",
                "transformer",
                "gpt",
                "bert",
                "embedding",
                "임베딩",
                "inference",
                "추론",
            ],
            Domain.MOBILE_APP: [
                "app",
                "앱",
                "mobile",
                "모바일",
                "ios",
                "android",
                "안드로이드",
                "flutter",
                "react native",
                "swift",
                "kotlin",
                "휴대폰",
            ],
            Domain.INFRA: [
                "infrastructure",
                "인프라",
                "deploy",
                "배포",
                "kubernetes",
                "k8s",
                "docker",
                "terraform",
                "aws",
                "gcp",
                "azure",
                "cloud",
                "클라우드",
                "ci/cd",
                "pipeline",
                "파이프라인",
                "devops",
            ],
            Domain.LIBRARY: [
                "library",
                "라이브러리",
                "package",
                "패키지",
                "sdk",
                "module",
                "모듈",
                "utility",
                "유틸리티",
                "framework",
                "프레임워크",
            ],
            Domain.FIRMWARE: [
                "firmware",
                "펌웨어",
                "embedded",
                "임베디드",
                "microcontroller",
                "마이크로컨트롤러",
                "arduino",
                "stm32",
                "esp32",
                "iot",
            ],
        }

        scores: dict[Domain, int] = {}
        for domain, keywords in patterns.items():
            score = sum(1 for kw in keywords if kw in desc_lower)
            if score > 0:
                scores[domain] = score

        if not scores:
            return Domain.UNKNOWN

        return max(scores, key=lambda d: scores[d])
