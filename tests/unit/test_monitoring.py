"""Tests for C4 Cloud Monitoring - Sentry, Prometheus, Grafana, and Alerting."""

import json
from datetime import timedelta

import pytest

from infra.monitoring.alerting import (
    Alert,
    AlertManager,
    AlertRule,
    AlertSeverity,
    AlertState,
    NotificationChannel,
    NotificationConfig,
)
from infra.monitoring.grafana import (
    Dashboard,
    DashboardBuilder,
    Panel,
    PanelTarget,
    PanelType,
    Row,
    create_c4_overview_dashboard,
)
from infra.monitoring.metrics import (
    Counter,
    Gauge,
    Histogram,
    MetricsRegistry,
    c4_metrics,
)
from infra.monitoring.sentry import (
    SentryConfig,
    SentryEnvironment,
    get_sentry_status,
    init_sentry,
)

# =============================================================================
# Metrics Tests
# =============================================================================


class TestCounter:
    """Test Counter metric."""

    def test_initial_value(self):
        """Test counter starts at zero."""
        counter = Counter("test_counter")
        values = counter.get_all()
        assert len(values) == 0

    def test_increment(self):
        """Test incrementing counter."""
        counter = Counter("test_counter")
        counter.inc()
        counter.inc(5)

        values = counter.get_all()
        assert len(values) == 1
        assert values[0].value == 6

    def test_increment_with_labels(self):
        """Test incrementing counter with labels."""
        counter = Counter("test_counter", labels=["method", "status"])
        counter.inc(method="GET", status="200")
        counter.inc(2, method="POST", status="201")

        values = counter.get_all()
        assert len(values) == 2

    def test_negative_increment_raises(self):
        """Test negative increment raises error."""
        counter = Counter("test_counter")
        with pytest.raises(ValueError):
            counter.inc(-1)

    def test_labeled_counter(self):
        """Test labeled counter helper."""
        counter = Counter("test_counter", labels=["method"])
        labeled = counter.labels(method="GET")
        labeled.inc()
        labeled.inc(3)

        values = counter.get_all()
        assert values[0].value == 4
        assert values[0].labels == {"method": "GET"}

    def test_reset(self):
        """Test resetting counter."""
        counter = Counter("test_counter")
        counter.inc(10)
        counter.reset()

        values = counter.get_all()
        assert len(values) == 0


class TestGauge:
    """Test Gauge metric."""

    def test_set_value(self):
        """Test setting gauge value."""
        gauge = Gauge("test_gauge")
        gauge.set(42)

        values = gauge.get_all()
        assert len(values) == 1
        assert values[0].value == 42

    def test_inc_dec(self):
        """Test incrementing and decrementing gauge."""
        gauge = Gauge("test_gauge")
        gauge.set(10)
        gauge.inc(5)
        gauge.dec(3)

        values = gauge.get_all()
        assert values[0].value == 12

    def test_with_labels(self):
        """Test gauge with labels."""
        gauge = Gauge("test_gauge", labels=["region"])
        gauge.set(100, region="us-east")
        gauge.set(200, region="eu-west")

        values = gauge.get_all()
        assert len(values) == 2

    def test_labeled_gauge(self):
        """Test labeled gauge helper."""
        gauge = Gauge("test_gauge", labels=["type"])
        labeled = gauge.labels(type="worker")
        labeled.set(5)
        labeled.inc()
        labeled.dec(2)

        values = gauge.get_all()
        assert values[0].value == 4


class TestHistogram:
    """Test Histogram metric."""

    def test_observe(self):
        """Test observing values."""
        histogram = Histogram("test_histogram")
        histogram.observe(0.1)
        histogram.observe(0.5)
        histogram.observe(1.0)

        data = histogram.get_all()
        assert len(data) == 1
        assert data[0]["count"] == 3
        assert data[0]["sum"] == pytest.approx(1.6, rel=0.01)

    def test_buckets(self):
        """Test histogram buckets."""
        histogram = Histogram("test_histogram", buckets=(0.1, 0.5, 1.0))
        histogram.observe(0.05)
        histogram.observe(0.3)
        histogram.observe(0.8)
        histogram.observe(2.0)

        data = histogram.get_all()
        buckets = data[0]["buckets"]
        assert buckets[0.1] == 1
        assert buckets[0.5] == 2
        assert buckets[1.0] == 3
        assert buckets[float("inf")] == 4

    def test_with_labels(self):
        """Test histogram with labels."""
        histogram = Histogram("test_histogram", labels=["path"])
        histogram.observe(0.1, path="/api/v1")
        histogram.observe(0.2, path="/api/v2")

        data = histogram.get_all()
        assert len(data) == 2

    def test_timer(self):
        """Test histogram timer context manager."""
        histogram = Histogram("test_histogram")

        with histogram.time():
            pass  # Some operation

        data = histogram.get_all()
        assert data[0]["count"] == 1
        assert data[0]["sum"] > 0


