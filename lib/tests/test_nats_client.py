"""
Unit tests for NatsClient wrapper.

These tests use mocking to test the client logic without requiring
a running NATS server. For integration tests with a real server,
set GT_NATS_URL and run with pytest -m integration.
"""

import asyncio
import json
import os
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

# Import with mocked nats module for unit tests
with patch.dict("sys.modules", {"nats": MagicMock(), "nats.aio.client": MagicMock(), "nats.errors": MagicMock()}):
    from nats_client import (
        NatsClient,
        emit_event,
        get_nats_client,
        close_nats_client,
        NATS_AVAILABLE,
    )


class TestNatsClient:
    """Tests for NatsClient class."""

    @pytest.fixture
    def mock_nats(self):
        """Create a mock NATS client."""
        mock_nc = AsyncMock()
        mock_nc.is_connected = True
        mock_nc.connect = AsyncMock()
        mock_nc.publish = AsyncMock()
        mock_nc.subscribe = AsyncMock()
        mock_nc.flush = AsyncMock()
        mock_nc.drain = AsyncMock()
        return mock_nc

    @pytest.fixture
    def client(self, mock_nats):
        """Create a NatsClient with mocked internals."""
        with patch("nats_client.NATS_AVAILABLE", True):
            with patch("nats_client.NATS") as mock_nats_class:
                mock_nats_class.return_value = mock_nats
                client = NatsClient()
                client._nc = mock_nats
                client._connected = True
                return client

    def test_default_server_url(self):
        """Test that default server URL is used when no env var set."""
        with patch.dict(os.environ, {}, clear=True):
            with patch("nats_client.NATS_AVAILABLE", True):
                with patch("nats_client.NATS"):
                    client = NatsClient()
                    assert client._servers == ["nats://localhost:4222"]

    def test_env_var_server_url(self):
        """Test that GT_NATS_URL env var is respected."""
        with patch.dict(os.environ, {"GT_NATS_URL": "nats://custom:4222"}, clear=True):
            with patch("nats_client.NATS_AVAILABLE", True):
                with patch("nats_client.NATS"):
                    client = NatsClient()
                    assert client._servers == ["nats://custom:4222"]

    def test_custom_servers(self):
        """Test that custom server list is used."""
        with patch("nats_client.NATS_AVAILABLE", True):
            with patch("nats_client.NATS"):
                servers = ["nats://server1:4222", "nats://server2:4222"]
                client = NatsClient(servers=servers)
                assert client._servers == servers

    def test_is_connected_false_initially(self):
        """Test that client is not connected initially."""
        with patch("nats_client.NATS_AVAILABLE", True):
            with patch("nats_client.NATS"):
                client = NatsClient()
                assert not client.is_connected

    def test_is_connected_true_after_connect(self, client):
        """Test that is_connected returns True after connecting."""
        assert client.is_connected

    @pytest.mark.asyncio
    async def test_publish_sends_json(self, client, mock_nats):
        """Test that publish sends JSON-encoded payload."""
        payload = {"name": "test", "value": 123}
        await client.publish("test.subject", payload)

        mock_nats.publish.assert_called_once()
        call_args = mock_nats.publish.call_args
        assert call_args[0][0] == "test.subject"
        assert json.loads(call_args[0][1].decode()) == payload

    @pytest.mark.asyncio
    async def test_publish_flushes(self, client, mock_nats):
        """Test that publish flushes after sending."""
        await client.publish("test.subject", {"key": "value"})
        mock_nats.flush.assert_called_once()

    @pytest.mark.asyncio
    async def test_publish_raises_when_not_connected(self):
        """Test that publish raises ConnectionError when not connected."""
        with patch("nats_client.NATS_AVAILABLE", True):
            with patch("nats_client.NATS"):
                client = NatsClient()
                with pytest.raises(ConnectionError, match="Not connected"):
                    await client.publish("test.subject", {})

    @pytest.mark.asyncio
    async def test_subscribe_creates_subscription(self, client, mock_nats):
        """Test that subscribe creates a subscription."""
        mock_sub = MagicMock()
        mock_sub.sid = 123
        mock_nats.subscribe.return_value = mock_sub

        callback = AsyncMock()
        sub_id = await client.subscribe("test.>", callback)

        assert sub_id == "123"
        mock_nats.subscribe.assert_called_once()

    @pytest.mark.asyncio
    async def test_subscribe_with_queue(self, client, mock_nats):
        """Test that subscribe passes queue parameter."""
        mock_sub = MagicMock()
        mock_sub.sid = 456
        mock_nats.subscribe.return_value = mock_sub

        callback = AsyncMock()
        await client.subscribe("test.>", callback, queue="workers")

        call_kwargs = mock_nats.subscribe.call_args[1]
        assert call_kwargs["queue"] == "workers"

    @pytest.mark.asyncio
    async def test_unsubscribe_removes_subscription(self, client, mock_nats):
        """Test that unsubscribe removes and unsubscribes."""
        mock_sub = AsyncMock()
        mock_sub.sid = 789
        mock_sub.unsubscribe = AsyncMock()
        mock_nats.subscribe.return_value = mock_sub

        callback = AsyncMock()
        sub_id = await client.subscribe("test.>", callback)
        await client.unsubscribe(sub_id)

        mock_sub.unsubscribe.assert_called_once()
        assert sub_id not in client._subscriptions

    @pytest.mark.asyncio
    async def test_close_drains_connection(self, client, mock_nats):
        """Test that close drains the connection."""
        await client.close()
        mock_nats.drain.assert_called_once()

    @pytest.mark.asyncio
    async def test_close_clears_state(self, client, mock_nats):
        """Test that close clears connection state."""
        await client.close()
        assert not client._connected
        assert client._nc is None


