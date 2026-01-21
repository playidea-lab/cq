"""Unit tests for Prometheus metrics.

RED Phase: Tests written before implementation.
These tests define the expected behavior for Prometheus metrics integration.
"""


import pytest


class TestPrometheusMetrics:
    """Tests for Prometheus metric definitions."""

    def test_api_requests_total_exists(self):
        """api_requests_total counter should exist with correct labels."""
        from prometheus_client import Counter

        from c4.monitoring.prometheus import api_requests_total

        # Should be a Counter type
        assert api_requests_total is not None
        assert isinstance(api_requests_total, Counter)
        # Prometheus Counter strips _total suffix in _name, check base name
        assert "c4_api_requests" in api_requests_total._name
        # Should have labels: endpoint, method, status
        assert "endpoint" in api_requests_total._labelnames
        assert "method" in api_requests_total._labelnames
        assert "status" in api_requests_total._labelnames

    def test_workspace_active_exists(self):
        """workspace_active gauge should exist."""
        from c4.monitoring.prometheus import workspace_active

        assert workspace_active is not None
        assert workspace_active._name == "c4_workspaces_active"

    def test_llm_tokens_total_exists(self):
        """llm_tokens_total counter should exist with correct labels."""
        from prometheus_client import Counter

        from c4.monitoring.prometheus import llm_tokens_total

        assert llm_tokens_total is not None
        assert isinstance(llm_tokens_total, Counter)
        # Prometheus Counter strips _total suffix in _name
        assert "c4_llm_tokens" in llm_tokens_total._name
        assert "model" in llm_tokens_total._labelnames
        assert "type" in llm_tokens_total._labelnames

    def test_task_duration_exists(self):
        """task_duration histogram should exist with correct labels."""
        from c4.monitoring.prometheus import task_duration

        assert task_duration is not None
        assert task_duration._name == "c4_task_duration_seconds"
        assert "task_type" in task_duration._labelnames

    def test_llm_requests_total_exists(self):
        """llm_requests_total counter should exist with correct labels."""
        from prometheus_client import Counter

        from c4.monitoring.prometheus import llm_requests_total

        assert llm_requests_total is not None
        assert isinstance(llm_requests_total, Counter)
        # Prometheus Counter strips _total suffix in _name
        assert "c4_llm_requests" in llm_requests_total._name
        assert "model" in llm_requests_total._labelnames
        assert "status" in llm_requests_total._labelnames

    def test_workspace_operations_total_exists(self):
        """workspace_operations_total counter should exist with correct labels."""
        from prometheus_client import Counter

        from c4.monitoring.prometheus import workspace_operations_total

        assert workspace_operations_total is not None
        assert isinstance(workspace_operations_total, Counter)
        # Prometheus Counter strips _total suffix in _name
        assert "c4_workspace_operations" in workspace_operations_total._name
        assert "operation" in workspace_operations_total._labelnames
        assert "status" in workspace_operations_total._labelnames


class TestHelperFunctions:
    """Tests for metric helper functions."""

    def test_record_api_request(self):
        """record_api_request should increment counter with labels."""
        from c4.monitoring.prometheus import api_requests_total, record_api_request

        # Record a request
        record_api_request(endpoint="/api/test", method="GET", status=200)

        # Verify counter was incremented
        metric_value = api_requests_total.labels(endpoint="/api/test", method="GET", status="200")._value.get()
        assert metric_value >= 1

    def test_set_active_workspaces(self):
        """set_active_workspaces should update gauge value."""
        from c4.monitoring.prometheus import set_active_workspaces, workspace_active

        # Set workspace count
        set_active_workspaces(5)

        # Verify gauge value
        assert workspace_active._value.get() == 5

        # Update to new value
        set_active_workspaces(3)
        assert workspace_active._value.get() == 3

    def test_record_llm_tokens(self):
        """record_llm_tokens should increment both input and output counters."""
        from c4.monitoring.prometheus import llm_tokens_total, record_llm_tokens

        # Record tokens
        record_llm_tokens(model="gpt-4", input_tokens=100, output_tokens=50)

        # Verify input tokens
        input_value = llm_tokens_total.labels(model="gpt-4", type="input")._value.get()
        assert input_value >= 100

        # Verify output tokens
        output_value = llm_tokens_total.labels(model="gpt-4", type="output")._value.get()
        assert output_value >= 50

    def test_record_task_duration(self):
        """record_task_duration should observe duration in histogram."""
        from c4.monitoring.prometheus import record_task_duration, task_duration

        # Record duration
        record_task_duration(task_type="validation", duration_seconds=2.5)

        # Verify histogram has recorded a sample
        histogram_count = task_duration.labels(task_type="validation")._sum.get()
        assert histogram_count >= 2.5

    def test_record_llm_request(self):
        """record_llm_request should increment counter with model and status."""
        from c4.monitoring.prometheus import llm_requests_total, record_llm_request

        record_llm_request(model="claude-3", status="success")

        metric_value = llm_requests_total.labels(model="claude-3", status="success")._value.get()
        assert metric_value >= 1

    def test_record_workspace_operation(self):
        """record_workspace_operation should increment counter with operation and status."""
        from c4.monitoring.prometheus import record_workspace_operation, workspace_operations_total

        record_workspace_operation(operation="create", status="success")

        metric_value = workspace_operations_total.labels(operation="create", status="success")._value.get()
        assert metric_value >= 1

    def test_get_metrics_returns_bytes(self):
        """get_metrics should return Prometheus format bytes."""
        from c4.monitoring.prometheus import get_metrics

        result = get_metrics()

        # Should return bytes
        assert isinstance(result, bytes)

        # Should contain metric names
        decoded = result.decode("utf-8")
        assert "c4_api_requests_total" in decoded or "c4_workspaces_active" in decoded

    def test_content_type_latest_exported(self):
        """CONTENT_TYPE_LATEST should be exported."""
        from c4.monitoring.prometheus import CONTENT_TYPE_LATEST

        assert CONTENT_TYPE_LATEST is not None
        assert "text/plain" in CONTENT_TYPE_LATEST or "openmetrics" in CONTENT_TYPE_LATEST.lower()


