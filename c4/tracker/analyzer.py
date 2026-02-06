"""AST-based code analysis - extract imports, algorithms, hyperparameters.

Absorbed from piq/piqr/analyzer.py.
"""

from __future__ import annotations

import ast
import logging
from pathlib import Path

logger = logging.getLogger(__name__)

# Known ML library → algorithm mapping
_ALGORITHM_MAP = {
    "sklearn.ensemble.RandomForestClassifier": "RandomForest",
    "sklearn.ensemble.GradientBoostingClassifier": "GradientBoosting",
    "sklearn.linear_model.LogisticRegression": "LogisticRegression",
    "sklearn.svm.SVC": "SVM",
    "xgboost": "XGBoost",
    "lightgbm": "LightGBM",
    "catboost": "CatBoost",
    "torch.nn": "PyTorch",
    "tensorflow": "TensorFlow",
    "transformers": "HuggingFace Transformers",
    "keras": "Keras",
}


def analyze_code(source: str) -> dict:
    """Analyze Python source code for ML-relevant features.

    Args:
        source: Python source code string

    Returns:
        Dict with: imports, algorithm, function_calls, model_params
    """
    try:
        tree = ast.parse(source)
    except SyntaxError:
        return {"imports": [], "algorithm": None, "error": "SyntaxError"}

    imports = _extract_imports(tree)
    algorithm = _infer_algorithm(imports)
    function_calls = _extract_calls(tree)

    return {
        "imports": imports,
        "algorithm": algorithm,
        "function_calls": function_calls[:20],  # Limit
    }


def analyze_file(path: str | Path) -> dict:
    """Analyze a Python file."""
    path = Path(path)
    if not path.exists() or not path.suffix == ".py":
        return {"error": f"Not a Python file: {path}"}

    try:
        source = path.read_text(encoding="utf-8")
        result = analyze_code(source)
        result["file"] = str(path)
        return result
    except Exception as e:
        return {"error": str(e)}


def _extract_imports(tree: ast.AST) -> list[str]:
    """Extract all import statements."""
    imports = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                imports.append(alias.name)
        elif isinstance(node, ast.ImportFrom):
            module = node.module or ""
            for alias in node.names:
                imports.append(f"{module}.{alias.name}" if module else alias.name)
    return sorted(set(imports))


def _infer_algorithm(imports: list[str]) -> str | None:
    """Infer ML algorithm from imports."""
    for imp in imports:
        for pattern, algo in _ALGORITHM_MAP.items():
            if pattern in imp:
                return algo
    return None


def _extract_calls(tree: ast.AST) -> list[str]:
    """Extract function/method call names."""
    calls = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Call):
            name = _get_call_name(node)
            if name:
                calls.append(name)
    return calls


def _get_call_name(node: ast.Call) -> str | None:
    """Get the name of a function call."""
    if isinstance(node.func, ast.Name):
        return node.func.id
    elif isinstance(node.func, ast.Attribute):
        return node.func.attr
    return None
