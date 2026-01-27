# Slack Decision Bot: Detailed Design

**Epic:** gt-2gc
**Date:** 2026-01-27
**Status:** Design Complete

## 1. Overview

A Slack chatbot that surfaces Gas Town decision points to humans via Slack, enabling
decision resolution without SSH access to the town.

### Architectural Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Connection Mode | Socket Mode | Simpler, no public URL needed |
| Integration | RPC Client | Leverage existing gtmobile DecisionService |

## 2. System Architecture

```
┌────────────────────────────────────────────────────────────────────┐
│                         Gas Town Server                            │
├────────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐      ┌──────────────┐      ┌──────────────────┐ │
│  │   gtmobile   │      │    Beads     │      │  gt decision     │ │
│  │  RPC Server  │◄────►│   Database   │◄────►│    CLI           │ │
│  │  :8443       │      │ (Decisions)  │      │                  │ │
│  └──────┬───────┘      └──────────────┘      └──────────────────┘ │
│         │                                                          │
│         │ Connect-RPC (HTTP/JSON)                                  │
│         ▼                                                          │
│  ┌──────────────────────────────────────┐                         │
│  │        Slack Decision Bot            │                         │
│  │  ┌────────────┐  ┌────────────────┐  │                         │
│  │  │ RPC Client │  │ Socket Mode    │  │                         │
│  │  │ (gtmobile) │  │ Client (Slack) │  │                         │
│  │  └────────────┘  └───────┬────────┘  │                         │
│  └──────────────────────────┼───────────┘                         │
└─────────────────────────────┼──────────────────────────────────────┘
                              │
                              │ WebSocket (Socket Mode)
                              ▼
                    ┌──────────────────┐
                    │   Slack Cloud    │
                    │  ┌────────────┐  │
                    │  │ Workspace  │  │
                    │  │ #decisions │  │
                    │  └────────────┘  │
                    └──────────────────┘
```

## 3. Slack App Configuration

### Required OAuth Scopes

| Scope | Purpose |
|-------|---------|
| `chat:write` | Post decision messages |
| `commands` | Handle `/decisions` slash command |
| `app_mentions:read` | Respond to @mentions |
| `channels:read` | Access channel info |
| `users:read` | Map Slack users to resolvers |

### Environment Variables

```bash
# Slack credentials (from app dashboard)
SLACK_APP_TOKEN=xapp-1-...     # Socket Mode connection
SLACK_BOT_TOKEN=xoxb-...       # API authentication
SLACK_SIGNING_SECRET=...        # Request verification

# Gas Town connection
GT_RPC_URL=http://localhost:8443
GT_API_KEY=...                  # RPC authentication

# Configuration
GT_SLACK_CHANNEL=#decisions     # Where to post decisions
GT_POLL_INTERVAL=5s             # Decision check frequency
```

### Slack App Dashboard Setup

1. Create new app at https://api.slack.com/apps
2. Enable **Socket Mode** in Settings → Socket Mode
3. Generate **App-Level Token** with `connections:write` scope
4. Add **Bot Token Scopes** in OAuth & Permissions
5. Create **Slash Command** `/decisions`
6. Enable **Interactivity & Shortcuts**
7. Install to workspace

## 4. Bot Capabilities

### 4.1 Slash Commands

#### `/decisions` - List and manage decisions

```
/decisions              → List pending decisions
/decisions list         → Same as above
/decisions show <id>    → Show decision details
/decisions resolve <id> → Open resolution modal
/decisions help         → Show help
```

### 4.2 Notifications

When new decisions arrive, post to configured channel:

```
┌─────────────────────────────────────────────────────────────┐
│ 🟡 New Decision Required                          HIGH      │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ **Which caching strategy for the API layer?**               │
│                                                             │
│ Requested by: gastown/crew/backend • 2 minutes ago          │
│ Blocking: gt-abc123, gt-def456                              │
│                                                             │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ Option 1: Redis                                         │ │
│ │ Distributed, persistent caching with pub/sub support    │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                             │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ Option 2: In-memory ⭐ Recommended                       │ │
│ │ Simple and fast, limited to single process              │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                             │
│ [ Choose Option 1 ]  [ Choose Option 2 ]  [ View Details ] │
│                                                             │
│ ID: hq-abc123                                               │
└─────────────────────────────────────────────────────────────┘
```