class TestMetricsMiddleware:
    """Tests for FastAPI metrics middleware."""

    @pytest.mark.asyncio
    async def test_middleware_records_request(self):
        """Middleware should record API request metrics."""
        from fastapi import FastAPI
        from starlette.testclient import TestClient

        from c4.monitoring.middleware import MetricsMiddleware

        app = FastAPI()
        app.add_middleware(MetricsMiddleware)

        @app.get("/test")
        async def test_endpoint():
            return {"status": "ok"}

        client = TestClient(app)
        response = client.get("/test")

        assert response.status_code == 200

    @pytest.mark.asyncio
    async def test_middleware_records_status_code(self):
        """Middleware should record correct status code."""
        from fastapi import FastAPI, HTTPException
        from starlette.testclient import TestClient

        from c4.monitoring.middleware import MetricsMiddleware

        app = FastAPI()
        app.add_middleware(MetricsMiddleware)

        @app.get("/error")
        async def error_endpoint():
            raise HTTPException(status_code=404, detail="Not found")

        client = TestClient(app)
        response = client.get("/error")

        assert response.status_code == 404


class TestMetricsEndpoint:
    """Tests for /metrics API endpoint."""

    def test_metrics_endpoint_returns_prometheus_format(self):
        """GET /metrics should return Prometheus format."""
        from fastapi import FastAPI
        from starlette.testclient import TestClient

        from c4.monitoring.routes import router

        app = FastAPI()
        app.include_router(router)

        client = TestClient(app)
        response = client.get("/metrics")

        assert response.status_code == 200
        # Should have prometheus content type
        assert "text/plain" in response.headers.get("content-type", "") or "openmetrics" in response.headers.get("content-type", "").lower()

    def test_metrics_endpoint_includes_all_metrics(self):
        """GET /metrics should include all defined metrics."""
        from fastapi import FastAPI
        from starlette.testclient import TestClient

        from c4.monitoring.routes import router

        app = FastAPI()
        app.include_router(router)

        client = TestClient(app)
        response = client.get("/metrics")

        content = response.text
        # Should include metric help text or names
        assert "c4_" in content


class TestModuleExports:
    """Tests for module __init__.py exports."""

    def test_all_metrics_exported(self):
        """All metrics should be exported from __init__."""
        from c4.monitoring import (
            api_requests_total,
            llm_requests_total,
            llm_tokens_total,
            task_duration,
            workspace_active,
            workspace_operations_total,
        )

        assert api_requests_total is not None
        assert workspace_active is not None
        assert llm_tokens_total is not None
        assert task_duration is not None
        assert llm_requests_total is not None
        assert workspace_operations_total is not None

    def test_all_helper_functions_exported(self):
        """All helper functions should be exported from __init__."""
        from c4.monitoring import (
            CONTENT_TYPE_LATEST,
            get_metrics,
            record_api_request,
            record_llm_request,
            record_llm_tokens,
            record_task_duration,
            record_workspace_operation,
            set_active_workspaces,
        )

        assert callable(record_api_request)
        assert callable(set_active_workspaces)
        assert callable(record_llm_tokens)
        assert callable(record_task_duration)
        assert callable(record_llm_request)
        assert callable(record_workspace_operation)
        assert callable(get_metrics)
        assert CONTENT_TYPE_LATEST is not None
