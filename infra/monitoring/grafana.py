"""C4 Grafana Integration - Dashboard generation and configuration."""

from __future__ import annotations

import json
import logging
from dataclasses import dataclass, field
from enum import Enum
from typing import Any

logger = logging.getLogger(__name__)


class PanelType(str, Enum):
    """Grafana panel types."""

    GRAPH = "graph"
    STAT = "stat"
    GAUGE = "gauge"
    TABLE = "table"
    TIMESERIES = "timeseries"
    HEATMAP = "heatmap"
    PIE = "piechart"
    LOG = "logs"
    TEXT = "text"


class DataSource(str, Enum):
    """Data source types."""

    PROMETHEUS = "prometheus"
    LOKI = "loki"
    ELASTICSEARCH = "elasticsearch"


@dataclass
class PanelTarget:
    """Query target for a panel."""

    expr: str
    legend_format: str = ""
    ref_id: str = "A"
    instant: bool = False
    interval: str = ""

    def to_dict(self) -> dict[str, Any]:
        """Convert to Grafana target format."""
        return {
            "expr": self.expr,
            "legendFormat": self.legend_format,
            "refId": self.ref_id,
            "instant": self.instant,
            "interval": self.interval,
        }


@dataclass
class Panel:
    """Grafana dashboard panel."""

    id: int
    title: str
    type: PanelType
    targets: list[PanelTarget]
    grid_pos: dict[str, int] = field(default_factory=lambda: {"x": 0, "y": 0, "w": 12, "h": 8})
    datasource: str = "${DS_PROMETHEUS}"
    description: str = ""
    unit: str = ""
    thresholds: list[dict[str, Any]] = field(default_factory=list)
    options: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        """Convert to Grafana panel format."""
        panel = {
            "id": self.id,
            "title": self.title,
            "type": self.type.value,
            "gridPos": self.grid_pos,
            "datasource": self.datasource,
            "description": self.description,
            "targets": [t.to_dict() for t in self.targets],
        }

        if self.type == PanelType.TIMESERIES:
            panel["fieldConfig"] = {
                "defaults": {
                    "unit": self.unit,
                    "thresholds": {
                        "mode": "absolute",
                        "steps": self.thresholds or [
                            {"color": "green", "value": None},
                        ],
                    },
                },
                "overrides": [],
            }
            panel["options"] = self.options or {
                "tooltip": {"mode": "single"},
                "legend": {"displayMode": "list", "placement": "bottom"},
            }

        elif self.type == PanelType.STAT:
            panel["fieldConfig"] = {
                "defaults": {
                    "unit": self.unit,
                    "thresholds": {
                        "mode": "absolute",
                        "steps": self.thresholds or [
                            {"color": "green", "value": None},
                        ],
                    },
                },
                "overrides": [],
            }
            panel["options"] = self.options or {
                "colorMode": "value",
                "graphMode": "area",
                "justifyMode": "auto",
                "textMode": "auto",
            }

        elif self.type == PanelType.GAUGE:
            panel["fieldConfig"] = {
                "defaults": {
                    "unit": self.unit,
                    "min": 0,
                    "max": 100,
                    "thresholds": {
                        "mode": "absolute",
                        "steps": self.thresholds or [
                            {"color": "green", "value": None},
                            {"color": "yellow", "value": 70},
                            {"color": "red", "value": 90},
                        ],
                    },
                },
                "overrides": [],
            }

        elif self.type == PanelType.TABLE:
            panel["options"] = self.options or {
                "showHeader": True,
            }

        return panel


@dataclass
class Row:
    """Dashboard row for organizing panels."""

    title: str
    collapsed: bool = False
    panels: list[Panel] = field(default_factory=list)

    def to_dict(self, y_position: int) -> dict[str, Any]:
        """Convert to Grafana row format."""
        return {
            "collapsed": self.collapsed,
            "gridPos": {"h": 1, "w": 24, "x": 0, "y": y_position},
            "id": hash(self.title) % 1000000,
            "panels": [],
            "title": self.title,
            "type": "row",
        }


