"""OpenTelemetry tracing for C4.

Provides distributed tracing for workers, daemon, and API.
"""

import logging
import os

from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import SERVICE_NAME, SERVICE_VERSION, Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor, ConsoleSpanExporter

logger = logging.getLogger(__name__)

# Tracer instance
_tracer = None

def setup_tracing(service_name: str = "c4", version: str = "0.1.0") -> None:
    """Initialize OpenTelemetry tracing.

    Args:
        service_name: Name of the service being traced
        version: Service version
    """
    global _tracer

    # Create resource with service name
    resource = Resource.create({
        SERVICE_NAME: service_name,
        SERVICE_VERSION: version,
        "deployment.environment": os.getenv("C4_ENV", "development"),
    })

    # Initialize tracer provider
    provider = TracerProvider(resource=resource)

    # Add Console exporter for development
    if os.getenv("C4_DEBUG_TRACING") == "true":
        console_exporter = ConsoleSpanExporter()
        provider.add_span_processor(BatchSpanProcessor(console_exporter))

    # Add OTLP exporter if endpoint is provided
    otlp_endpoint = os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
    if otlp_endpoint:
        try:
            otlp_exporter = OTLPSpanExporter(endpoint=otlp_endpoint)
            provider.add_span_processor(BatchSpanProcessor(otlp_exporter))
            logger.info(f"OTLP tracing enabled for {service_name} at {otlp_endpoint}")
        except Exception as e:
            logger.warning(f"Failed to initialize OTLP exporter: {e}")

    # Set as global tracer provider
    trace.set_tracer_provider(provider)
    _tracer = trace.get_tracer("c4")

def instrument_app(app):
    """Instrument a FastAPI application with OpenTelemetry.

    Args:
        app: FastAPI application instance
    """
    from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor

    FastAPIInstrumentor.instrument_app(app)
    logger.info("FastAPI application instrumented with OpenTelemetry")

def get_tracer():
    """Get the C4 tracer.

    Returns:
        Tracer instance
    """
    global _tracer
    if _tracer is None:
        _tracer = trace.get_tracer("c4")
    return _tracer
