#!/usr/bin/env python3
"""
Herald Agent — Announces Wasteland events.

Polls the DoltHub SQL API for new rows in wanted, completions, and rigs
tables, diffs against local state, and prints announcements to stdout.
Optionally posts to a Discord webhook.

Usage:
    python3 herald.py                      # run once
    python3 herald.py --loop               # poll continuously
    python3 herald.py --loop --interval 60 # poll every 60s
    python3 herald.py --discord-webhook URL # post to Discord
"""

import argparse
import json
import os
import sys
import time
import urllib.request
import urllib.parse
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, List, Optional, Set

DOLTHUB_API = "https://www.dolthub.com/api/v1alpha1/hop/wl-commons"
STATE_FILE = Path(__file__).parent / "state.json"

# Queries to detect new rows in each table.
QUERIES = {
    "wanted": "SELECT id, title, project, type, priority, posted_by, status, created_at FROM wanted ORDER BY created_at DESC LIMIT 50",
    "completions": "SELECT id, wanted_id, completed_by, evidence, validated_by, completed_at, validated_at FROM completions ORDER BY completed_at DESC LIMIT 50",
    "rigs": "SELECT handle, display_name, trust_level, rig_type, registered_at FROM rigs ORDER BY registered_at DESC LIMIT 50",
}


def query_dolthub(sql: str) -> List[dict]:
    """Execute a SQL query against the DoltHub API and return rows."""
    url = f"{DOLTHUB_API}?q={urllib.parse.quote(sql)}"
    req = urllib.request.Request(url)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            data = json.loads(resp.read().decode())
    except Exception as e:
        print(f"[herald] ERROR querying DoltHub: {e}", file=sys.stderr)
        return []
    if data.get("query_execution_status") != "Success":
        msg = data.get("query_execution_message", "unknown error")
        print(f"[herald] Query failed: {msg}", file=sys.stderr)
        return []
    return data.get("rows", [])


def load_state() -> dict:
    """Load previously seen IDs from state file."""
    if STATE_FILE.exists():
        try:
            return json.loads(STATE_FILE.read_text())
        except (json.JSONDecodeError, OSError):
            pass
    return {"wanted": [], "completions": [], "rigs": []}


def save_state(state: dict) -> None:
    """Persist seen IDs to state file."""
    STATE_FILE.write_text(json.dumps(state, indent=2) + "\n")


def id_for_table(table: str, row: dict) -> str:
    """Return the primary key value for a row in the given table."""
    if table == "rigs":
        return row["handle"]
    return row["id"]


def format_wanted(row: dict) -> str:
    """Format an announcement for a new wanted item."""
    priority = row.get("priority", "?")
    title = row.get("title", "Untitled")
    project = row.get("project", "unknown")
    wtype = row.get("type", "task")
    posted_by = row.get("posted_by", "unknown")
    status = row.get("status", "open")
    return (
        f"[WANTED] New {wtype} posted by @{posted_by} in [{project}] (P{priority}, {status})\n"
        f"         {row.get('id', '?')}: {title}"
    )


def format_completion(row: dict) -> str:
    """Format an announcement for a new completion."""
    completed_by = row.get("completed_by", "unknown")
    wanted_id = row.get("wanted_id", "?")
    evidence = row.get("evidence", "")
    validated = row.get("validated_at")
    status = "validated" if validated else "pending review"
    lines = [
        f"[COMPLETION] @{completed_by} completed {wanted_id} ({status})"
    ]
    if evidence:
        lines.append(f"             Evidence: {evidence}")
    if validated:
        validator = row.get("validated_by", "?")
        lines.append(f"             Validated by @{validator}")
    return "\n".join(lines)


def format_rig(row: dict) -> str:
    """Format an announcement for a new rig registration."""
    handle = row.get("handle", "unknown")
    display = row.get("display_name", handle)
    trust = row.get("trust_level", "0")
    rig_type = row.get("rig_type", "human")
    return f"[NEW RIG] @{handle} ({display}) joined the Wasteland — {rig_type}, trust T{trust}"


FORMATTERS = {
    "wanted": format_wanted,
    "completions": format_completion,
    "rigs": format_rig,
}


def post_discord(webhook_url: str, message: str) -> None:
    """Post a message to a Discord webhook."""
    payload = json.dumps({"content": message}).encode()
    req = urllib.request.Request(
        webhook_url,
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            if resp.status not in (200, 204):
                print(f"[herald] Discord webhook returned {resp.status}", file=sys.stderr)
    except Exception as e:
        print(f"[herald] Discord webhook error: {e}", file=sys.stderr)


def poll(discord_webhook: Optional[str] = None) -> int:
    """Poll all tables, announce new rows, return count of announcements."""
    state = load_state()
    total = 0

    for table, sql in QUERIES.items():
        rows = query_dolthub(sql)
        seen = set(state.get(table, []))
        formatter = FORMATTERS[table]
        new_rows = []

        for row in rows:
            row_id = id_for_table(table, row)
            if row_id not in seen:
                new_rows.append(row)
                seen.add(row_id)

        # Announce oldest first.
        for row in reversed(new_rows):
            msg = formatter(row)
            timestamp = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")
            announcement = f"[{timestamp}] {msg}"
            print(announcement)
            print()
            if discord_webhook:
                post_discord(discord_webhook, msg)
            total += 1

        state[table] = list(seen)

    save_state(state)
    return total


def seed_state() -> None:
    """Initialize state with current rows so the first real poll only shows new items."""
    state = load_state()
    has_data = any(state.get(t) for t in QUERIES)
    if has_data:
        return
    print("[herald] Seeding initial state...")
    for table, sql in QUERIES.items():
        rows = query_dolthub(sql)
        state[table] = [id_for_table(table, r) for r in rows]
    save_state(state)
    print(f"[herald] Seeded: {', '.join(f'{t}={len(state[t])}' for t in QUERIES)}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Herald Agent — Wasteland event announcer")
    parser.add_argument("--loop", action="store_true", help="Run continuously")
    parser.add_argument("--interval", type=int, default=120, help="Poll interval in seconds (default: 120)")
    parser.add_argument("--discord-webhook", type=str, default=None, help="Discord webhook URL for posting")
    parser.add_argument("--no-seed", action="store_true", help="Skip seeding initial state")
    args = parser.parse_args()

    if not args.no_seed:
        seed_state()

    if args.loop:
        print(f"[herald] Starting poll loop (interval={args.interval}s)")
        while True:
            try:
                count = poll(discord_webhook=args.discord_webhook)
                if count:
                    print(f"[herald] Announced {count} event(s)")
            except KeyboardInterrupt:
                print("\n[herald] Shutting down.")
                break
            except Exception as e:
                print(f"[herald] Error during poll: {e}", file=sys.stderr)
            time.sleep(args.interval)
    else:
        count = poll(discord_webhook=args.discord_webhook)
        if count == 0:
            print("[herald] No new events.")
        else:
            print(f"[herald] Announced {count} event(s).")


if __name__ == "__main__":
    main()
