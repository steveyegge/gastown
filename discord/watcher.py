#!/usr/bin/env python3
"""
Discord watcher for Gas Town.

Listens for @mentions and DMs, forwarding them to mayor/ via gt mail.
Tracks processed message IDs to avoid duplicates.
Handles graceful shutdown on SIGTERM.
"""

import asyncio
import json
import os
import signal
import subprocess
import sys
from pathlib import Path

try:
    import discord
except ImportError:
    print("ERROR: discord.py not installed. Run: pip install discord.py", flush=True)
    sys.exit(1)

# State file for tracking processed message IDs (avoids duplicate mail on restart).
STATE_FILE = os.path.join(os.environ.get("HOME", "/tmp"), ".gt-discord-watcher-state.json")


def load_state() -> set:
    """Load set of already-processed message IDs from state file."""
    try:
        with open(STATE_FILE) as f:
            data = json.load(f)
            return set(data.get("processed_ids", []))
    except (FileNotFoundError, json.JSONDecodeError):
        return set()


def save_state(processed_ids: set) -> None:
    """Persist processed message IDs to state file."""
    # Keep only the most recent 1000 IDs to prevent unbounded growth.
    recent = sorted(processed_ids)[-1000:]
    try:
        with open(STATE_FILE, "w") as f:
            json.dump({"processed_ids": recent}, f)
    except OSError as e:
        print(f"WARNING: failed to save state: {e}", flush=True)


def send_mail(subject: str, body: str) -> None:
    """Send a mail message to mayor/ via gt mail."""
    try:
        result = subprocess.run(
            ["gt", "mail", "send", "mayor/", "-s", subject, "-m", body],
            capture_output=True,
            text=True,
            timeout=30,
        )
        if result.returncode != 0:
            print(f"WARNING: gt mail send failed: {result.stderr.strip()}", flush=True)
        else:
            print(f"Sent mail: {subject}", flush=True)
    except subprocess.TimeoutExpired:
        print("WARNING: gt mail send timed out", flush=True)
    except FileNotFoundError:
        print("ERROR: gt command not found in PATH", flush=True)


class DiscordBot(discord.Client):
    def __init__(self, processed_ids: set):
        intents = discord.Intents.default()
        intents.message_content = True
        intents.dm_messages = True
        super().__init__(intents=intents)
        self.processed_ids = processed_ids

    async def on_ready(self):
        print(f"Discord watcher connected as {self.user}", flush=True)

    async def on_message(self, message: discord.Message):
        # Ignore messages from ourselves.
        if message.author == self.user:
            return

        msg_id = str(message.id)

        # Skip already-processed messages (e.g. replayed on reconnect).
        if msg_id in self.processed_ids:
            return

        is_dm = isinstance(message.channel, discord.DMChannel)
        is_mention = self.user in message.mentions

        if not (is_dm or is_mention):
            return

        # Mark as processed before sending mail to avoid double-send on crash.
        self.processed_ids.add(msg_id)
        save_state(self.processed_ids)

        # Build notification.
        author = str(message.author)
        if is_dm:
            kind = "DM"
            location = "direct message"
        else:
            guild = message.guild.name if message.guild else "unknown server"
            channel = str(message.channel)
            kind = "mention"
            location = f"#{channel} in {guild}"

        subject = f"Discord {kind} from {author}"
        body = (
            f"Discord {kind} received.\n\n"
            f"From: {author}\n"
            f"Location: {location}\n"
            f"Message ID: {msg_id}\n\n"
            f"Content:\n{message.content}"
        )

        print(f"Forwarding Discord {kind} from {author}", flush=True)
        send_mail(subject, body)


def main():
    token = os.environ.get("DISCORD_TOKEN")
    if not token:
        print("ERROR: DISCORD_TOKEN environment variable not set", flush=True)
        sys.exit(1)

    processed_ids = load_state()
    print(f"Discord watcher starting (tracking {len(processed_ids)} processed IDs)", flush=True)

    client = DiscordBot(processed_ids)

    # Handle SIGTERM for graceful shutdown.
    loop = asyncio.new_event_loop()

    def handle_sigterm(*_):
        print("SIGTERM received, shutting down Discord watcher", flush=True)
        loop.call_soon_threadsafe(loop.stop)

    signal.signal(signal.SIGTERM, handle_sigterm)

    try:
        loop.run_until_complete(client.start(token))
    except KeyboardInterrupt:
        pass
    finally:
        loop.run_until_complete(client.close())
        loop.close()
        print("Discord watcher stopped", flush=True)


if __name__ == "__main__":
    main()