@dataclass
class Dashboard:
    """Grafana dashboard."""

    uid: str
    title: str
    description: str = ""
    tags: list[str] = field(default_factory=list)
    panels: list[Panel] = field(default_factory=list)
    rows: list[Row] = field(default_factory=list)
    refresh: str = "30s"
    time_from: str = "now-1h"
    time_to: str = "now"
    timezone: str = "browser"
    editable: bool = True
    version: int = 1

    def to_dict(self) -> dict[str, Any]:
        """Convert to Grafana dashboard format."""
        all_panels = []
        y_pos = 0

        # Add rows and their panels
        for row in self.rows:
            all_panels.append(row.to_dict(y_pos))
            y_pos += 1

            for panel in row.panels:
                panel.grid_pos["y"] = y_pos
                all_panels.append(panel.to_dict())
                y_pos += panel.grid_pos.get("h", 8)

        # Add standalone panels
        for panel in self.panels:
            if "y" not in panel.grid_pos:
                panel.grid_pos["y"] = y_pos
            all_panels.append(panel.to_dict())
            y_pos += panel.grid_pos.get("h", 8)

        return {
            "uid": self.uid,
            "title": self.title,
            "description": self.description,
            "tags": self.tags,
            "timezone": self.timezone,
            "editable": self.editable,
            "refresh": self.refresh,
            "time": {
                "from": self.time_from,
                "to": self.time_to,
            },
            "panels": all_panels,
            "schemaVersion": 36,
            "version": self.version,
            "templating": {
                "list": [],
            },
            "annotations": {
                "list": [],
            },
        }

    def to_json(self, indent: int = 2) -> str:
        """Export dashboard as JSON.

        Args:
            indent: JSON indentation

        Returns:
            JSON string
        """
        return json.dumps(self.to_dict(), indent=indent)

    def save(self, path: str) -> None:
        """Save dashboard to file.

        Args:
            path: File path
        """
        with open(path, "w") as f:
            f.write(self.to_json())
        logger.info(f"Saved dashboard to {path}")


class DashboardBuilder:
    """Builder for creating Grafana dashboards."""

    def __init__(self, uid: str, title: str):
        """Initialize builder.

        Args:
            uid: Dashboard UID
            title: Dashboard title
        """
        self._uid = uid
        self._title = title
        self._description = ""
        self._tags: list[str] = []
        self._panels: list[Panel] = []
        self._rows: list[Row] = []
        self._panel_id = 1
        self._current_row: Row | None = None

    def description(self, desc: str) -> "DashboardBuilder":
        """Set description."""
        self._description = desc
        return self

    def tags(self, *tags: str) -> "DashboardBuilder":
        """Add tags."""
        self._tags.extend(tags)
        return self

    def row(self, title: str, collapsed: bool = False) -> "DashboardBuilder":
        """Start a new row.

        Args:
            title: Row title
            collapsed: Whether collapsed by default
        """
        if self._current_row:
            self._rows.append(self._current_row)
        self._current_row = Row(title=title, collapsed=collapsed)
        return self

    def add_panel(
        self,
        title: str,
        panel_type: PanelType,
        expr: str,
        legend: str = "",
        unit: str = "",
        width: int = 12,
        height: int = 8,
        **kwargs: Any,
    ) -> "DashboardBuilder":
        """Add a panel.

        Args:
            title: Panel title
            panel_type: Panel type
            expr: Prometheus expression
            legend: Legend format
            unit: Value unit
            width: Panel width
            height: Panel height
            **kwargs: Additional options
        """
        panel = Panel(
            id=self._panel_id,
            title=title,
            type=panel_type,
            targets=[PanelTarget(expr=expr, legend_format=legend)],
            grid_pos={"x": 0, "y": 0, "w": width, "h": height},
            unit=unit,
            **kwargs,
        )
        self._panel_id += 1

        if self._current_row:
            self._current_row.panels.append(panel)
        else:
            self._panels.append(panel)

        return self

    def stat(
        self, title: str, expr: str, unit: str = "", width: int = 6, height: int = 4
    ) -> "DashboardBuilder":
        """Add stat panel."""
        return self.add_panel(title, PanelType.STAT, expr, unit=unit, width=width, height=height)

    def timeseries(
        self,
        title: str,
        expr: str,
        legend: str = "",
        unit: str = "",
        width: int = 12,
        height: int = 8,
    ) -> "DashboardBuilder":
        """Add timeseries panel."""
        return self.add_panel(
            title, PanelType.TIMESERIES, expr, legend, unit, width, height
        )

    def gauge(
        self, title: str, expr: str, unit: str = "percent", width: int = 6, height: int = 6
    ) -> "DashboardBuilder":
        """Add gauge panel."""
        return self.add_panel(title, PanelType.GAUGE, expr, unit=unit, width=width, height=height)

    def table(
        self, title: str, expr: str, width: int = 24, height: int = 8
    ) -> "DashboardBuilder":
        """Add table panel."""
        return self.add_panel(title, PanelType.TABLE, expr, width=width, height=height)

    def build(self) -> Dashboard:
        """Build the dashboard.

        Returns:
            Dashboard instance
        """
        if self._current_row:
            self._rows.append(self._current_row)

        return Dashboard(
            uid=self._uid,
            title=self._title,
            description=self._description,
            tags=self._tags,
            panels=self._panels,
            rows=self._rows,
        )