class TestMetricsRegistry:
    """Test MetricsRegistry."""

    def test_register_counter(self):
        """Test registering counter."""
        registry = MetricsRegistry()
        counter = registry.counter("requests", "Total requests")

        assert counter.name == "requests"
        assert counter.description == "Total requests"

    def test_register_with_prefix(self):
        """Test registry with prefix."""
        registry = MetricsRegistry(prefix="app_")
        counter = registry.counter("requests")

        assert counter.name == "app_requests"

    def test_get_same_metric(self):
        """Test getting same metric returns same instance."""
        registry = MetricsRegistry()
        counter1 = registry.counter("requests")
        counter2 = registry.counter("requests")

        assert counter1 is counter2

    def test_export_prometheus(self):
        """Test exporting to Prometheus format."""
        registry = MetricsRegistry()
        counter = registry.counter("test_counter", "Test counter")
        counter.inc(10, method="GET")

        gauge = registry.gauge("test_gauge", "Test gauge")
        gauge.set(42)

        output = registry.export_prometheus()

        assert "# HELP test_counter Test counter" in output
        assert "# TYPE test_counter counter" in output
        assert 'test_counter{method="GET"} 10' in output
        assert "# TYPE test_gauge gauge" in output
        assert "test_gauge 42" in output

    def test_reset_all(self):
        """Test resetting all metrics."""
        registry = MetricsRegistry()
        registry.counter("test").inc(10)
        registry.gauge("test2").set(20)

        registry.reset_all()

        assert len(registry._counters["test"].get_all()) == 0


class TestGlobalMetrics:
    """Test global c4_metrics registry."""

    def test_c4_prefix(self):
        """Test c4_ prefix is applied."""
        counter = c4_metrics.counter("test_metric")
        assert counter.name == "c4_test_metric"


# =============================================================================
# Sentry Tests
# =============================================================================


class TestSentryConfig:
    """Test SentryConfig."""

    def test_default_values(self):
        """Test default configuration values."""
        config = SentryConfig()

        assert config.dsn is None
        assert config.environment == SentryEnvironment.DEVELOPMENT
        assert config.sample_rate == 1.0
        assert config.enabled is True

    def test_to_dict(self):
        """Test converting to dictionary."""
        config = SentryConfig(
            dsn="https://test@sentry.io/123",
            environment=SentryEnvironment.PRODUCTION,
        )
        data = config.to_dict()

        assert data["dsn"] == "https://test@sentry.io/123"
        assert data["environment"] == "production"


class TestSentryInit:
    """Test Sentry initialization."""

    def test_init_disabled(self):
        """Test initialization with disabled config."""
        config = SentryConfig(enabled=False)
        result = init_sentry(config)
        assert result is False

    def test_init_no_dsn(self):
        """Test initialization without DSN (mock mode)."""
        config = SentryConfig(dsn=None)
        result = init_sentry(config)
        # Should succeed in mock mode
        assert result is True

    def test_get_status(self):
        """Test getting Sentry status."""
        status = get_sentry_status()
        assert "initialized" in status
        assert "enabled" in status


# =============================================================================
# Alerting Tests
# =============================================================================


class TestAlert:
    """Test Alert dataclass."""

    def test_create_alert(self):
        """Test creating alert."""
        alert = Alert(
            id="alert-1",
            rule_name="high_cpu",
            severity=AlertSeverity.WARNING,
            state=AlertState.FIRING,
            message="CPU usage high",
        )

        assert alert.id == "alert-1"
        assert alert.severity == AlertSeverity.WARNING
        assert alert.state == AlertState.FIRING

    def test_to_dict(self):
        """Test converting alert to dict."""
        alert = Alert(
            id="alert-1",
            rule_name="test",
            severity=AlertSeverity.CRITICAL,
            state=AlertState.FIRING,
            message="Test alert",
            labels={"env": "prod"},
        )
        data = alert.to_dict()

        assert data["id"] == "alert-1"
        assert data["severity"] == "critical"
        assert data["labels"] == {"env": "prod"}


