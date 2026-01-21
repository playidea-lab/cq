"""Prometheus metrics for C4 monitoring.

Provides Prometheus metrics for:
- API requests (counter)
- Active workspaces (gauge)
- LLM token usage (counter)
- Task execution duration (histogram)
- LLM API requests (counter)
- Workspace operations (counter)
"""

from prometheus_client import CONTENT_TYPE_LATEST, Counter, Gauge, Histogram, generate_latest

# Re-export CONTENT_TYPE_LATEST for use in routes
__all__ = [
    "api_requests_total",
    "workspace_active",
    "llm_tokens_total",
    "task_duration",
    "llm_requests_total",
    "workspace_operations_total",
    "request_duration",
    "record_api_request",
    "set_active_workspaces",
    "record_llm_tokens",
    "record_task_duration",
    "record_llm_request",
    "record_workspace_operation",
    "record_request_duration",
    "get_metrics",
    "CONTENT_TYPE_LATEST",
]

# API request counter
api_requests_total = Counter(
    "c4_api_requests_total",
    "Total number of API requests",
    ["endpoint", "method", "status"],
)

# Active workspace gauge
workspace_active = Gauge(
    "c4_workspaces_active",
    "Number of active workspaces",
)

# LLM token counter (labels: model, type where type is input/output)
llm_tokens_total = Counter(
    "c4_llm_tokens_total",
    "Total LLM tokens used",
    ["model", "type"],
)

# Task execution time histogram
task_duration = Histogram(
    "c4_task_duration_seconds",
    "Task completion time in seconds",
    ["task_type"],
    buckets=[0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0, 120.0, 300.0],
)

# LLM API request counter
llm_requests_total = Counter(
    "c4_llm_requests_total",
    "Total LLM API requests",
    ["model", "status"],
)

# Workspace operations counter (labels: operation, status where operation is create/destroy/exec)
workspace_operations_total = Counter(
    "c4_workspace_operations_total",
    "Total workspace operations",
    ["operation", "status"],
)

# HTTP request duration histogram
request_duration = Histogram(
    "c4_request_duration_seconds",
    "HTTP request duration in seconds",
    ["endpoint", "method"],
    buckets=[0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0],
)


def record_api_request(endpoint: str, method: str, status: int) -> None:
    """Record an API request.

    Args:
        endpoint: The API endpoint path
        method: HTTP method (GET, POST, etc.)
        status: HTTP status code
    """
    api_requests_total.labels(endpoint=endpoint, method=method, status=str(status)).inc()


def set_active_workspaces(count: int) -> None:
    """Set the number of active workspaces.

    Args:
        count: Number of active workspaces
    """
    workspace_active.set(count)


def record_llm_tokens(model: str, input_tokens: int, output_tokens: int) -> None:
    """Record LLM token usage.

    Args:
        model: Model name (e.g., 'gpt-4', 'claude-3')
        input_tokens: Number of input tokens
        output_tokens: Number of output tokens
    """
    llm_tokens_total.labels(model=model, type="input").inc(input_tokens)
    llm_tokens_total.labels(model=model, type="output").inc(output_tokens)


def record_task_duration(task_type: str, duration_seconds: float) -> None:
    """Record task execution duration.

    Args:
        task_type: Type of task (e.g., 'validation', 'build')
        duration_seconds: Duration in seconds
    """
    task_duration.labels(task_type=task_type).observe(duration_seconds)


def record_llm_request(model: str, status: str) -> None:
    """Record an LLM API request.

    Args:
        model: Model name (e.g., 'gpt-4', 'claude-3')
        status: Request status ('success', 'error', etc.)
    """
    llm_requests_total.labels(model=model, status=status).inc()


def record_workspace_operation(operation: str, status: str) -> None:
    """Record a workspace operation.

    Args:
        operation: Operation type ('create', 'destroy', 'exec')
        status: Operation status ('success', 'error', etc.)
    """
    workspace_operations_total.labels(operation=operation, status=status).inc()


def record_request_duration(endpoint: str, method: str, duration_seconds: float) -> None:
    """Record HTTP request duration.

    Args:
        endpoint: The API endpoint path
        method: HTTP method (GET, POST, etc.)
        duration_seconds: Request duration in seconds
    """
    request_duration.labels(endpoint=endpoint, method=method).observe(duration_seconds)


def get_metrics() -> bytes:
    """Get metrics in Prometheus format.

    Returns:
        Bytes containing Prometheus-formatted metrics
    """
    return generate_latest()