### 4.3 Interactive Resolution

When user clicks a "Choose Option N" button:

1. Open modal for rationale input
2. User enters rationale (optional)
3. Bot calls RPC `Resolve(decision_id, chosen_index, rationale)`
4. Bot updates original message to show resolution
5. Bot posts thread reply with resolution details

### 4.4 Resolution Confirmation

After resolution, update original message:

```
┌─────────────────────────────────────────────────────────────┐
│ ✅ Decision Resolved                                        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ **Which caching strategy for the API layer?**               │
│                                                             │
│ ✅ Chosen: In-memory                                        │
│ 📝 Rationale: Simpler for current scale, can add Redis     │
│              later if needed                                │
│ 👤 Resolved by: @steve • just now                           │
│                                                             │
│ ID: hq-abc123                                               │
└─────────────────────────────────────────────────────────────┘
```

## 5. Block Kit Message Structures

### 5.1 Pending Decision Message

```json
{
  "blocks": [
    {
      "type": "header",
      "text": {"type": "plain_text", "text": "🟡 New Decision Required"}
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "*Which caching strategy for the API layer?*"
      },
      "accessory": {
        "type": "button",
        "text": {"type": "plain_text", "text": "HIGH"},
        "style": "danger"
      }
    },
    {
      "type": "context",
      "elements": [
        {"type": "mrkdwn", "text": "Requested by: `gastown/crew/backend` • 2 minutes ago"}
      ]
    },
    {"type": "divider"},
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "*Option 1: Redis*\nDistributed, persistent caching with pub/sub support"
      },
      "accessory": {
        "type": "button",
        "text": {"type": "plain_text", "text": "Choose"},
        "value": "hq-abc123:1",
        "action_id": "resolve_decision_1"
      }
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "*Option 2: In-memory* ⭐ Recommended\nSimple and fast, limited to single process"
      },
      "accessory": {
        "type": "button",
        "text": {"type": "plain_text", "text": "Choose"},
        "value": "hq-abc123:2",
        "action_id": "resolve_decision_2",
        "style": "primary"
      }
    },
    {"type": "divider"},
    {
      "type": "context",
      "elements": [
        {"type": "mrkdwn", "text": "ID: `hq-abc123` | Blocking: `gt-abc123`, `gt-def456`"}
      ]
    }
  ]
}
```

### 5.2 Resolution Modal

```json
{
  "type": "modal",
  "title": {"type": "plain_text", "text": "Resolve Decision"},
  "submit": {"type": "plain_text", "text": "Confirm"},
  "close": {"type": "plain_text", "text": "Cancel"},
  "private_metadata": "{\"decision_id\":\"hq-abc123\",\"choice\":2}",
  "blocks": [
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "*You selected:* Option 2: In-memory"
      }
    },
    {
      "type": "input",
      "block_id": "rationale_block",
      "optional": true,
      "element": {
        "type": "plain_text_input",
        "action_id": "rationale_input",
        "multiline": true,
        "placeholder": {"type": "plain_text", "text": "Why did you choose this option?"}
      },
      "label": {"type": "plain_text", "text": "Rationale (optional)"}
    }
  ]
}
```

## 6. Code Structure

```
slack-decision-bot/
├── cmd/
│   └── slackbot/
│       └── main.go           # Entry point
├── internal/
│   ├── bot/
│   │   ├── bot.go            # Main bot orchestration
│   │   ├── handlers.go       # Slack event handlers
│   │   └── messages.go       # Block Kit message builders
│   ├── gtclient/
│   │   └── client.go         # Gas Town RPC client wrapper
│   └── config/
│       └── config.go         # Configuration loading
├── go.mod
└── go.sum
```

