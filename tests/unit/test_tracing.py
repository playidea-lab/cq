"""Tests for tracing initialization."""

import os
from unittest.mock import patch

from c4.monitoring.tracing import get_tracer, setup_tracing


def test_setup_tracing():
    """Test that tracing can be setup without errors."""
    with patch("opentelemetry.trace.set_tracer_provider"):
        setup_tracing(service_name="test-service")
        tracer = get_tracer()
        assert tracer is not None

def test_tracing_with_otlp():
    """Test tracing setup with OTLP endpoint."""
    with patch.dict(os.environ, {"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4318"}):
        with patch("c4.monitoring.tracing.OTLPSpanExporter") as mock_exporter:
            setup_tracing(service_name="test-otlp")
            # Should not raise exception
            assert mock_exporter.called