class TestAlertRule:
    """Test AlertRule."""

    def test_evaluate_true(self):
        """Test rule evaluation when threshold exceeded."""
        rule = AlertRule(
            name="test_rule",
            expression=lambda: 100,
            threshold=50,
            operator="gt",
        )

        is_firing, value = rule.evaluate()
        assert is_firing is True
        assert value == 100

    def test_evaluate_false(self):
        """Test rule evaluation when threshold not exceeded."""
        rule = AlertRule(
            name="test_rule",
            expression=lambda: 30,
            threshold=50,
            operator="gt",
        )

        is_firing, value = rule.evaluate()
        assert is_firing is False
        assert value == 30

    def test_operators(self):
        """Test different operators."""
        test_cases = [
            ("gt", 60, 50, True),
            ("gt", 50, 50, False),
            ("gte", 50, 50, True),
            ("lt", 40, 50, True),
            ("lte", 50, 50, True),
            ("eq", 50, 50, True),
        ]

        for op, value, threshold, expected in test_cases:
            rule = AlertRule(
                name=f"test_{op}",
                expression=lambda v=value: v,
                threshold=threshold,
                operator=op,
            )
            is_firing, _ = rule.evaluate()
            assert is_firing == expected, f"Failed for {op}: {value} {op} {threshold}"

    def test_evaluate_none_value(self):
        """Test evaluation when expression returns None."""
        rule = AlertRule(
            name="test_rule",
            expression=lambda: None,
            threshold=50,
            operator="gt",
        )

        is_firing, value = rule.evaluate()
        assert is_firing is False
        assert value is None


class TestAlertManager:
    """Test AlertManager."""

    @pytest.fixture
    def manager(self):
        """Create alert manager."""
        return AlertManager()

    def test_add_rule(self, manager):
        """Test adding rule."""
        rule = AlertRule(
            name="test",
            expression=lambda: 100,
            threshold=50,
            operator="gt",
        )
        manager.add_rule(rule)

        assert "test" in manager._rules

    def test_remove_rule(self, manager):
        """Test removing rule."""
        rule = AlertRule(
            name="test",
            expression=lambda: 100,
            threshold=50,
            operator="gt",
        )
        manager.add_rule(rule)
        result = manager.remove_rule("test")

        assert result is True
        assert "test" not in manager._rules

    @pytest.mark.asyncio
    async def test_evaluate_rules_firing(self, manager):
        """Test evaluating rules that should fire."""
        rule = AlertRule(
            name="test",
            expression=lambda: 100,
            threshold=50,
            operator="gt",
            for_duration=timedelta(seconds=0),  # Fire immediately
        )
        manager.add_rule(rule)

        # First evaluation - pending
        alerts1 = await manager.evaluate_rules()
        assert len(alerts1) == 0

        # Second evaluation - firing
        alerts2 = await manager.evaluate_rules()
        assert len(alerts2) == 1
        assert alerts2[0].state == AlertState.FIRING

    @pytest.mark.asyncio
    async def test_evaluate_rules_resolved(self, manager):
        """Test evaluating rules that resolve."""
        value = [100]  # Use list for mutable closure
        rule = AlertRule(
            name="test",
            expression=lambda: value[0],
            threshold=50,
            operator="gt",
            for_duration=timedelta(seconds=0),
        )
        manager.add_rule(rule)

        # Trigger alert
        await manager.evaluate_rules()
        await manager.evaluate_rules()

        # Resolve
        value[0] = 30
        alerts = await manager.evaluate_rules()

        assert len(alerts) == 1
        assert alerts[0].state == AlertState.RESOLVED

    def test_get_active_alerts(self, manager):
        """Test getting active alerts."""
        manager._alerts["test"] = Alert(
            id="alert-1",
            rule_name="test",
            severity=AlertSeverity.WARNING,
            state=AlertState.FIRING,
            message="Test",
        )
        manager._alerts["test2"] = Alert(
            id="alert-2",
            rule_name="test2",
            severity=AlertSeverity.WARNING,
            state=AlertState.RESOLVED,
            message="Test2",
        )

        active = manager.get_active_alerts()
        assert len(active) == 1
        assert active[0].id == "alert-1"

    def test_add_notification(self, manager):
        """Test adding notification channel."""
        config = NotificationConfig(
            channel=NotificationChannel.SLACK,
            name="slack-alerts",
            url="https://hooks.slack.com/test",
        )
        manager.add_notification(config)

        assert len(manager._notifications) == 1


class TestNotificationConfig:
    """Test NotificationConfig."""

    def test_slack_config(self):
        """Test Slack notification config."""
        config = NotificationConfig(
            channel=NotificationChannel.SLACK,
            name="slack-alerts",
            url="https://hooks.slack.com/test",
        )

        assert config.channel == NotificationChannel.SLACK
        assert config.enabled is True

    def test_webhook_config(self):
        """Test webhook notification config."""
        config = NotificationConfig(
            channel=NotificationChannel.WEBHOOK,
            name="custom-webhook",
            url="https://example.com/webhook",
            token="secret-token",
        )

        assert config.token == "secret-token"


# =============================================================================
# Grafana Tests
# =============================================================================


class TestPanelTarget:
    """Test PanelTarget."""

    def test_to_dict(self):
        """Test converting target to dict."""
        target = PanelTarget(
            expr='rate(http_requests_total[5m])',
            legend_format="{{method}}",
        )
        data = target.to_dict()

        assert data["expr"] == 'rate(http_requests_total[5m])'
        assert data["legendFormat"] == "{{method}}"


