"""
Gas Town Python libraries.

Provides Python utilities for the Gas Town CLI ecosystem.
"""

from .nats_client import (
    NatsClient,
    emit_event,
    get_nats_client,
    close_nats_client,
    NATS_AVAILABLE,
)

__all__ = [
    "NatsClient",
    "emit_event",
    "get_nats_client",
    "close_nats_client",
    "NATS_AVAILABLE",
]
