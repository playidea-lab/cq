"""C4 Alerting - Alert rules, notifications, and alert management."""

from __future__ import annotations

import asyncio
import json
import logging
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from enum import Enum
from typing import Any, Callable
from urllib.error import URLError
from urllib.request import Request, urlopen

logger = logging.getLogger(__name__)


class AlertSeverity(str, Enum):
    """Alert severity levels."""

    CRITICAL = "critical"
    WARNING = "warning"
    INFO = "info"


class AlertState(str, Enum):
    """Alert states."""

    PENDING = "pending"
    FIRING = "firing"
    RESOLVED = "resolved"


class NotificationChannel(str, Enum):
    """Notification channel types."""

    SLACK = "slack"
    WEBHOOK = "webhook"
    EMAIL = "email"
    PAGERDUTY = "pagerduty"


@dataclass
class Alert:
    """Single alert instance."""

    id: str
    rule_name: str
    severity: AlertSeverity
    state: AlertState
    message: str
    labels: dict[str, str] = field(default_factory=dict)
    annotations: dict[str, str] = field(default_factory=dict)
    started_at: datetime = field(default_factory=datetime.now)
    ended_at: datetime | None = None
    value: float | None = None

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "id": self.id,
            "rule_name": self.rule_name,
            "severity": self.severity.value,
            "state": self.state.value,
            "message": self.message,
            "labels": self.labels,
            "annotations": self.annotations,
            "started_at": self.started_at.isoformat(),
            "ended_at": self.ended_at.isoformat() if self.ended_at else None,
            "value": self.value,
        }


@dataclass
class AlertRule:
    """Alert rule definition."""

    name: str
    expression: Callable[[], float | None]
    threshold: float
    operator: str  # gt, lt, gte, lte, eq
    severity: AlertSeverity = AlertSeverity.WARNING
    for_duration: timedelta = field(default_factory=lambda: timedelta(minutes=5))
    labels: dict[str, str] = field(default_factory=dict)
    annotations: dict[str, str] = field(default_factory=dict)
    enabled: bool = True

    def evaluate(self) -> tuple[bool, float | None]:
        """Evaluate alert condition.

        Returns:
            Tuple of (is_firing, current_value)
        """
        try:
            value = self.expression()
            if value is None:
                return False, None

            ops = {
                "gt": lambda v, t: v > t,
                "lt": lambda v, t: v < t,
                "gte": lambda v, t: v >= t,
                "lte": lambda v, t: v <= t,
                "eq": lambda v, t: v == t,
            }

            op_func = ops.get(self.operator, ops["gt"])
            return op_func(value, self.threshold), value

        except Exception as e:
            logger.error(f"Failed to evaluate rule {self.name}: {e}")
            return False, None


@dataclass
class NotificationConfig:
    """Notification channel configuration."""

    channel: NotificationChannel
    name: str
    url: str | None = None
    token: str | None = None
    email: str | None = None
    enabled: bool = True
    severity_filter: list[AlertSeverity] = field(
        default_factory=lambda: [AlertSeverity.CRITICAL, AlertSeverity.WARNING]
    )


