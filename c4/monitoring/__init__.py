"""C4 Monitoring package.

Provides Prometheus metrics integration for observability.

Metrics:
- api_requests_total: Total API requests (counter)
- workspace_active: Active workspaces (gauge)
- llm_tokens_total: LLM token usage (counter)
- task_duration: Task execution time (histogram)
- llm_requests_total: LLM API requests (counter)
- workspace_operations_total: Workspace operations (counter)
- request_duration: HTTP request duration (histogram)

Usage:
    from c4.monitoring import record_api_request, get_metrics

    # Record a request
    record_api_request(endpoint="/api/task", method="POST", status=200)

    # Get Prometheus-formatted metrics
    metrics_bytes = get_metrics()
"""

from .prometheus import (
    CONTENT_TYPE_LATEST,
    api_requests_total,
    get_metrics,
    llm_requests_total,
    llm_tokens_total,
    record_api_request,
    record_llm_request,
    record_llm_tokens,
    record_request_duration,
    record_task_duration,
    record_workspace_operation,
    request_duration,
    set_active_workspaces,
    task_duration,
    workspace_active,
    workspace_operations_total,
)

__all__ = [
    # Metrics
    "api_requests_total",
    "workspace_active",
    "llm_tokens_total",
    "task_duration",
    "llm_requests_total",
    "workspace_operations_total",
    "request_duration",
    # Helper functions
    "record_api_request",
    "set_active_workspaces",
    "record_llm_tokens",
    "record_task_duration",
    "record_llm_request",
    "record_workspace_operation",
    "record_request_duration",
    "get_metrics",
    # Constants
    "CONTENT_TYPE_LATEST",
]
