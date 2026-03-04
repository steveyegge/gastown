# Wasteland Discord Bot

A lightweight bot that posts notifications to a Discord channel when items on the Wasteland wanted board change status.

## What It Does

The bot polls the DoltHub API for the Wasteland `wanted` table and posts Discord embeds when:

- **New item posted** — A new wanted item appears on the board
- **Item claimed** — Someone claims an open item
- **Item completed** — A claimed item is marked complete with evidence

## Setup

### Prerequisites

- Python 3.8+
- A Discord webhook URL (Settings > Integrations > Webhooks in your Discord server)
- Network access to `dolthub.com`

### Installation

```bash
pip install requests
```

### Configuration

Set environment variables:

```bash
# Required
export DISCORD_WEBHOOK_URL="https://discord.com/api/webhooks/YOUR/WEBHOOK"

# Optional
export DOLTHUB_DB="hop/wl-commons"           # Default: hop/wl-commons
export POLL_INTERVAL="300"                     # Seconds between polls (default: 300 = 5 min)
export STATE_FILE="$HOME/.wasteland-bot.json"  # Tracks known items (default: ~/.wasteland-bot.json)
```

### Running

```bash
python3 wasteland_discord_bot.py
```

Or run in the background:

```bash
nohup python3 wasteland_discord_bot.py > /tmp/wasteland-bot.log 2>&1 &
```

### Running with systemd

Create `/etc/systemd/system/wasteland-bot.service`:

```ini
[Unit]
Description=Wasteland Discord Bot
After=network.target

[Service]
Type=simple
User=your-user
Environment=DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/YOUR/WEBHOOK
ExecStart=/usr/bin/python3 /path/to/wasteland_discord_bot.py
Restart=on-failure
RestartSec=30

[Install]
WantedBy=multi-user.target
```

Then:

```bash
sudo systemctl enable wasteland-bot
sudo systemctl start wasteland-bot
```

## Notification Format

Notifications are posted as Discord embeds with color-coded status:

| Event | Color | Title |
|-------|-------|-------|
| New item | Blue (0x3498db) | New Wanted: {title} |
| Claimed | Orange (0xe67e22) | Claimed: {title} |
| Completed | Green (0x2ecc71) | Completed: {title} |

Each embed includes:

- **ID** and **Project**
- **Priority** (P0-P4) and **Effort** level
- **Type** (feature, bug, task, etc.)
- **Claimed by** (when applicable)
- **Evidence URL** (when completed)
- **DoltHub link** to the wanted table

## Architecture

The bot is intentionally simple: a single Python script with no framework dependencies beyond `requests`. It polls DoltHub's read API (no authentication required) and posts to Discord via webhook (no bot token needed).

State is tracked in a local JSON file mapping item IDs to their last-seen status and claimed_by. On each poll cycle, the bot compares current state to previous state and emits notifications for any changes.

## DoltHub API

The bot queries DoltHub's public SQL API:

```
GET https://www.dolthub.com/api/v1alpha1/{owner}/{repo}?q={SQL}
```

No authentication is needed for public databases. The `hop/wl-commons` database contains the `wanted` table with the Wasteland wanted board.