### 6.1 Main Entry Point

```go
// cmd/slackbot/main.go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"

    "github.com/slack-go/slack"
    "github.com/slack-go/slack/socketmode"

    "slackbot/internal/bot"
    "slackbot/internal/config"
    "slackbot/internal/gtclient"
)

func main() {
    cfg := config.Load()

    // Connect to Gas Town RPC
    gt := gtclient.New(cfg.GTRpcURL, cfg.GTApiKey)

    // Connect to Slack
    api := slack.New(cfg.SlackBotToken,
        slack.OptionAppLevelToken(cfg.SlackAppToken))
    client := socketmode.New(api)

    // Create and run bot
    b := bot.New(api, client, gt, cfg)

    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()

    if err := b.Run(ctx); err != nil {
        log.Fatal(err)
    }
}
```

### 6.2 Bot Core

```go
// internal/bot/bot.go
package bot

type Bot struct {
    api      *slack.Client
    socket   *socketmode.Client
    gt       *gtclient.Client
    cfg      *config.Config
    posted   map[string]string // decision_id -> message_ts
}

func (b *Bot) Run(ctx context.Context) error {
    // Start decision watcher
    go b.watchDecisions(ctx)

    // Handle Slack events
    go b.handleEvents(ctx)

    // Run Socket Mode client
    return b.socket.RunContext(ctx)
}

func (b *Bot) watchDecisions(ctx context.Context) {
    ticker := time.NewTicker(b.cfg.PollInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            decisions, err := b.gt.ListPending(ctx)
            if err != nil {
                log.Printf("Error fetching decisions: %v", err)
                continue
            }
            for _, d := range decisions {
                if _, posted := b.posted[d.ID]; !posted {
                    b.postDecision(ctx, d)
                }
            }
        }
    }
}
```

## 7. RPC Integration

### 7.1 Current Server Status

| Method | Status | Bot Requirement |
|--------|--------|-----------------|
| `ListPending` | ✅ Implemented | Required - list decisions |
| `WatchDecisions` | ✅ Implemented | Required - stream new decisions |
| `GetDecision` | ❌ Stub | Optional - show details |
| `Resolve` | ❌ Stub | **CRITICAL** - resolve decisions |
| `Cancel` | ❌ Stub | Nice to have |

### 7.2 Required RPC Implementation

Before the Slack bot can resolve decisions, `DecisionServer.Resolve()` must be
implemented in `mobile/cmd/gtmobile/server.go`:

```go
func (s *DecisionServer) Resolve(
    ctx context.Context,
    req *connect.Request[gastownv1.ResolveRequest],
) (*connect.Response[gastownv1.ResolveResponse], error) {
    townBeadsPath := beads.GetTownBeadsPath(s.townRoot)
    client := beads.New(townBeadsPath)

    // Resolve the decision
    err := client.ResolveDecision(
        req.Msg.DecisionId,
        req.Msg.ChosenIndex,
        req.Msg.Rationale,
        "slack-bot", // resolver identity
    )
    if err != nil {
        return nil, connect.NewError(connect.CodeInternal, err)
    }

    // Fetch updated decision
    // ... return updated decision
}
```

### 7.3 Workaround (Pre-RPC Implementation)

Until `Resolve` RPC is implemented, the bot can shell out to `gt` CLI:

```go
func (c *Client) ResolveViaCmd(id string, choice int, rationale string) error {
    cmd := exec.Command("gt", "decision", "resolve", id,
        "--choice", strconv.Itoa(choice),
        "--rationale", rationale)
    return cmd.Run()
}
```

## 8. Authentication Model

### 8.1 Slack → Bot

- **App-Level Token** (`xapp-`): Authenticates Socket Mode connection
- **Bot Token** (`xoxb-`): Authenticates API calls
- **Signing Secret**: Verifies requests came from Slack (for HTTP fallback)

### 8.2 Bot → Gas Town

- **API Key**: Shared secret via `X-GT-API-Key` header
- Single key per gtmobile server instance
- No per-user authorization (workspace-level access)

