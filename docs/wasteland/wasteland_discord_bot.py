#!/usr/bin/env python3
"""Wasteland Discord Bot — posts notifications when wanted board items change."""

import json
import os
import sys
import time
import urllib.parse
import urllib.request

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

DISCORD_WEBHOOK_URL = os.environ.get("DISCORD_WEBHOOK_URL", "")
DOLTHUB_DB = os.environ.get("DOLTHUB_DB", "hop/wl-commons")
POLL_INTERVAL = int(os.environ.get("POLL_INTERVAL", "300"))
STATE_FILE = os.environ.get("STATE_FILE", os.path.expanduser("~/.wasteland-bot.json"))

DOLTHUB_API = f"https://www.dolthub.com/api/v1alpha1/{DOLTHUB_DB}"
DOLTHUB_TABLE_URL = f"https://www.dolthub.com/repositories/{DOLTHUB_DB}/data/main/wanted"

PRIORITY_LABELS = {0: "P0 (critical)", 1: "P1 (high)", 2: "P2 (medium)", 3: "P3 (low)", 4: "P4 (backlog)"}

# Discord embed colours
COLOR_NEW = 0x3498DB       # blue
COLOR_CLAIMED = 0xE67E22   # orange
COLOR_COMPLETED = 0x2ECC71 # green

# ---------------------------------------------------------------------------
# DoltHub query
# ---------------------------------------------------------------------------

def query_dolthub(sql: str) -> list[dict]:
    """Execute a SQL query against DoltHub's public read API."""
    url = f"{DOLTHUB_API}?q={urllib.parse.quote(sql)}"
    req = urllib.request.Request(url, headers={"Accept": "application/json"})
    with urllib.request.urlopen(req, timeout=30) as resp:
        data = json.loads(resp.read())
    if data.get("query_execution_status") != "Success":
        raise RuntimeError(f"DoltHub query failed: {data.get('query_execution_message')}")
    columns = [col["columnName"] for col in data.get("schema", [])]
    return [dict(zip(columns, row)) for row in data.get("rows", [])]

def fetch_wanted_items() -> list[dict]:
    """Fetch all non-archived wanted items."""
    sql = "SELECT * FROM wanted WHERE status IN ('open','claimed','in_review','completed') ORDER BY created_at DESC"
    return query_dolthub(sql)

# ---------------------------------------------------------------------------
# State tracking
# ---------------------------------------------------------------------------

def load_state() -> dict:
    """Load previously seen item states from disk."""
    if os.path.exists(STATE_FILE):
        with open(STATE_FILE) as f:
            return json.load(f)
    return {}

def save_state(state: dict) -> None:
    """Persist item states to disk."""
    with open(STATE_FILE, "w") as f:
        json.dump(state, f, indent=2)

# ---------------------------------------------------------------------------
# Discord notifications
# ---------------------------------------------------------------------------

def post_discord(embeds: list[dict]) -> None:
    """Send embeds to Discord via webhook."""
    if not DISCORD_WEBHOOK_URL:
        # Dry-run mode: print to stdout
        for embed in embeds:
            print(f"[discord] {embed.get('title', '?')}: {embed.get('description', '')}")
        return

    payload = json.dumps({"embeds": embeds}).encode()
    req = urllib.request.Request(
        DISCORD_WEBHOOK_URL,
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=15) as resp:
        if resp.status not in (200, 204):
            print(f"[discord] webhook returned {resp.status}", file=sys.stderr)

