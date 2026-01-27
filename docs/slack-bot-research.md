# Slack Bot Framework Research

**Task:** gt-w6i
**Status:** Complete
**Date:** 2026-01-27

## Executive Summary

For a Go-based Slack decision bot, we should use the **slack-go/slack** library with **Socket Mode** for development and optionally migrate to **HTTP Events API** for production. There is no official Bolt framework for Go.

## Key Findings

### 1. Go SDK Options

| Library | Status | Recommendation |
|---------|--------|----------------|
| [slack-go/slack](https://github.com/slack-go/slack) | Active, community-maintained | **Use this** |
| nlopes/slack | Deprecated, redirects to slack-go | Don't use |
| Slack Bolt | Not available for Go | N/A |

The `slack-go/slack` library is the authoritative Go library. It supports:
- All REST API methods
- Socket Mode (WebSocket-based, no public endpoint needed)
- Events API (HTTP webhook-based)
- RTM (deprecated, avoid)

### 2. Connection Modes

#### Socket Mode (Recommended for Development)
**Pros:**
- No public HTTPS endpoint required
- Works behind firewalls/NAT
- Faster development iteration
- Simpler infrastructure

**Cons:**
- Long-lived WebSocket connections
- Not recommended for Marketplace apps at scale

**Use case:** Internal tools, development, apps behind corporate firewalls

#### Events API over HTTP (Production)
**Pros:**
- Production-standard for published apps
- Stateless HTTP - more reliable at scale
- Better for distributed deployments

**Cons:**
- Requires public HTTPS endpoint
- More infrastructure setup

### 3. Interactive Features

| Feature | Purpose | Trigger |
|---------|---------|---------|
| Slash Commands | User-initiated actions (`/decide`) | User types command |
| Buttons/Menus | Interactive choices | User clicks element |
| Modals | Rich forms/dialogs | Triggered by commands/buttons |
| Block Kit | Rich message formatting | App constructs messages |

All features work with both Socket Mode and Events API.

### 4. Block Kit for Decision UX

Block Kit is Slack's UI framework for rich messages. Example decision message:

```json
{
  "blocks": [
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "*Decision Required* :warning:\n\nWhich caching strategy for the API layer?"
      }
    },
    {
      "type": "actions",
      "elements": [
        {
          "type": "button",
          "text": {"type": "plain_text", "text": "Redis"},
          "value": "option_1",
          "action_id": "decide_1"
        },
        {
          "type": "button",
          "text": {"type": "plain_text", "text": "In-memory"},
          "value": "option_2",
          "action_id": "decide_2"
        }
      ]
    }
  ]
}
```

### 5. Rate Limits

| API | Limit | Notes |
|-----|-------|-------|
| General | 1 req/sec/method/workspace | Burst allowed |
| Messages | 1 msg/sec/channel | All posting methods |
| conversations.history | 1 req/min (non-Marketplace) | New limit 2025-2026 |

**Handling:** Check for HTTP 429, use `Retry-After` header, implement exponential backoff.

### 6. Architecture Pattern

```go
// Socket Mode event loop
client := socketmode.New(slackClient)
go client.Run()

for evt := range client.Events {
    switch evt.Type {
    case socketmode.EventTypeSlashCommands:
        cmd := evt.Data.(slack.SlashCommand)
        client.Ack(*evt.Request)
        handleSlashCommand(cmd)

    case socketmode.EventTypeInteractive:
        interaction := evt.Data.(slack.InteractionCallback)
        client.Ack(*evt.Request)
        handleButtonClick(interaction)
    }
}
```

## Recommendations

1. **Use slack-go/slack** - The only mature Go library
2. **Start with Socket Mode** - Simpler development, no endpoint needed
3. **Design with Block Kit** - Rich, interactive decision UI
4. **Implement rate limit handling** - Critical for reliability
5. **Plan HTTP migration path** - If scaling beyond internal use

## References

- [slack-go/slack GitHub](https://github.com/slack-go/slack)
- [Socket Mode Examples](https://github.com/slack-go/slack/blob/master/examples/socketmode/)
- [Slack Block Kit](https://docs.slack.dev/block-kit/)
- [Events API Docs](https://docs.slack.dev/apis/events-api/)
- [Rate Limits](https://docs.slack.dev/apis/web-api/rate-limits/)
