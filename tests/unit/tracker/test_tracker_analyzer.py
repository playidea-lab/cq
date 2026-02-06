"""Tests for AST code analyzer."""

from c4.tracker.analyzer import analyze_code


class TestAnalyzeCode:
    def test_extract_imports(self):
        source = "import numpy as np\nfrom sklearn.ensemble import RandomForestClassifier"
        result = analyze_code(source)
        assert "numpy" in result["imports"]
        assert "sklearn.ensemble.RandomForestClassifier" in result["imports"]

    def test_infer_algorithm_sklearn(self):
        source = "from sklearn.ensemble import RandomForestClassifier\nmodel = RandomForestClassifier()"
        result = analyze_code(source)
        assert result["algorithm"] == "RandomForest"

    def test_infer_algorithm_xgboost(self):
        source = "import xgboost as xgb\nmodel = xgb.XGBClassifier()"
        result = analyze_code(source)
        assert result["algorithm"] == "XGBoost"

    def test_infer_algorithm_pytorch(self):
        source = "import torch.nn as nn\nmodel = nn.Linear(10, 1)"
        result = analyze_code(source)
        assert result["algorithm"] == "PyTorch"

    def test_no_algorithm(self):
        source = "x = 1 + 2"
        result = analyze_code(source)
        assert result["algorithm"] is None

    def test_function_calls(self):
        source = "model.fit(X, y)\nprint(model.predict(X_test))"
        result = analyze_code(source)
        assert "fit" in result["function_calls"]
        assert "predict" in result["function_calls"]

    def test_syntax_error(self):
        result = analyze_code("def broken(")
        assert "error" in result