### 8.3 Future: User Identity Mapping

To track who resolved decisions:

1. Slack user ID from interaction callback
2. Map to Gas Town identity (or use Slack username directly)
3. Pass identity to `Resolve` RPC as `resolved_by`

## 9. Deployment

### 9.1 Same-Host Deployment (Initial)

```bash
# Run alongside gtmobile on Gas Town server
./slackbot &

# Ensure gtmobile is running
gtmobile --port 8443 --town /path/to/town --api-key $GT_API_KEY &
```

### 9.2 Systemd Service

```ini
# /etc/systemd/system/gt-slackbot.service
[Unit]
Description=Gas Town Slack Decision Bot
After=network.target gt-mobile.service

[Service]
Type=simple
User=gastown
Environment=SLACK_APP_TOKEN=xapp-...
Environment=SLACK_BOT_TOKEN=xoxb-...
Environment=GT_RPC_URL=http://localhost:8443
Environment=GT_API_KEY=...
Environment=GT_SLACK_CHANNEL=#decisions
ExecStart=/usr/local/bin/slackbot
Restart=always

[Install]
WantedBy=multi-user.target
```

### 9.3 Docker Compose (Future)

```yaml
version: '3.8'
services:
  gtmobile:
    image: gastown/mobile:latest
    ports:
      - "8443:8443"
    volumes:
      - /path/to/town:/town
    command: --town /town --api-key ${GT_API_KEY}

  slackbot:
    image: gastown/slackbot:latest
    environment:
      - SLACK_APP_TOKEN
      - SLACK_BOT_TOKEN
      - GT_RPC_URL=http://gtmobile:8443
      - GT_API_KEY
      - GT_SLACK_CHANNEL=#decisions
    depends_on:
      - gtmobile
```

## 10. Implementation Tasks

### Phase 1: Prerequisites

1. **Implement `Resolve` RPC** in gtmobile server
   - Update `mobile/cmd/gtmobile/server.go`
   - Add `ResolveDecision` to beads client
   - Test via `gt decision watch --rpc`

2. **Implement `GetDecision` RPC** (optional but useful)

### Phase 2: Bot Core

3. **Create bot Go module**
   - Set up `slack-decision-bot/` directory
   - Add slack-go/slack dependency
   - Configure Go module

4. **Implement configuration loading**
   - Environment variables
   - Validation

5. **Implement Gas Town RPC client wrapper**
   - ListPending
   - WatchDecisions (or polling)
   - Resolve

### Phase 3: Slack Integration

6. **Implement Socket Mode connection**
   - Event loop
   - Reconnection handling

7. **Implement slash command handler**
   - `/decisions list`
   - `/decisions show <id>`

8. **Implement decision notification posting**
   - Block Kit message builder
   - Track posted messages

9. **Implement button interaction handler**
   - Parse action values
   - Open rationale modal

10. **Implement modal submission handler**
    - Extract rationale
    - Call Resolve RPC
    - Update original message

### Phase 4: Polish

11. **Add error handling and logging**
12. **Add health check endpoint** (optional)
13. **Write deployment documentation**
14. **Create Slack app manifest for easy installation**

## 11. Open Questions

1. **Multi-workspace support**: Should one bot instance support multiple Gas Town
   workspaces, or one bot per workspace?

2. **User mapping**: How to map Slack users to Gas Town identities for `resolved_by`?

3. **Notification preferences**: Should users be able to configure which urgency
   levels they want notifications for?

4. **Thread behavior**: Post decisions as new messages or in a dedicated thread?

## 12. References

- [slack-go/slack GitHub](https://github.com/slack-go/slack)
- [Socket Mode Documentation](https://docs.slack.dev/apis/events-api/using-socket-mode/)
- [Block Kit Builder](https://app.slack.com/block-kit-builder)
- `mobile/cmd/gtmobile/server.go` - RPC server implementation
- `internal/rpcclient/client.go` - Existing RPC client
