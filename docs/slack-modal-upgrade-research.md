# Slack App Modal Upgrade Research

**Issue:** bd-epc-slack_app_upgrade_modal_based_context.2
**Date:** 2026-01-29
**Status:** Research Complete

## Executive Summary

**Key Finding:** The current gtslack implementation ALREADY has full modal support via Socket Mode. The proposed upgrade is about ENHANCING context display within existing modals, not implementing modals from scratch.

The current implementation uses:
- Socket Mode (WebSocket-based, no public URL required)
- Bot token (`xoxb-*`) + App-level token (`xapp-*`)
- Interactive modals for decision resolution
- `views.open` API for modal display

## 1. Required Slack App Scopes for Modals

### Currently Configured (from `docs/slack-bot-setup.md`)

| Scope | Purpose | Required for Modals |
|-------|---------|---------------------|
| `chat:write` | Post messages | Yes (for message updates) |
| `commands` | Handle slash commands | Yes (provides trigger_id) |
| `users:read` | Get user info | No (optional) |
| `channels:read` | List channels | No (for routing) |
| `channels:join` | Join channels | No (for auto-join) |
| `channels:manage` | Create channels | No (for dynamic channels) |

### No Additional Scopes Required

Modal functionality (`views.open`, `views.push`, `views.update`) does **not** require dedicated OAuth scopes. The modal API methods use the bot token you already have.

**Key insight:** Modal permissions are implicit when you have:
1. A valid bot token (`xoxb-*`)
2. A valid `trigger_id` from a user interaction

## 2. OAuth Flow for Workspace Installation

### Current Setup (Single Workspace)

The current gtslack uses the **simple installation flow**:
1. "Install to Workspace" button in Slack App dashboard
2. Generates bot token (`xoxb-*`) instantly
3. No OAuth redirect implementation needed

This is appropriate for internal apps used in a single workspace.

### Multi-Workspace Distribution (Not Currently Needed)

If distributing to multiple workspaces, implement OAuth 2.0:

```
1. User clicks Install → Redirects to Slack OAuth URL
2. User authorizes → Slack redirects to your redirect_uri with code
3. Exchange code → POST oauth.v2.access with code
4. Store tokens → Save xoxb-* token per workspace
```

**Requirements for OAuth 2.0:**
- HTTPS redirect_uri endpoint
- Token storage (database per workspace)
- State parameter for CSRF protection

**Not needed for current single-workspace deployment.**

## 3. Endpoints for Interactivity

### With Socket Mode (Current Implementation)

**No public endpoints required!**

Socket Mode handles all interactivity via WebSocket:
- Slash commands → Delivered via WebSocket
- Button clicks → Delivered via WebSocket
- Modal submissions → Delivered via WebSocket

The current bot (`bot.go`) already handles:
```go
case socketmode.EventTypeSlashCommand:    // /decisions
case socketmode.EventTypeInteractive:      // Button clicks, modal submissions
```

### Without Socket Mode (HTTP Request Mode)

Would require public HTTPS endpoints:
- **Interactivity URL**: `https://your-domain.com/slack/interactive`
- **Slash Commands URL**: `https://your-domain.com/slack/commands`

**Recommendation:** Continue using Socket Mode. It's simpler and more secure.

## 4. Webhooks + App Coexistence

### Can They Coexist?

**Yes, but gtslack doesn't use traditional webhooks.**

The current architecture uses:
- **Socket Mode** for Slack communication (not webhooks)
- **SSE** from gtmobile for decision events (`/events/decisions`)

There are no Slack Incoming Webhooks in the current implementation.

### If You Wanted Webhooks