class AlertManager:
    """Manage alert rules, evaluation, and notifications."""

    def __init__(self):
        """Initialize alert manager."""
        self._rules: dict[str, AlertRule] = {}
        self._alerts: dict[str, Alert] = {}
        self._pending_since: dict[str, datetime] = {}
        self._notifications: list[NotificationConfig] = []
        self._running = False
        self._task: asyncio.Task | None = None
        self._alert_counter = 0

    def add_rule(self, rule: AlertRule) -> None:
        """Add alert rule.

        Args:
            rule: Alert rule to add
        """
        self._rules[rule.name] = rule
        logger.info(f"Added alert rule: {rule.name}")

    def remove_rule(self, name: str) -> bool:
        """Remove alert rule.

        Args:
            name: Rule name

        Returns:
            True if removed
        """
        if name in self._rules:
            del self._rules[name]
            self._pending_since.pop(name, None)
            logger.info(f"Removed alert rule: {name}")
            return True
        return False

    def add_notification(self, config: NotificationConfig) -> None:
        """Add notification channel.

        Args:
            config: Notification configuration
        """
        self._notifications.append(config)
        logger.info(f"Added notification channel: {config.name} ({config.channel.value})")

    def _generate_alert_id(self) -> str:
        """Generate unique alert ID."""
        self._alert_counter += 1
        return f"alert-{self._alert_counter:06d}"

    async def evaluate_rules(self) -> list[Alert]:
        """Evaluate all rules and generate alerts.

        Returns:
            List of new or changed alerts
        """
        changed_alerts = []
        now = datetime.now()

        for name, rule in self._rules.items():
            if not rule.enabled:
                continue

            is_firing, value = rule.evaluate()

            if is_firing:
                if name not in self._pending_since:
                    self._pending_since[name] = now
                    continue

                pending_duration = now - self._pending_since[name]
                if pending_duration >= rule.for_duration:
                    # Create or update alert
                    if name not in self._alerts or self._alerts[name].state == AlertState.RESOLVED:
                        alert = Alert(
                            id=self._generate_alert_id(),
                            rule_name=name,
                            severity=rule.severity,
                            state=AlertState.FIRING,
                            message=f"Alert {name} is firing (value: {value})",
                            labels=rule.labels.copy(),
                            annotations=rule.annotations.copy(),
                            value=value,
                        )
                        self._alerts[name] = alert
                        changed_alerts.append(alert)
                        logger.warning(f"Alert firing: {name} (value: {value})")
            else:
                # Clear pending state
                self._pending_since.pop(name, None)

                # Resolve existing alert
                if name in self._alerts and self._alerts[name].state == AlertState.FIRING:
                    self._alerts[name].state = AlertState.RESOLVED
                    self._alerts[name].ended_at = now
                    changed_alerts.append(self._alerts[name])
                    logger.info(f"Alert resolved: {name}")

        return changed_alerts

    async def send_notifications(self, alerts: list[Alert]) -> None:
        """Send notifications for alerts.

        Args:
            alerts: Alerts to notify about
        """
        for alert in alerts:
            for config in self._notifications:
                if not config.enabled:
                    continue

                if alert.severity not in config.severity_filter:
                    continue

                try:
                    if config.channel == NotificationChannel.SLACK:
                        await self._send_slack(config, alert)
                    elif config.channel == NotificationChannel.WEBHOOK:
                        await self._send_webhook(config, alert)
                    elif config.channel == NotificationChannel.PAGERDUTY:
                        await self._send_pagerduty(config, alert)
                except Exception as e:
                    logger.error(f"Failed to send notification via {config.name}: {e}")

    async def _send_slack(self, config: NotificationConfig, alert: Alert) -> None:
        """Send Slack notification."""
        if not config.url:
            return

        color = {
            AlertSeverity.CRITICAL: "#FF0000",
            AlertSeverity.WARNING: "#FFA500",
            AlertSeverity.INFO: "#0000FF",
        }.get(alert.severity, "#808080")

        payload = {
            "attachments": [
                {
                    "color": color,
                    "title": f"[{alert.severity.value.upper()}] {alert.rule_name}",
                    "text": alert.message,
                    "fields": [
                        {"title": "State", "value": alert.state.value, "short": True},
                        {
                            "title": "Started",
                            "value": alert.started_at.strftime("%Y-%m-%d %H:%M:%S"),
                            "short": True,
                        },
                    ],
                    "footer": "C4 Alerting",
                    "ts": int(alert.started_at.timestamp()),
                }
            ]
        }

        await self._http_post(config.url, payload)
        logger.debug(f"Sent Slack notification for {alert.rule_name}")

    async def _send_webhook(self, config: NotificationConfig, alert: Alert) -> None:
        """Send generic webhook notification."""
        if not config.url:
            return

        payload = alert.to_dict()
        headers = {}
        if config.token:
            headers["Authorization"] = f"Bearer {config.token}"

        await self._http_post(config.url, payload, headers)
        logger.debug(f"Sent webhook notification for {alert.rule_name}")

    async def _send_pagerduty(self, config: NotificationConfig, alert: Alert) -> None:
        """Send PagerDuty notification."""
        if not config.token:
            return

        event_type = "trigger" if alert.state == AlertState.FIRING else "resolve"

        payload = {
            "routing_key": config.token,
            "event_action": event_type,
            "dedup_key": alert.id,
            "payload": {
                "summary": f"[{alert.severity.value}] {alert.rule_name}: {alert.message}",
                "severity": alert.severity.value,
                "source": "c4-monitoring",
                "timestamp": alert.started_at.isoformat(),
                "custom_details": alert.labels,
            },
        }

        url = "https://events.pagerduty.com/v2/enqueue"
        await self._http_post(url, payload)
        logger.debug(f"Sent PagerDuty notification for {alert.rule_name}")

    async def _http_post(
        self,
        url: str,
        payload: dict,
        headers: dict[str, str] | None = None,
    ) -> None:
        """Make HTTP POST request."""
        headers = headers or {}
        headers["Content-Type"] = "application/json"

        data = json.dumps(payload).encode("utf-8")
        request = Request(url, data=data, headers=headers, method="POST")

        def _do_request() -> None:
            try:
                with urlopen(request, timeout=10) as response:
                    response.read()
            except URLError as e:
                logger.error(f"HTTP request failed: {e}")

        await asyncio.get_event_loop().run_in_executor(None, _do_request)

    async def start(self, interval_seconds: int = 60) -> None:
        """Start alert evaluation loop.

        Args:
            interval_seconds: Evaluation interval
        """
        if self._running:
            return

        self._running = True
        logger.info(f"Starting alert manager (interval: {interval_seconds}s)")

        async def _loop() -> None:
            while self._running:
                try:
                    alerts = await self.evaluate_rules()
                    if alerts:
                        await self.send_notifications(alerts)
                except Exception as e:
                    logger.error(f"Alert evaluation error: {e}")

                await asyncio.sleep(interval_seconds)

        self._task = asyncio.create_task(_loop())

    async def stop(self) -> None:
        """Stop alert evaluation loop."""
        self._running = False
        if self._task:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass
            self._task = None
        logger.info("Alert manager stopped")

    def get_active_alerts(self) -> list[Alert]:
        """Get all active (firing) alerts.

        Returns:
            List of firing alerts
        """
        return [a for a in self._alerts.values() if a.state == AlertState.FIRING]

    def get_all_alerts(self) -> list[Alert]:
        """Get all alerts.

        Returns:
            List of all alerts
        """
        return list(self._alerts.values())

    def get_rules(self) -> list[dict[str, Any]]:
        """Get all rules.

        Returns:
            List of rule information
        """
        return [
            {
                "name": rule.name,
                "threshold": rule.threshold,
                "operator": rule.operator,
                "severity": rule.severity.value,
                "for_duration_seconds": rule.for_duration.total_seconds(),
                "enabled": rule.enabled,
                "labels": rule.labels,
            }
            for rule in self._rules.values()
        ]

    def silence_alert(self, rule_name: str, duration: timedelta) -> bool:
        """Silence an alert temporarily.

        Args:
            rule_name: Rule to silence
            duration: Silence duration

        Returns:
            True if silenced
        """
        if rule_name in self._rules:
            self._rules[rule_name].enabled = False
            # Schedule re-enable
            logger.info(f"Silenced alert {rule_name} for {duration}")
            return True
        return False


