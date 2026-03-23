# Telegram Bridge

The Telegram bridge lets the human overseer chat with the Mayor agent and
receive workspace notifications from a mobile device or any Telegram client.

## Quick Start

```bash
# 1. Create a bot via @BotFather on Telegram, copy the token
# 2. Send /start to your bot, then get your chat ID:
#    curl https://api.telegram.org/bot<TOKEN>/getUpdates
#    → result[0].message.chat.id

# 3. Configure the bridge
gt telegram configure \
    --token "123456789:AAH..." \
    --chat-id <CHAT_ID> \
    --allow-from <USER_ID>

# 4. Enable the daemon patrol
#    This step is required — configuring the bridge alone does NOT start it.
#    Add telegram_bridge to the patrols section of mayor/daemon.json:
#    { "patrols": { "telegram_bridge": { "enabled": true } } }
#    Then restart the daemon:
gt daemon stop && gt daemon start

# Or run standalone (for testing):
gt telegram run
```

## How It Works

```
You (Telegram)                    Gas Town
    │                                │
    │  "Hi mayor, status?"           │
    ├───────────────────────────────→ │
    │   Bot API long-poll             │
    │   → AccessGate (allow_from)     │
    │   → RateLimiter (30/min)        │
    │   → gt mail send mayor/         │
    │   → gt nudge hq-mayor           │
    │                                 │
    │                          Mayor processes
    │                          mail, replies to
    │                          overseer inbox
    │                                 │
    │  "@mayor/: All systems green"   │
    │ ←──────────────────────────────┤
    │   ReplyForwarder polls          │
    │   overseer inbox every 3s       │
    │   → bot.SendMessage()           │
```

### Inbound (Telegram → Mayor)

1. Bot long-polls Telegram with 30s timeout
2. Access gate rejects bots, checks `allow_from` list (fail-closed: empty list blocks everyone)
3. Rate limiter enforces per-user sliding window (default 30 msgs/min)
4. Message sent as mail: `from: overseer`, `to: mayor/`, `subject: Telegram`
5. Nudge queued to `hq-mayor` via `gt nudge hq-mayor --mode=queue`
6. Mayor picks up the message on its next turn via its existing `UserPromptSubmit`
   hook (`gt mail check --inject`), which checks for queued nudges and injects
   unread mail into the prompt

### Mayor's Side (no configuration needed)

The Mayor requires no changes to work with Telegram. It uses its existing mail
infrastructure:

- **Receiving**: The Mayor's `UserPromptSubmit` hook already runs
  `gt mail check --inject`, which picks up nudges and injects unread mail.
  Telegram messages arrive as ordinary mail from `overseer` with subject
  `Telegram`.
- **Replying**: The Mayor replies normally via `gt mail send overseer` or
  `gt mail reply`. The reply forwarder detects these replies in the overseer
  inbox and forwards them to Telegram automatically.
- **No special handling**: From the Mayor's perspective, Telegram messages are
  indistinguishable from any other overseer mail. It doesn't know or care that
  the message originated from Telegram.

### Outbound: Mayor Replies → Telegram

1. Reply forwarder polls `bd list` for messages assigned to overseer with
   `gt:message` label every 3 seconds
2. Skips messages `from:overseer` (our own outbound) and already-forwarded IDs
3. Forwards to Telegram, marks as forwarded only after successful delivery
4. On bridge startup, seeds the forwarded set with all existing messages to
   prevent duplicates on restart

### Outbound: Event Notifications → Telegram

The bridge tails `.feed.jsonl` and forwards events matching configured categories:

| Category | Events | Description |
|----------|--------|-------------|
| `stuck_agents` | `mass_death`, `session_death` | Agent sessions dying unexpectedly |
| `escalations` | `escalation_sent` | Problems agents couldn't auto-resolve |
| `merge_failures` | `merge_failed` | Refinery merge queue failures |

Default: `["escalations"]`. Configure more with `--notify stuck_agents,escalations,merge_failures`.

### Thread Mapping

A bidirectional `MessageMap` (10k entries, FIFO eviction) maps Telegram message
IDs to mail thread IDs. When the Mayor replies to a thread that originated from
Telegram, the outbound message is sent as a Telegram reply to the original
message, preserving conversation context.