Traditional Slack Incoming Webhooks could coexist:
- Webhooks: One-way notifications (can't receive button clicks)
- App: Full interactivity (buttons, modals, slash commands)

But this adds complexity without benefit since Socket Mode already handles everything.

## 5. Minimal Viable App Configuration

### Current Configuration (Already Sufficient)

```yaml
# Slack App Settings (api.slack.com/apps)

Socket Mode: ENABLED
  App-Level Token: xapp-... (connections:write scope)

OAuth & Permissions:
  Bot Token Scopes:
    - chat:write
    - commands
    - users:read (optional)
    - channels:read
    - channels:join

Slash Commands:
  - /decisions → List pending decisions

Interactivity & Shortcuts:
  - Interactivity: ENABLED
  - Request URL: (empty - Socket Mode handles this)

Event Subscriptions:
  - channel_created (for auto-join)
```

### Tokens Required

| Token | Format | Purpose |
|-------|--------|---------|
| Bot Token | `xoxb-...` | API calls (post messages, open modals) |
| App Token | `xapp-...` | Socket Mode WebSocket connection |

## 6. Required gtslack Changes for Enhanced Context Display

### What Exists (bot.go)

The current implementation already has:
- `handleViewDecision()` - Shows decision details in-channel
- `buildResolveModal()` - Opens modal for resolution rationale
- `NotifyNewDecision()` - Posts decision with "View Details" button

### Proposed Enhancement

To show **full context in a modal** (instead of truncating to 2500 chars):

1. **Add "Show Full Context" button** to decision notifications
2. **Create context modal builder** with scrollable content
3. **Handle button click** to open context modal via `views.open`

### Code Changes Required

```go
// New action handler
case "show_full_context":
    b.handleShowFullContext(callback, action.Value)

// New modal builder
func (b *Bot) buildContextModal(decision rpcclient.Decision) slack.ModalViewRequest {
    // Build modal with full context (no 2500 char limit)
    // Modals support up to ~10 blocks
}

// New handler
func (b *Bot) handleShowFullContext(callback slack.InteractionCallback, decisionID string) {
    // Fetch decision, build modal, call views.open
}
```

### Block Limits

Slack modals have limits:
- **100 blocks** maximum per modal
- **3000 characters** per text block
- **3 views** in modal stack

For very long context, consider:
- Paginated views (using `views.push`)
- Collapsible sections
- "Full context" link to external URL

## 7. Hosting Requirements

### Current Requirements (Unchanged)

| Component | Requirement |
|-----------|-------------|
| gtslack | Go binary, any Linux host |
| Network | Outbound HTTPS to Slack API |
| gtmobile | Local or accessible RPC endpoint |
| Public URL | **NOT REQUIRED** (Socket Mode) |

### No New Infrastructure Needed

Socket Mode eliminates the need for:
- Public IP address
- HTTPS certificates
- Firewall rules for inbound traffic
- Load balancer / reverse proxy

## 8. Summary of Research Answers

| Question | Answer |
|----------|--------|
| 1. Scopes for modals? | No additional scopes needed. Current scopes (`chat:write`, `commands`) are sufficient. |
| 2. OAuth flow? | Current simple install is sufficient. OAuth 2.0 only needed for multi-workspace distribution. |
| 3. Interactivity endpoint? | None needed - Socket Mode handles everything via WebSocket. |
| 4. Keep webhooks? | N/A - gtslack uses Socket Mode, not webhooks. They could coexist if needed. |
| 5. Minimal config? | Current config is already minimal and sufficient. |

## 9. Recommendations

1. **No App Reconfiguration Needed** - Current Slack app setup supports modals
2. **Enhance Existing Code** - Add context modal to existing bot.go
3. **Keep Socket Mode** - Simpler, no public URL, already working
4. **Focus on UX** - The work is in building better context display, not infrastructure

## References

- [Slack Modals Documentation](https://docs.slack.dev/surfaces/modals/)
- [views.open API](https://docs.slack.dev/reference/methods/views.open/)
- [Installing with OAuth](https://docs.slack.dev/authentication/installing-with-oauth/)
- [OAuth Scopes Reference](https://docs.slack.dev/reference/scopes/)
- Current implementation: `internal/slackbot/bot.go`
- Setup guide: `docs/slack-bot-setup.md`
