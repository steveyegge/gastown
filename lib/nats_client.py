"""
NATS Client wrapper for Gas Town CLI.

Provides async NATS messaging capabilities for event publishing
and subscription within the Gas Town ecosystem.

Environment Variables:
    GT_NATS_URL: NATS server URL (default: nats://localhost:4222)
    GT_EMIT_EVENTS: Enable event emission (default: false)

Usage:
    from lib.nats_client import NatsClient, emit_event

    # Direct usage
    client = NatsClient()
    await client.connect()
    await client.publish("rig.blaze.polecat.spawned", {"name": "jasper"})
    await client.close()

    # Helper function (respects GT_EMIT_EVENTS)
    await emit_event("rig.blaze.polecat.done", {"name": "jasper", "status": "success"})
"""

import asyncio
import json
import os
from typing import Any, Callable, List, Optional

try:
    import nats
    from nats.aio.client import Client as NATS
    from nats.errors import ConnectionClosedError, NoServersError, TimeoutError
    NATS_AVAILABLE = True
except ImportError:
    NATS_AVAILABLE = False
    NATS = None


# Module-level client singleton
_nats_client: Optional["NatsClient"] = None


class NatsClient:
    """
    Async NATS client wrapper with connection management.

    Handles reconnection, error handling, and graceful shutdown.
    """

    def __init__(self, servers: Optional[List[str]] = None):
        """
        Initialize NatsClient.

        Args:
            servers: List of NATS server URLs. Defaults to GT_NATS_URL env var
                    or nats://localhost:4222.
        """
        if not NATS_AVAILABLE:
            raise RuntimeError(
                "nats-py is not installed. Install with: pip install nats-py"
            )

        default_url = os.environ.get("GT_NATS_URL", "nats://localhost:4222")
        self._servers = servers or [default_url]
        self._nc: Optional[NATS] = None
        self._connected = False
        self._subscriptions: dict[str, Any] = {}

    @property
    def is_connected(self) -> bool:
        """Check if client is connected to NATS."""
        return self._connected and self._nc is not None and self._nc.is_connected

    async def connect(self, servers: Optional[List[str]] = None) -> None:
        """
        Connect to NATS server(s).

        Args:
            servers: Optional list of server URLs to override constructor value.

        Raises:
            ConnectionError: If unable to connect to any server.
        """
        if self.is_connected:
            return

        target_servers = servers or self._servers
        self._nc = NATS()

        try:
            await self._nc.connect(
                servers=target_servers,
                reconnect_time_wait=2,
                max_reconnect_attempts=5,
                error_cb=self._error_callback,
                disconnected_cb=self._disconnected_callback,
                reconnected_cb=self._reconnected_callback,
                closed_cb=self._closed_callback,
            )
            self._connected = True
        except (NoServersError, TimeoutError, OSError) as e:
            self._connected = False
            raise ConnectionError(f"Failed to connect to NATS: {e}") from e

    async def publish(self, subject: str, payload: dict) -> None:
        """
        Publish a message to a NATS subject.

        Args:
            subject: The NATS subject to publish to.
            payload: Dictionary payload to publish (will be JSON encoded).

        Raises:
            ConnectionError: If not connected to NATS.
        """
        if not self.is_connected:
            raise ConnectionError("Not connected to NATS. Call connect() first.")

        data = json.dumps(payload).encode("utf-8")
        await self._nc.publish(subject, data)
        await self._nc.flush()

    async def subscribe(
        self,
        subject: str,
        callback: Callable[[str, dict], Any],
        queue: Optional[str] = None,
    ) -> str:
        """
        Subscribe to a NATS subject.

        Args:
            subject: The NATS subject pattern to subscribe to.
            callback: Async callback function(subject, payload) for messages.
            queue: Optional queue group for load balancing.

        Returns:
            Subscription ID for later unsubscription.

        Raises:
            ConnectionError: If not connected to NATS.
        """
        if not self.is_connected:
            raise ConnectionError("Not connected to NATS. Call connect() first.")

        async def message_handler(msg):
            try:
                payload = json.loads(msg.data.decode("utf-8"))
            except json.JSONDecodeError:
                payload = {"raw": msg.data.decode("utf-8", errors="replace")}

            if asyncio.iscoroutinefunction(callback):
                await callback(msg.subject, payload)
            else:
                callback(msg.subject, payload)

        if queue:
            sub = await self._nc.subscribe(subject, queue=queue, cb=message_handler)
        else:
            sub = await self._nc.subscribe(subject, cb=message_handler)

        sub_id = str(sub.sid)
        self._subscriptions[sub_id] = sub
        return sub_id

    async def unsubscribe(self, sub_id: str) -> None:
        """
        Unsubscribe from a subscription.

        Args:
            sub_id: The subscription ID returned from subscribe().
        """
        if sub_id in self._subscriptions:
            await self._subscriptions[sub_id].unsubscribe()
            del self._subscriptions[sub_id]

    async def close(self) -> None:
        """Close the NATS connection gracefully."""
        if self._nc is not None:
            # Unsubscribe all
            for sub_id in list(self._subscriptions.keys()):
                await self.unsubscribe(sub_id)

            try:
                await self._nc.drain()
            except Exception:
                pass

            self._connected = False
            self._nc = None

    async def _error_callback(self, e: Exception) -> None:
        """Handle NATS errors."""
        # Log error but don't raise - let reconnection logic handle it
        pass

    async def _disconnected_callback(self) -> None:
        """Handle disconnection events."""
        self._connected = False

    async def _reconnected_callback(self) -> None:
        """Handle reconnection events."""
        self._connected = True

    async def _closed_callback(self) -> None:
        """Handle connection closed events."""
        self._connected = False


def get_nats_client() -> Optional[NatsClient]:
    """
    Get the module-level NatsClient singleton.

    Returns:
        NatsClient instance if NATS is enabled and available, None otherwise.
    """
    global _nats_client

    # Check if events are enabled
    emit_enabled = os.environ.get("GT_EMIT_EVENTS", "false").lower() in ("true", "1", "yes")
    if not emit_enabled:
        return None

    if not NATS_AVAILABLE:
        return None

    if _nats_client is None:
        _nats_client = NatsClient()

    return _nats_client


async def emit_event(subject: str, payload: dict) -> bool:
    """
    Publish an event to NATS. No-op if NATS not configured.

    This is a convenience function that:
    - Checks GT_EMIT_EVENTS environment variable
    - Manages connection automatically
    - Silently fails if NATS is unavailable

    Args:
        subject: The NATS subject to publish to.
        payload: Dictionary payload to publish.

    Returns:
        True if event was published, False if skipped or failed.

    Example:
        await emit_event("rig.blaze.polecat.spawned", {
            "name": "jasper",
            "rig": "blaze",
            "timestamp": "2026-02-03T12:00:00Z"
        })
    """
    client = get_nats_client()
    if client is None:
        return False

    try:
        if not client.is_connected:
            await client.connect()

        await client.publish(subject, payload)
        return True
    except Exception:
        # Silently fail - event emission should not break workflows
        return False


async def close_nats_client() -> None:
    """Close the module-level NatsClient singleton if it exists."""
    global _nats_client

    if _nats_client is not None:
        await _nats_client.close()
        _nats_client = None
