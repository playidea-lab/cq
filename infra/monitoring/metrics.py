"""C4 Prometheus Metrics - Custom metrics collection and export."""

from __future__ import annotations

import logging
import time
from collections import defaultdict
from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Callable

logger = logging.getLogger(__name__)


class MetricType(str, Enum):
    """Prometheus metric types."""

    COUNTER = "counter"
    GAUGE = "gauge"
    HISTOGRAM = "histogram"
    SUMMARY = "summary"


@dataclass
class MetricValue:
    """Single metric value with labels."""

    value: float
    labels: dict[str, str]
    timestamp: float = field(default_factory=time.time)


class Counter:
    """Prometheus counter metric."""

    def __init__(
        self,
        name: str,
        description: str = "",
        labels: list[str] | None = None,
    ):
        """Initialize counter.

        Args:
            name: Metric name
            description: Metric description
            labels: Label names
        """
        self.name = name
        self.description = description
        self.label_names = labels or []
        self._values: dict[tuple, float] = defaultdict(float)

    def inc(self, value: float = 1.0, **labels: str) -> None:
        """Increment counter.

        Args:
            value: Value to add (default: 1)
            **labels: Label values
        """
        if value < 0:
            raise ValueError("Counter can only be incremented")
        key = self._make_key(labels)
        self._values[key] += value

    def labels(self, **labels: str) -> "CounterLabeled":
        """Get labeled counter for specific label values.

        Args:
            **labels: Label values

        Returns:
            Labeled counter instance
        """
        return CounterLabeled(self, labels)

    def _make_key(self, labels: dict[str, str]) -> tuple:
        """Create hashable key from labels."""
        return tuple(sorted(labels.items()))

    def get_all(self) -> list[MetricValue]:
        """Get all metric values.

        Returns:
            List of metric values with labels
        """
        result = []
        for key, value in self._values.items():
            labels = dict(key)
            result.append(MetricValue(value=value, labels=labels))
        return result

    def reset(self) -> None:
        """Reset counter to zero."""
        self._values.clear()


class CounterLabeled:
    """Counter with pre-set labels."""

    def __init__(self, counter: Counter, labels: dict[str, str]):
        self._counter = counter
        self._labels = labels

    def inc(self, value: float = 1.0) -> None:
        """Increment counter."""
        self._counter.inc(value, **self._labels)


class Gauge:
    """Prometheus gauge metric."""

    def __init__(
        self,
        name: str,
        description: str = "",
        labels: list[str] | None = None,
    ):
        """Initialize gauge.

        Args:
            name: Metric name
            description: Metric description
            labels: Label names
        """
        self.name = name
        self.description = description
        self.label_names = labels or []
        self._values: dict[tuple, float] = defaultdict(float)

    def set(self, value: float, **labels: str) -> None:
        """Set gauge value.

        Args:
            value: Value to set
            **labels: Label values
        """
        key = self._make_key(labels)
        self._values[key] = value

    def inc(self, value: float = 1.0, **labels: str) -> None:
        """Increment gauge.

        Args:
            value: Value to add
            **labels: Label values
        """
        key = self._make_key(labels)
        self._values[key] += value

    def dec(self, value: float = 1.0, **labels: str) -> None:
        """Decrement gauge.

        Args:
            value: Value to subtract
            **labels: Label values
        """
        key = self._make_key(labels)
        self._values[key] -= value

    def labels(self, **labels: str) -> "GaugeLabeled":
        """Get labeled gauge for specific label values.

        Args:
            **labels: Label values

        Returns:
            Labeled gauge instance
        """
        return GaugeLabeled(self, labels)

    def _make_key(self, labels: dict[str, str]) -> tuple:
        """Create hashable key from labels."""
        return tuple(sorted(labels.items()))

    def get_all(self) -> list[MetricValue]:
        """Get all metric values.

        Returns:
            List of metric values with labels
        """
        result = []
        for key, value in self._values.items():
            labels = dict(key)
            result.append(MetricValue(value=value, labels=labels))
        return result

    def reset(self) -> None:
        """Reset gauge to zero."""
        self._values.clear()


class GaugeLabeled:
    """Gauge with pre-set labels."""

    def __init__(self, gauge: Gauge, labels: dict[str, str]):
        self._gauge = gauge
        self._labels = labels

    def set(self, value: float) -> None:
        """Set gauge value."""
        self._gauge.set(value, **self._labels)

    def inc(self, value: float = 1.0) -> None:
        """Increment gauge."""
        self._gauge.inc(value, **self._labels)

    def dec(self, value: float = 1.0) -> None:
        """Decrement gauge."""
        self._gauge.dec(value, **self._labels)