class TestEmitEvent:
    """Tests for emit_event helper function."""

    @pytest.fixture(autouse=True)
    def reset_client(self):
        """Reset the module-level client before each test."""
        import nats_client
        nats_client._nats_client = None
        yield
        nats_client._nats_client = None

    @pytest.mark.asyncio
    async def test_emit_event_noop_when_disabled(self):
        """Test that emit_event is a no-op when GT_EMIT_EVENTS is not set."""
        with patch.dict(os.environ, {"GT_EMIT_EVENTS": "false"}, clear=True):
            result = await emit_event("test.subject", {"key": "value"})
            assert result is False

    @pytest.mark.asyncio
    async def test_emit_event_enabled_when_true(self):
        """Test that emit_event works when GT_EMIT_EVENTS is true."""
        mock_client = AsyncMock()
        mock_client.is_connected = True
        mock_client.publish = AsyncMock()

        with patch.dict(os.environ, {"GT_EMIT_EVENTS": "true"}):
            with patch("nats_client.get_nats_client", return_value=mock_client):
                result = await emit_event("test.subject", {"key": "value"})
                assert result is True
                mock_client.publish.assert_called_once_with(
                    "test.subject", {"key": "value"}
                )

    @pytest.mark.asyncio
    async def test_emit_event_connects_if_needed(self):
        """Test that emit_event connects if not already connected."""
        mock_client = AsyncMock()
        mock_client.is_connected = False
        mock_client.connect = AsyncMock()
        mock_client.publish = AsyncMock()

        # After connect, is_connected should return True
        async def connect_side_effect():
            mock_client.is_connected = True

        mock_client.connect.side_effect = connect_side_effect

        with patch.dict(os.environ, {"GT_EMIT_EVENTS": "true"}):
            with patch("nats_client.get_nats_client", return_value=mock_client):
                await emit_event("test.subject", {})
                mock_client.connect.assert_called_once()

    @pytest.mark.asyncio
    async def test_emit_event_fails_silently(self):
        """Test that emit_event fails silently on errors."""
        mock_client = AsyncMock()
        mock_client.is_connected = True
        mock_client.publish = AsyncMock(side_effect=Exception("Connection lost"))

        with patch.dict(os.environ, {"GT_EMIT_EVENTS": "true"}):
            with patch("nats_client.get_nats_client", return_value=mock_client):
                result = await emit_event("test.subject", {})
                assert result is False


class TestGetNatsClient:
    """Tests for get_nats_client helper function."""

    @pytest.fixture(autouse=True)
    def reset_client(self):
        """Reset the module-level client before each test."""
        import nats_client
        nats_client._nats_client = None
        yield
        nats_client._nats_client = None

    def test_returns_none_when_disabled(self):
        """Test that get_nats_client returns None when events disabled."""
        with patch.dict(os.environ, {"GT_EMIT_EVENTS": "false"}):
            assert get_nats_client() is None

    def test_returns_none_when_nats_unavailable(self):
        """Test that get_nats_client returns None when nats-py not installed."""
        with patch.dict(os.environ, {"GT_EMIT_EVENTS": "true"}):
            with patch("nats_client.NATS_AVAILABLE", False):
                assert get_nats_client() is None

    def test_returns_singleton(self):
        """Test that get_nats_client returns the same instance."""
        with patch.dict(os.environ, {"GT_EMIT_EVENTS": "true"}):
            with patch("nats_client.NATS_AVAILABLE", True):
                with patch("nats_client.NatsClient") as mock_class:
                    mock_class.return_value = MagicMock()
                    client1 = get_nats_client()
                    client2 = get_nats_client()
                    assert client1 is client2
                    mock_class.assert_called_once()


class TestCloseNatsClient:
    """Tests for close_nats_client helper function."""

    @pytest.fixture(autouse=True)
    def reset_client(self):
        """Reset the module-level client before each test."""
        import nats_client
        nats_client._nats_client = None
        yield
        nats_client._nats_client = None

    @pytest.mark.asyncio
    async def test_closes_existing_client(self):
        """Test that close_nats_client closes the singleton."""
        import nats_client

        mock_client = AsyncMock()
        nats_client._nats_client = mock_client

        await close_nats_client()

        mock_client.close.assert_called_once()
        assert nats_client._nats_client is None

    @pytest.mark.asyncio
    async def test_noop_when_no_client(self):
        """Test that close_nats_client is safe when no client exists."""
        await close_nats_client()  # Should not raise