def create_c4_overview_dashboard() -> Dashboard:
    """Create C4 overview dashboard.

    Returns:
        Dashboard for C4 system overview
    """
    # Prometheus query expressions
    error_rate_expr = (
        'sum(rate(c4_http_requests_total{status=~"5.."}[5m])) '
        '/ sum(rate(c4_http_requests_total[5m])) * 100'
    )
    p99_latency_expr = (
        'histogram_quantile(0.99, '
        'sum(rate(c4_http_request_duration_seconds_bucket[5m])) by (le))'
    )
    http_error_rate_expr = (
        'sum(rate(c4_http_requests_total{status=~"5.."}[5m])) by (path)'
    )

    return (
        DashboardBuilder("c4-overview", "C4 Overview")
        .description("C4 System Overview Dashboard")
        .tags("c4", "overview")
        # Summary row
        .row("Summary")
        .stat("Total Requests", 'sum(c4_http_requests_total)', width=4)
        .stat("Error Rate", error_rate_expr, unit="percent", width=4)
        .stat("Active Workers", 'sum(c4_active_workers)', width=4)
        .stat("P99 Latency", p99_latency_expr, unit="s", width=4)
        .stat(
            "Tasks Completed",
            'sum(c4_task_operations_total{status="completed"})',
            width=4,
        )
        .stat("LLM Requests", 'sum(c4_llm_requests_total)', width=4)
        # HTTP row
        .row("HTTP Metrics")
        .timeseries(
            "Request Rate",
            'sum(rate(c4_http_requests_total[5m])) by (path)',
            legend="{{path}}",
            unit="reqps",
        )
        .timeseries(
            "Error Rate", http_error_rate_expr, legend="{{path}}", unit="reqps"
        )
        .timeseries("Latency (P50, P95, P99)", p99_latency_expr, unit="s")
        # Tasks row
        .row("Task Metrics")
        .timeseries(
            "Task Operations",
            'sum(rate(c4_task_operations_total[5m])) by (operation)',
            legend="{{operation}}",
            unit="ops",
        )
        .timeseries("Worker Count", 'c4_active_workers', legend="{{domain}}")
        # LLM row
        .row("LLM Metrics")
        .timeseries(
            "LLM Requests",
            'sum(rate(c4_llm_requests_total[5m])) by (model)',
            legend="{{model}}",
            unit="reqps",
        )
        .timeseries(
            "Token Usage",
            'sum(rate(c4_llm_tokens_total[5m])) by (model, type)',
            legend="{{model}} ({{type}})",
            unit="tok/s",
        )
        .build()
    )


def create_c4_api_dashboard() -> Dashboard:
    """Create C4 API metrics dashboard.

    Returns:
        Dashboard for API metrics
    """
    # Prometheus query expressions
    success_rate_expr = (
        '(1 - sum(rate(c4_http_requests_total{status=~"5.."}[5m])) '
        '/ sum(rate(c4_http_requests_total[5m]))) * 100'
    )
    avg_latency_expr = (
        'sum(rate(c4_http_request_duration_seconds_sum[5m])) '
        '/ sum(rate(c4_http_request_duration_seconds_count[5m]))'
    )
    p50_latency_expr = (
        'histogram_quantile(0.50, '
        'sum(rate(c4_http_request_duration_seconds_bucket[5m])) by (le, path))'
    )
    p99_latency_expr = (
        'histogram_quantile(0.99, '
        'sum(rate(c4_http_request_duration_seconds_bucket[5m])) by (le, path))'
    )

    return (
        DashboardBuilder("c4-api", "C4 API Metrics")
        .description("C4 API Performance Dashboard")
        .tags("c4", "api")
        # Overview
        .row("Overview")
        .stat(
            "Request Rate",
            'sum(rate(c4_http_requests_total[5m]))',
            unit="reqps",
            width=6,
        )
        .stat("Success Rate", success_rate_expr, unit="percent", width=6)
        .stat("Avg Latency", avg_latency_expr, unit="s", width=6)
        .stat("Active Connections", 'c4_active_connections', width=6)
        # Latency
        .row("Latency")
        .timeseries(
            "Latency by Path (P50)", p50_latency_expr, legend="{{path}}", unit="s"
        )
        .timeseries(
            "Latency by Path (P99)", p99_latency_expr, legend="{{path}}", unit="s"
        )
        # Status codes
        .row("Status Codes")
        .timeseries(
            "Status Code Distribution",
            'sum(rate(c4_http_requests_total[5m])) by (status)',
            legend="{{status}}",
            unit="reqps",
        )
        .build()
    )


def create_c4_alerts_dashboard() -> Dashboard:
    """Create C4 alerts dashboard.

    Returns:
        Dashboard for alerts
    """
    # Prometheus query expressions
    critical_alerts_expr = 'count(ALERTS{severity="critical", alertstate="firing"})'
    warning_alerts_expr = 'count(ALERTS{severity="warning", alertstate="firing"})'
    alert_rate_expr = (
        'sum(rate(ALERTS{alertstate="firing"}[5m])) by (alertname)'
    )

    return (
        DashboardBuilder("c4-alerts", "C4 Alerts")
        .description("C4 Alert Status Dashboard")
        .tags("c4", "alerts")
        .row("Active Alerts")
        .stat("Critical Alerts", critical_alerts_expr, width=6)
        .stat("Warning Alerts", warning_alerts_expr, width=6)
        .row("Alert History")
        .timeseries("Alert Rate", alert_rate_expr, legend="{{alertname}}")
        .build()
    )