def make_embed(item: dict, event: str) -> dict:
    """Build a Discord embed for a wanted item event."""
    item_id = item.get("id", "?")
    title_text = item.get("title", "Untitled")
    project = item.get("project", "unknown")
    priority_raw = item.get("priority", "?")
    priority = PRIORITY_LABELS.get(int(priority_raw), f"P{priority_raw}") if str(priority_raw).isdigit() else str(priority_raw)
    effort = item.get("effort_level", "?")
    item_type = item.get("type", "?")
    claimed_by = item.get("claimed_by") or "unclaimed"
    evidence = item.get("evidence_url") or None

    if event == "new":
        color = COLOR_NEW
        title = f"New Wanted: {title_text}"
    elif event == "claimed":
        color = COLOR_CLAIMED
        title = f"Claimed: {title_text}"
    elif event == "completed":
        color = COLOR_COMPLETED
        title = f"Completed: {title_text}"
    else:
        color = 0x95A5A6
        title = f"Updated: {title_text}"

    fields = [
        {"name": "ID", "value": item_id, "inline": True},
        {"name": "Project", "value": project, "inline": True},
        {"name": "Priority", "value": priority, "inline": True},
        {"name": "Effort", "value": effort, "inline": True},
        {"name": "Type", "value": item_type, "inline": True},
        {"name": "Claimed By", "value": claimed_by, "inline": True},
    ]
    if evidence:
        fields.append({"name": "Evidence", "value": evidence, "inline": False})

    return {
        "title": title,
        "color": color,
        "fields": fields,
        "footer": {"text": f"Wasteland Wanted Board"},
        "url": DOLTHUB_TABLE_URL,
    }

# ---------------------------------------------------------------------------
# Diff and notify
# ---------------------------------------------------------------------------

def detect_changes(old_state: dict, items: list[dict]) -> list[dict]:
    """Compare current items to previous state and return Discord embeds."""
    embeds = []
    for item in items:
        item_id = item.get("id")
        if not item_id:
            continue
        status = item.get("status", "")
        claimed_by = item.get("claimed_by") or ""
        prev = old_state.get(item_id)

        if prev is None:
            # New item we haven't seen before
            embeds.append(make_embed(item, "new"))
        elif prev.get("status") != status:
            if status in ("completed", "in_review"):
                embeds.append(make_embed(item, "completed"))
            elif status == "claimed":
                embeds.append(make_embed(item, "claimed"))
        elif prev.get("claimed_by", "") != claimed_by and claimed_by:
            # Status didn't change but claimed_by did
            embeds.append(make_embed(item, "claimed"))

    return embeds

def build_state(items: list[dict]) -> dict:
    """Build a state dict from current items."""
    state = {}
    for item in items:
        item_id = item.get("id")
        if item_id:
            state[item_id] = {
                "status": item.get("status", ""),
                "claimed_by": item.get("claimed_by") or "",
            }
    return state

# ---------------------------------------------------------------------------
# Main loop
# ---------------------------------------------------------------------------

def poll_once(state: dict) -> dict:
    """Run one poll cycle. Returns the updated state."""
    items = fetch_wanted_items()
    new_state = build_state(items)
    embeds = detect_changes(state, items)

    if embeds:
        # Discord allows max 10 embeds per message
        for i in range(0, len(embeds), 10):
            post_discord(embeds[i : i + 10])
        print(f"[poll] posted {len(embeds)} notification(s)")
    else:
        print("[poll] no changes")

    save_state(new_state)
    return new_state

def main() -> None:
    if not DISCORD_WEBHOOK_URL:
        print("WARNING: DISCORD_WEBHOOK_URL not set — running in dry-run mode (printing to stdout)")

    print(f"Wasteland Discord Bot starting")
    print(f"  DoltHub DB: {DOLTHUB_DB}")
    print(f"  Poll interval: {POLL_INTERVAL}s")
    print(f"  State file: {STATE_FILE}")

    state = load_state()

    # If first run (no state file), seed state without sending notifications
    if not state:
        print("[init] first run — seeding state without notifications")
        items = fetch_wanted_items()
        state = build_state(items)
        save_state(state)
        print(f"[init] tracking {len(state)} items")

    while True:
        try:
            state = poll_once(state)
        except Exception as e:
            print(f"[error] poll failed: {e}", file=sys.stderr)
        time.sleep(POLL_INTERVAL)

if __name__ == "__main__":
    main()