class TestPanel:
    """Test Panel."""

    def test_create_panel(self):
        """Test creating panel."""
        panel = Panel(
            id=1,
            title="Request Rate",
            type=PanelType.TIMESERIES,
            targets=[
                PanelTarget(expr='rate(http_requests_total[5m])')
            ],
        )

        assert panel.id == 1
        assert panel.type == PanelType.TIMESERIES

    def test_to_dict(self):
        """Test converting panel to dict."""
        panel = Panel(
            id=1,
            title="Test Panel",
            type=PanelType.STAT,
            targets=[PanelTarget(expr='sum(requests)')],
            unit="reqps",
        )
        data = panel.to_dict()

        assert data["id"] == 1
        assert data["title"] == "Test Panel"
        assert data["type"] == "stat"


class TestRow:
    """Test Row."""

    def test_create_row(self):
        """Test creating row."""
        row = Row(title="Overview", collapsed=False)

        assert row.title == "Overview"
        assert row.collapsed is False

    def test_to_dict(self):
        """Test converting row to dict."""
        row = Row(title="Test Row")
        data = row.to_dict(y_position=0)

        assert data["title"] == "Test Row"
        assert data["type"] == "row"


class TestDashboard:
    """Test Dashboard."""

    def test_create_dashboard(self):
        """Test creating dashboard."""
        dashboard = Dashboard(
            uid="test-dashboard",
            title="Test Dashboard",
            description="A test dashboard",
            tags=["test", "monitoring"],
        )

        assert dashboard.uid == "test-dashboard"
        assert len(dashboard.tags) == 2

    def test_to_dict(self):
        """Test converting dashboard to dict."""
        dashboard = Dashboard(
            uid="test",
            title="Test",
            panels=[
                Panel(
                    id=1,
                    title="Test Panel",
                    type=PanelType.STAT,
                    targets=[PanelTarget(expr='test')],
                )
            ],
        )
        data = dashboard.to_dict()

        assert data["uid"] == "test"
        assert data["schemaVersion"] == 36
        assert len(data["panels"]) == 1

    def test_to_json(self):
        """Test exporting dashboard as JSON."""
        dashboard = Dashboard(uid="test", title="Test")
        json_str = dashboard.to_json()

        data = json.loads(json_str)
        assert data["uid"] == "test"


class TestDashboardBuilder:
    """Test DashboardBuilder."""

    def test_basic_build(self):
        """Test basic dashboard build."""
        dashboard = (
            DashboardBuilder("test-uid", "Test Dashboard")
            .description("Test description")
            .tags("test", "monitoring")
            .build()
        )

        assert dashboard.uid == "test-uid"
        assert dashboard.title == "Test Dashboard"
        assert "test" in dashboard.tags

    def test_add_row(self):
        """Test adding row."""
        dashboard = (
            DashboardBuilder("test", "Test")
            .row("Overview")
            .stat("Requests", 'sum(requests)')
            .build()
        )

        assert len(dashboard.rows) == 1
        assert dashboard.rows[0].title == "Overview"
        assert len(dashboard.rows[0].panels) == 1

    def test_multiple_panels(self):
        """Test adding multiple panels."""
        dashboard = (
            DashboardBuilder("test", "Test")
            .stat("Stat 1", 'expr1')
            .timeseries("Graph 1", 'expr2')
            .gauge("Gauge 1", 'expr3')
            .build()
        )

        assert len(dashboard.panels) == 3


class TestPredefinedDashboards:
    """Test predefined dashboards."""

    def test_overview_dashboard(self):
        """Test C4 overview dashboard."""
        dashboard = create_c4_overview_dashboard()

        assert dashboard.uid == "c4-overview"
        assert "c4" in dashboard.tags
        assert len(dashboard.rows) > 0

    def test_overview_dashboard_json(self):
        """Test overview dashboard exports to valid JSON."""
        dashboard = create_c4_overview_dashboard()
        json_str = dashboard.to_json()

        # Should be valid JSON
        data = json.loads(json_str)
        assert "panels" in data


# =============================================================================
# Integration Tests
# =============================================================================


class TestMonitoringIntegration:
    """Test integration between monitoring components."""

    def test_metrics_to_alert_rule(self):
        """Test using metrics in alert rules."""
        registry = MetricsRegistry()
        counter = registry.counter("errors", labels=["status"])
        counter.inc(100, status="500")

        def error_count() -> float:
            for mv in counter.get_all():
                if mv.labels.get("status") == "500":
                    return mv.value
            return 0

        rule = AlertRule(
            name="high_errors",
            expression=error_count,
            threshold=50,
            operator="gt",
        )

        is_firing, value = rule.evaluate()
        assert is_firing is True
        assert value == 100
