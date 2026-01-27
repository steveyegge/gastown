# Connect-RPC Integration Architecture

**Task:** gt-gp28
**Status:** Complete
**Date:** 2026-01-27

## Executive Summary

Gas Town already has a Connect-RPC mobile backend (`gtmobile`) with decision service endpoints. A Slack bot can connect as an RPC client using the existing `rpcclient` package. Some RPC methods need implementation before full bidirectional support is possible.

## Existing Infrastructure

### RPC Server Location
```
mobile/cmd/gtmobile/
├── main.go      # Server entry point (port, TLS, API key)
└── server.go    # Service implementations
```

### Proto Definitions
```
mobile/proto/gastown/v1/
├── decision.proto   # Decision service
├── status.proto     # Status service
├── mail.proto       # Mail service
└── common.proto     # Shared types
```

### Client Library
```
internal/rpcclient/client.go   # Ready-to-use RPC client
```

## Decision Service API

### Proto Definition
```protobuf
service DecisionService {
  rpc ListPending(ListPendingRequest) returns (ListPendingResponse);
  rpc GetDecision(GetDecisionRequest) returns (GetDecisionResponse);
  rpc Resolve(ResolveRequest) returns (ResolveResponse);
  rpc Cancel(CancelRequest) returns (CancelResponse);
  rpc WatchDecisions(WatchDecisionsRequest) returns (stream Decision);
}
```

### Decision Message
```protobuf
message Decision {
  string id = 1;
  string question = 2;
  string context = 3;
  repeated DecisionOption options = 4;
  int32 chosen_index = 5;
  string rationale = 6;
  string resolved_by = 7;
  string resolved_at = 8;
  AgentAddress requested_by = 9;
  string requested_at = 10;
  Urgency urgency = 11;
  repeated string blockers = 12;
  bool resolved = 13;
  bool cancelled = 14;
}
```

### Implementation Status

| Method | Status | Notes |
|--------|--------|-------|
| ListPending | Implemented | Returns pending decisions |
| WatchDecisions | Implemented | Polls every 5 seconds |
| GetDecision | Stub | Needs implementation |
| Resolve | Stub | **Critical for Slack bot** |
| Cancel | Stub | Needs implementation |

## Authentication

### Current Model
- **API Key**: Single shared key via `X-GT-API-Key` header
- **TLS**: Optional HTTPS with cert/key files
- **Workspace isolation**: Server binds to specific town root

### Starting the Server
```bash
gtmobile --port 8443 --town /path/to/workspace --api-key <key>
```

### Client Connection
```go
client := rpcclient.NewClient("http://localhost:8443",
    rpcclient.WithAPIKey("shared-key"))
```

## Integration Architecture

### Option A: RPC Client (Recommended)

```
┌─────────────┐     HTTP/Connect-RPC     ┌──────────────┐
│  Slack Bot  │ ──────────────────────── │  gtmobile    │
│  (Go)       │                          │  RPC Server  │
└─────────────┘                          └──────────────┘
       │                                        │
       │ Slack API                              │ File I/O
       ▼                                        ▼
┌─────────────┐                          ┌──────────────┐
│   Slack     │                          │  Beads DB    │
│   Workspace │                          │  (Decisions) │
└─────────────┘                          └──────────────┘
```

**Flow:**
1. Slack bot starts, connects to RPC server
2. Bot calls `WatchDecisions()` to stream updates
3. New decision arrives → Bot posts to Slack with buttons
4. User clicks button → Bot receives interaction
5. Bot calls `Resolve()` RPC → Decision marked resolved
6. Bot updates Slack message to show resolution

### Option B: Direct File Access (Simpler but Coupled)

```
┌─────────────┐     File I/O     ┌──────────────┐
│  Slack Bot  │ ──────────────── │  Beads DB    │
│  (Go)       │                  │  (Decisions) │
└─────────────┘                  └──────────────┘
```

Not recommended - bypasses the abstraction layer and couples to storage format.

### Option C: Shell-out to gt CLI

```go
// Read decisions
out, _ := exec.Command("gt", "decision", "list", "--json").Output()

// Resolve decision
exec.Command("gt", "decision", "resolve", id,
    "--choice", "1", "--rationale", "Approved via Slack").Run()
```

Viable short-term workaround if RPC Resolve isn't ready.

## Required Work

### Before Slack Bot Can Resolve Decisions

1. **Implement `DecisionServer.Resolve()`** in `mobile/cmd/gtmobile/server.go`
   - Accept decision ID, chosen option index, rationale
   - Call `beads.ResolveDecision()` to persist
   - Return success/error

2. **Implement `DecisionServer.GetDecision()`** for detail views
   - Accept decision ID
   - Return full decision details

### Optional Enhancements

- True streaming (currently polling every 5s)
- Per-user authentication (currently single API key)
- Multi-workspace support in single server

## Slack Bot Integration Code

```go
package main

import (
    "context"
    "log"

    "github.com/slack-go/slack"
    "github.com/slack-go/slack/socketmode"
    "gastown/internal/rpcclient"
)

func main() {
    // Connect to Gas Town RPC
    gt := rpcclient.NewClient("http://localhost:8443",
        rpcclient.WithAPIKey(os.Getenv("GT_API_KEY")))

    // Connect to Slack
    api := slack.New(os.Getenv("SLACK_BOT_TOKEN"),
        slack.OptionAppLevelToken(os.Getenv("SLACK_APP_TOKEN")))
    client := socketmode.New(api)

    // Watch for decisions and post to Slack
    go func() {
        gt.WatchDecisions(context.Background(), func(d rpcclient.Decision) error {
            blocks := formatDecisionBlocks(d)
            _, _, err := api.PostMessage(channel, slack.MsgOptionBlocks(blocks...))
            return err
        })
    }()

    // Handle Slack interactions
    go func() {
        for evt := range client.Events {
            if evt.Type == socketmode.EventTypeInteractive {
                interaction := evt.Data.(slack.InteractionCallback)
                client.Ack(*evt.Request)
                handleDecisionButton(gt, api, interaction)
            }
        }
    }()

    client.Run()
}
```

## Deployment Options

| Option | Pros | Cons |
|--------|------|------|
| Same host as Gas Town | Low latency, simple | Single point of failure |
| Separate container | Isolated, scalable | Network complexity |
| Serverless (Lambda) | Auto-scaling | Cold starts, connection limits |

**Recommendation:** Start on same host, containerize later if needed.

## References

- `mobile/cmd/gtmobile/server.go` - RPC server implementation
- `internal/rpcclient/client.go` - Client library
- `internal/beads/beads_decision.go` - Decision storage
- `internal/cmd/decision.go` - CLI implementation