class Histogram:
    """Prometheus histogram metric."""

    DEFAULT_BUCKETS = (0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0)

    def __init__(
        self,
        name: str,
        description: str = "",
        labels: list[str] | None = None,
        buckets: tuple[float, ...] | None = None,
    ):
        """Initialize histogram.

        Args:
            name: Metric name
            description: Metric description
            labels: Label names
            buckets: Bucket boundaries
        """
        self.name = name
        self.description = description
        self.label_names = labels or []
        self.buckets = buckets or self.DEFAULT_BUCKETS
        self._values: dict[tuple, list[float]] = defaultdict(list)

    def observe(self, value: float, **labels: str) -> None:
        """Observe a value.

        Args:
            value: Value to observe
            **labels: Label values
        """
        key = self._make_key(labels)
        self._values[key].append(value)

    def labels(self, **labels: str) -> "HistogramLabeled":
        """Get labeled histogram for specific label values.

        Args:
            **labels: Label values

        Returns:
            Labeled histogram instance
        """
        return HistogramLabeled(self, labels)

    def time(self, **labels: str) -> "HistogramTimer":
        """Return a timer context manager.

        Args:
            **labels: Label values

        Returns:
            Timer context manager
        """
        return HistogramTimer(self, labels)

    def _make_key(self, labels: dict[str, str]) -> tuple:
        """Create hashable key from labels."""
        return tuple(sorted(labels.items()))

    def get_all(self) -> list[dict[str, Any]]:
        """Get all histogram data.

        Returns:
            List of histogram data with buckets
        """
        result = []
        for key, values in self._values.items():
            labels = dict(key)
            bucket_counts = {b: 0 for b in self.buckets}
            bucket_counts[float("inf")] = 0

            # Count each value in its bucket (non-cumulative first)
            sorted_buckets = sorted(bucket_counts.keys())
            for v in values:
                for b in sorted_buckets:
                    if v <= b:
                        bucket_counts[b] += 1
                        break  # Count only in first matching bucket

            # Make cumulative (Prometheus histogram format)
            cumulative = {}
            running = 0
            for b in sorted_buckets:
                running += bucket_counts[b]
                cumulative[b] = running

            result.append(
                {
                    "labels": labels,
                    "count": len(values),
                    "sum": sum(values) if values else 0,
                    "buckets": cumulative,
                }
            )
        return result

    def reset(self) -> None:
        """Reset histogram."""
        self._values.clear()


class HistogramLabeled:
    """Histogram with pre-set labels."""

    def __init__(self, histogram: Histogram, labels: dict[str, str]):
        self._histogram = histogram
        self._labels = labels

    def observe(self, value: float) -> None:
        """Observe a value."""
        self._histogram.observe(value, **self._labels)

    def time(self) -> "HistogramTimer":
        """Return a timer context manager."""
        return HistogramTimer(self._histogram, self._labels)


class HistogramTimer:
    """Timer context manager for histogram."""

    def __init__(self, histogram: Histogram, labels: dict[str, str]):
        self._histogram = histogram
        self._labels = labels
        self._start: float | None = None

    def __enter__(self) -> "HistogramTimer":
        self._start = time.perf_counter()
        return self

    def __exit__(self, *args: Any) -> None:
        if self._start is not None:
            duration = time.perf_counter() - self._start
            self._histogram.observe(duration, **self._labels)