# Global alert manager instance
_alert_manager: AlertManager | None = None


def get_alert_manager() -> AlertManager:
    """Get global alert manager instance."""
    global _alert_manager
    if _alert_manager is None:
        _alert_manager = AlertManager()
    return _alert_manager


# Pre-defined alert rules
def create_default_rules(metrics_registry: Any) -> list[AlertRule]:
    """Create default C4 alert rules.

    Args:
        metrics_registry: Metrics registry for expressions

    Returns:
        List of default rules
    """
    rules = []

    # High error rate
    def error_rate() -> float | None:
        total = 0
        errors = 0
        counter = metrics_registry._counters.get("c4_http_requests_total")
        if counter:
            for mv in counter.get_all():
                total += mv.value
                if mv.labels.get("status", "").startswith("5"):
                    errors += mv.value
        return (errors / total * 100) if total > 0 else None

    rules.append(
        AlertRule(
            name="high_error_rate",
            expression=error_rate,
            threshold=5.0,
            operator="gt",
            severity=AlertSeverity.CRITICAL,
            for_duration=timedelta(minutes=5),
            labels={"team": "platform"},
            annotations={"description": "Error rate exceeds 5%"},
        )
    )

    # High latency
    def p99_latency() -> float | None:
        histogram = metrics_registry._histograms.get("c4_http_request_duration_seconds")
        if histogram:
            all_data = histogram.get_all()
            if all_data:
                all_values = []
                for hdata in all_data:
                    labels_key = tuple(sorted(hdata["labels"].items()))
                    all_values.extend(histogram._values.get(labels_key, []))
                if all_values:
                    sorted_values = sorted(all_values)
                    idx = int(len(sorted_values) * 0.99)
                    return sorted_values[min(idx, len(sorted_values) - 1)]
        return None

    rules.append(
        AlertRule(
            name="high_latency",
            expression=p99_latency,
            threshold=2.0,
            operator="gt",
            severity=AlertSeverity.WARNING,
            for_duration=timedelta(minutes=10),
            labels={"team": "platform"},
            annotations={"description": "P99 latency exceeds 2 seconds"},
        )
    )

    return rules
