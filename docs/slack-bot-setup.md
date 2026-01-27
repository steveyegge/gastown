# Gas Town Slack Bot Setup Guide

## Overview

The Gas Town Slack bot (`gtslack`) allows team members to view and resolve pending decisions directly from Slack without SSH access to the town.

## Prerequisites

1. **gtmobile server running** on accessible endpoint (default: `http://localhost:8443`)
2. **Slack workspace** with admin access to create apps
3. **Go 1.21+** for building the bot

## Slack App Configuration

### Step 1: Create Slack App

1. Go to [api.slack.com/apps](https://api.slack.com/apps)
2. Click "Create New App" → "From scratch"
3. Name: "Gas Town Decisions" (or your preference)
4. Select your workspace
5. Click "Create App"

### Step 2: Enable Socket Mode

1. In app settings, go to "Socket Mode"
2. Toggle "Enable Socket Mode" ON
3. Create an App-Level Token:
   - Token Name: "socket-mode"
   - Scopes: `connections:write`
4. Click "Generate"
5. **Save the token** (starts with `xapp-`) - this is your `SLACK_APP_TOKEN`

### Step 3: Configure Bot Token Scopes

1. Go to "OAuth & Permissions"
2. Under "Scopes" → "Bot Token Scopes", add:
   - `chat:write` - Post messages
   - `commands` - Handle slash commands
   - `users:read` - Get user info

### Step 4: Create Slash Command

1. Go to "Slash Commands"
2. Click "Create New Command"
3. Configure:
   - Command: `/decisions`
   - Description: "List pending Gas Town decisions"
   - Usage Hint: (leave empty)
4. Click "Save"

### Step 5: Enable Interactivity

1. Go to "Interactivity & Shortcuts"
2. Toggle "Interactivity" ON
3. Request URL can be left empty (Socket Mode handles this)
4. Click "Save Changes"

### Step 6: Install App to Workspace

1. Go to "Install App"
2. Click "Install to Workspace"
3. Review permissions and click "Allow"
4. **Save the Bot Token** (starts with `xoxb-`) - this is your `SLACK_BOT_TOKEN`

### Step 7: Get Channel ID (for notifications)

1. In Slack, right-click the channel for notifications
2. Click "View channel details"
3. At the bottom, copy the Channel ID (starts with `C`)

## Running the Bot

### Build

```bash
cd /path/to/gastown
go build -o gtslack ./cmd/gtslack
```

### Run with Flags

```bash
./gtslack \
  -bot-token=xoxb-your-token \
  -app-token=xapp-your-token \
  -channel=C0123456789 \
  -rpc=http://localhost:8443
```

### Run with Environment Variables

```bash
export SLACK_BOT_TOKEN=xoxb-your-token
export SLACK_APP_TOKEN=xapp-your-token
export SLACK_CHANNEL=C0123456789
export GTMOBILE_RPC=http://localhost:8443

./gtslack
```

### Verify Connection

You should see:
```
Starting Gas Town Slack bot
RPC endpoint: http://localhost:8443
Notifications channel: C0123456789
Starting SSE listener: http://localhost:8443/events/decisions
Slack: Connecting to Socket Mode...
Slack: Connected to Socket Mode
SSE: Connected to decision events stream
```

## Architecture

```
┌─────────────┐     Socket Mode      ┌─────────────┐
│   Slack     │◄────────────────────►│  gtslack    │
│  Workspace  │     (WebSocket)      │    bot      │
└─────────────┘                      └──────┬──────┘
                                            │
                              ┌─────────────┴─────────────┐
                              │                           │
                              ▼                           ▼
                     ┌─────────────┐             ┌─────────────┐
                     │  gtmobile   │◄────────────│     SSE     │
                     │  RPC API    │   /events   │  Listener   │
                     └─────────────┘             └─────────────┘
```

## Features

| Command/Action | Description |
|---------------|-------------|
| `/decisions` | List all pending decisions with urgency indicators |
| "View" button | Show decision details with resolve options |
| Option buttons | Open modal to enter rationale |
| Modal submit | Resolve decision via RPC |
| SSE notifications | Auto-post new decisions to channel |

## Troubleshooting

### "Invalid token" error
- Verify bot token starts with `xoxb-`
- Verify app token starts with `xapp-`
- Check tokens are not expired

### "Connection error" in SSE
- Verify gtmobile is running: `curl http://localhost:8443/health`
- Check RPC endpoint URL is correct
- Verify firewall allows connection

### Bot not responding to commands
- Verify Socket Mode is enabled in Slack app
- Check bot is installed to workspace
- Verify slash command is configured

### Decisions not appearing
- Check gtmobile has pending decisions: `curl -X POST http://localhost:8443/gastown.v1.DecisionService/ListPending -H "Content-Type: application/json" -d "{}"`
- Verify bot has `chat:write` scope

### Modal not opening
- Check interactivity is enabled in Slack app
- Verify trigger ID is being received (enable debug mode)

## Debug Mode

Run with `-debug` for verbose logging:

```bash
./gtslack -debug -bot-token=... -app-token=...
```