class MetricsRegistry:
    """Registry for all metrics."""

    def __init__(self, prefix: str = ""):
        """Initialize registry.

        Args:
            prefix: Metric name prefix
        """
        self.prefix = prefix
        self._counters: dict[str, Counter] = {}
        self._gauges: dict[str, Gauge] = {}
        self._histograms: dict[str, Histogram] = {}

    def counter(
        self,
        name: str,
        description: str = "",
        labels: list[str] | None = None,
    ) -> Counter:
        """Register or get a counter.

        Args:
            name: Metric name
            description: Metric description
            labels: Label names

        Returns:
            Counter instance
        """
        full_name = f"{self.prefix}{name}" if self.prefix else name
        if full_name not in self._counters:
            self._counters[full_name] = Counter(full_name, description, labels)
        return self._counters[full_name]

    def gauge(
        self,
        name: str,
        description: str = "",
        labels: list[str] | None = None,
    ) -> Gauge:
        """Register or get a gauge.

        Args:
            name: Metric name
            description: Metric description
            labels: Label names

        Returns:
            Gauge instance
        """
        full_name = f"{self.prefix}{name}" if self.prefix else name
        if full_name not in self._gauges:
            self._gauges[full_name] = Gauge(full_name, description, labels)
        return self._gauges[full_name]

    def histogram(
        self,
        name: str,
        description: str = "",
        labels: list[str] | None = None,
        buckets: tuple[float, ...] | None = None,
    ) -> Histogram:
        """Register or get a histogram.

        Args:
            name: Metric name
            description: Metric description
            labels: Label names
            buckets: Bucket boundaries

        Returns:
            Histogram instance
        """
        full_name = f"{self.prefix}{name}" if self.prefix else name
        if full_name not in self._histograms:
            self._histograms[full_name] = Histogram(
                full_name, description, labels, buckets
            )
        return self._histograms[full_name]

    def export_prometheus(self) -> str:
        """Export all metrics in Prometheus text format.

        Returns:
            Prometheus format string
        """
        lines = []

        # Export counters
        for name, counter in self._counters.items():
            lines.append(f"# HELP {name} {counter.description}")
            lines.append(f"# TYPE {name} counter")
            for mv in counter.get_all():
                labels_str = self._format_labels(mv.labels)
                lines.append(f"{name}{labels_str} {mv.value}")

        # Export gauges
        for name, gauge in self._gauges.items():
            lines.append(f"# HELP {name} {gauge.description}")
            lines.append(f"# TYPE {name} gauge")
            for mv in gauge.get_all():
                labels_str = self._format_labels(mv.labels)
                lines.append(f"{name}{labels_str} {mv.value}")

        # Export histograms
        for name, histogram in self._histograms.items():
            lines.append(f"# HELP {name} {histogram.description}")
            lines.append(f"# TYPE {name} histogram")
            for hdata in histogram.get_all():
                labels_str = self._format_labels(hdata["labels"])
                for bucket, count in sorted(hdata["buckets"].items()):
                    le = "+Inf" if bucket == float("inf") else str(bucket)
                    bucket_labels = self._format_labels(
                        {**hdata["labels"], "le": le}
                    )
                    lines.append(f"{name}_bucket{bucket_labels} {count}")
                lines.append(f"{name}_count{labels_str} {hdata['count']}")
                lines.append(f"{name}_sum{labels_str} {hdata['sum']}")

        return "\n".join(lines)

    def _format_labels(self, labels: dict[str, str]) -> str:
        """Format labels for Prometheus output."""
        if not labels:
            return ""
        parts = [f'{k}="{v}"' for k, v in sorted(labels.items())]
        return "{" + ",".join(parts) + "}"

    def reset_all(self) -> None:
        """Reset all metrics."""
        for counter in self._counters.values():
            counter.reset()
        for gauge in self._gauges.values():
            gauge.reset()
        for histogram in self._histograms.values():
            histogram.reset()


# Global metrics registry
c4_metrics = MetricsRegistry(prefix="c4_")

# Pre-defined C4 metrics
http_requests_total = c4_metrics.counter(
    "http_requests_total",
    "Total HTTP requests",
    ["method", "path", "status"],
)

http_request_duration_seconds = c4_metrics.histogram(
    "http_request_duration_seconds",
    "HTTP request duration in seconds",
    ["method", "path"],
    buckets=(0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0),
)

task_operations_total = c4_metrics.counter(
    "task_operations_total",
    "Total task operations",
    ["operation", "status"],
)

active_workers = c4_metrics.gauge(
    "active_workers",
    "Number of active workers",
    ["domain"],
)

llm_requests_total = c4_metrics.counter(
    "llm_requests_total",
    "Total LLM API requests",
    ["model", "status"],
)

llm_tokens_total = c4_metrics.counter(
    "llm_tokens_total",
    "Total LLM tokens used",
    ["model", "type"],
)


class MetricsMiddleware:
    """FastAPI middleware for request metrics."""

    def __init__(self, app: Any, registry: MetricsRegistry | None = None):
        """Initialize middleware.

        Args:
            app: FastAPI application
            registry: Metrics registry (default: c4_metrics)
        """
        self.app = app
        self.registry = registry or c4_metrics

    async def __call__(self, scope: dict, receive: Callable, send: Callable) -> None:
        """Handle request.

        Args:
            scope: ASGI scope
            receive: Receive callable
            send: Send callable
        """
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        method = scope.get("method", "UNKNOWN")
        path = scope.get("path", "/")
        start_time = time.perf_counter()

        # Capture status code from response
        status_code = "500"

        async def send_wrapper(message: dict) -> None:
            nonlocal status_code
            if message["type"] == "http.response.start":
                status_code = str(message.get("status", 500))
            await send(message)

        try:
            await self.app(scope, receive, send_wrapper)
        finally:
            duration = time.perf_counter() - start_time

            # Record metrics
            http_requests_total.inc(method=method, path=path, status=status_code)
            http_request_duration_seconds.observe(duration, method=method, path=path)


def create_metrics_middleware(
    app: Any, registry: MetricsRegistry | None = None
) -> MetricsMiddleware:
    """Create metrics middleware for FastAPI.

    Args:
        app: FastAPI application
        registry: Metrics registry

    Returns:
        Configured middleware
    """
    return MetricsMiddleware(app, registry)