## Configuration

Config lives at `<town-root>/mayor/telegram.json` with `0600` permissions.
The bridge refuses to start if the file is group- or world-readable.

```json
{
  "token": "123456789:AAH...",
  "chat_id": 123456789,
  "allow_from": [123456789],
  "target": "mayor/",
  "enabled": true,
  "notify": ["escalations"],
  "rate_limit": 30
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `token` | string | required | BotFather bot token |
| `chat_id` | int64 | required | Telegram chat for outbound messages |
| `allow_from` | []int64 | `[]` | Allowed user IDs (fail-closed) |
| `target` | string | `"mayor/"` | Mail recipient for inbound messages |
| `enabled` | bool | `true` | Enable/disable the bridge |
| `notify` | []string | `["escalations"]` | Notification categories |
| `rate_limit` | int | `30` | Max inbound messages per user per minute |

## CLI Commands

```bash
gt telegram configure   # Set token, chat ID, allow list, notifications
gt telegram status      # Show connection status, message counts
gt telegram status --json
gt telegram run         # Run bridge as standalone process
```

## Deployment Modes

### Standalone (`gt telegram run`)

Long-running process using `CLISender` — shells out to `gt mail send` and
`gt nudge` for delivery. Good for testing or when you don't want to modify the
daemon config.

### Daemon Patrol (recommended)

Bridge runs as a goroutine inside the daemon using `DirectSender` — calls Go
APIs directly, avoiding subprocess overhead.

**Both** the config file and the daemon patrol must be set up:

1. Configure the bridge: `gt telegram configure --token ... --chat-id ... --allow-from ...`
   (creates `mayor/telegram.json`)
2. Enable the patrol in `mayor/daemon.json`:
   ```json
   { "patrols": { "telegram_bridge": { "enabled": true } } }
   ```
3. Restart the daemon: `gt daemon stop && gt daemon start`

The config file (`mayor/telegram.json`) controls *what* the bridge connects to.
The daemon patrol entry controls *whether* the daemon starts it. Without the
patrol entry, the bridge won't run even if the config file is valid.

Both deployment modes use identical bridge logic. Only the `Sender` implementation differs.

## Architecture

```
internal/bridge/telegram/
  config.go      Config struct, validation, file I/O (0600 enforced)
  bot.go         Telegram Bot API: long-poll, send, AccessGate, RateLimiter
  bridge.go      Lifecycle orchestrator: retry loop, panic recovery, shutdown
  inbound.go     Telegram message → gt mail send + gt nudge
  outbound.go    .feed.jsonl tail → category filter → bot.SendMessage
  reply.go       Overseer inbox poll → bot.SendMessage (with dedup seeding)
  msgmap.go      Bidirectional Telegram msgID ↔ mail ThreadID (FIFO eviction)
  sender.go      Sender interface, DirectSender (daemon), CLISender (standalone)

internal/cmd/telegram.go     CLI: configure, status, run
internal/daemon/daemon.go    Daemon integration (start/stop bridge goroutine)
internal/daemon/types.go     TelegramBridgeConfig patrol type
```

## Security

| Concern | Mitigation |
|---------|-----------|
| Token storage | `0600` permissions enforced at load time. Token masked in logs and status output |
| Inbound access | `allow_from` is fail-closed. Empty list blocks all users |
| Bot messages | Always rejected (`is_bot` check before allow-list) |
| Rate limiting | Per-user sliding window, configurable (default 30/min) |
| Outbound | Only sends to configured `chat_id`, never to IDs from message content |

## Error Handling

- **Connection failures**: 5s backoff retry loop with panic recovery
- **Mail send failures**: Logged, message stays unread for retry on next cycle
- **Duplicate prevention**: On startup, all existing messages are seeded as
  already-forwarded. The forwarded set and message map persist across reconnects
  within the same process
- **Feed file rotation**: Detects inode changes and re-opens automatically

## Scope (v1)

- Text messages only (no files, images, or documents)
- Single chat ID (no group chat routing)
- No message editing (Telegram edits are ignored)
- No inline keyboards or bot commands (plain text only)
- No hot-reload (restart bridge to pick up config changes)

## Dependencies

- `github.com/go-telegram-bot-api/telegram-bot-api/v5` (same library Thrum uses)
