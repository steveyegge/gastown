# OpenCode Plugin Implementation Guide for Gastown

**Date**: 2026-01-17  
**Based on**: OpenCode Repository Investigation  
**Status**: ✅ All Plugin Gaps Have Solutions

---

## Executive Summary

After investigating the OpenCode repository, **all three plugin gaps identified have straightforward solutions** using OpenCode's existing event system and hooks. No workarounds needed - OpenCode provides native support for all required functionality.

---

## OpenCode Event System Overview

### Available Events (from SDK)

OpenCode provides a comprehensive event system through the `Event` type:

**Session Events**:
- `session.created` - When session is created ✅ (already implemented)
- `session.updated` - When session metadata changes
- `session.deleted` - When session is deleted
- `session.status` - When session status changes (idle, retry, waiting, streaming)
- `session.idle` - When session becomes idle ✅ (can use for Stop hook)
- `session.compacted` - When session compaction completes
- `session.error` - When session encounters an error

**Message Events**:
- `message.updated` - When message content updates ✅ (solution for UserPromptSubmit)
- `message.removed` - When message is removed
- `message.part.updated` - When message part updates (streaming)
- `message.part.removed` - When message part is removed

**Other Events**:
- `command.executed` - When command is executed
- `permission.updated` - When permission changes
- `tool.execute.before` - Before tool executes
- `tool.execute.after` - After tool executes

### Available Hooks (from Plugin SDK)

OpenCode plugins can implement these hooks:

```typescript
interface Hooks {
  event?: (input: { event: Event }) => Promise<void>
  config?: (input: Config) => Promise<void>
  tool?: { [key: string]: ToolDefinition }
  auth?: AuthHook
  "chat.message"?: (input, output) => Promise<void>
  "chat.params"?: (input, output) => Promise<void>
  "permission.ask"?: (input, output) => Promise<void>
  "tool.execute.before"?: (input, output) => Promise<void>
  "tool.execute.after"?: (input, output) => Promise<void>
  "experimental.session.compacting"?: (input, output) => Promise<void>  // ✅ Solution for PreCompact!
  "experimental.text.complete"?: (input, output) => Promise<void>
}
```

---

## Solutions for Each Plugin Gap

### Gap 1: UserPromptSubmit Hook (HIGH Priority) ✅

**Claude Functionality**: Runs `gt mail check --inject` when user submits prompt

**OpenCode Solution**: Use `message.updated` event

**Implementation**:
```javascript
export const GasTown = async ({ $, directory }) => {
  const role = (process.env.GT_ROLE || "").toLowerCase();
  const interactiveRoles = new Set(["mayor", "crew"]);
  
  const run = async (cmd) => {
    try {
      await $`/bin/sh -lc ${cmd}`.cwd(directory);
    } catch (err) {
      console.error(`[gastown] ${cmd} failed`, err?.message || err);
    }
  };

  return {
    event: async ({ event }) => {
      // UserPromptSubmit equivalent
      if (event.type === "message.updated" && event.properties.info) {
        const message = event.properties.info;
        // Check if it's a user message (not assistant)
        if (message.role === "user" && interactiveRoles.has(role)) {
          await run("gt mail check --inject");
        }
      }
    },
  };
};
```

**Why This Works**:
- `message.updated` fires when any message is created or updated
- Message object has `role: "user"` or `role: "assistant"` field
- We filter for user messages only
- Equivalent to Claude's UserPromptSubmit timing

---

### Gap 2: PreCompact Hook (MEDIUM Priority) ✅

**Claude Functionality**: Runs `gt prime` before context compaction

**OpenCode Solution**: Use `experimental.session.compacting` hook

**Implementation**:
```javascript
export const GasTown = async ({ $, directory }) => {
  const run = async (cmd) => {
    try {
      await $`/bin/sh -lc ${cmd}`.cwd(directory);
    } catch (err) {
      console.error(`[gastown] ${cmd} failed`, err?.message || err);
    }
  };

  return {
    // PreCompact equivalent - runs BEFORE compaction
    "experimental.session.compacting": async (input, output) => {
      await run("gt prime");
      // Can also customize compaction prompt if needed:
      // output.context.push("Additional context for compaction");
      // or
      // output.prompt = "Custom compaction prompt";
    },
  };
};
```

**Why This Works**:
- `experimental.session.compacting` hook is specifically designed for pre-compaction logic
- Runs before compaction starts (exactly like Claude's PreCompact)
- Allows customization of compaction prompt if needed
- Native OpenCode feature, not a workaround

---

### Gap 3: Stop Hook (LOW Priority) ✅

**Claude Functionality**: Runs `gt costs record` when session stops

**OpenCode Solution**: Use `session.idle` event

**Implementation**:
```javascript
export const GasTown = async ({ $, directory }) => {
  const run = async (cmd) => {
    try {
      await $`/bin/sh -lc ${cmd}`.cwd(directory);
    } catch (err) {
      console.error(`[gastown] ${cmd} failed`, err?.message || err);
    }
  };

  let lastIdleTime = 0;

  return {
    event: async ({ event }) => {
      // Stop equivalent - runs when session becomes idle
      if (event.type === "session.idle") {
        const now = Date.now();
        // Debounce: only run if idle for > 5 seconds
        if (now - lastIdleTime > 5000) {
          await run("gt costs record");
          lastIdleTime = now;
        }
      }
    },
  };
};
```

**Why This Works**:
- `session.idle` fires when session completes work and enters idle state
- Equivalent to Claude's Stop timing (after work is done)
- Debouncing prevents multiple rapid calls
- Perfect for cost recording after session work

**Alternative**: Could also use `session.status` event and check for `status.type === "idle"`

---

## Complete Enhanced Plugin

Here's the complete Gastown plugin with full Claude parity:

```javascript
// Gas Town OpenCode plugin: Full parity with Claude hooks
export const GasTown = async ({ $, directory }) => {
  const role = (process.env.GT_ROLE || "").toLowerCase();
  const autonomousRoles = new Set(["polecat", "witness", "refinery", "deacon"]);
  const interactiveRoles = new Set(["mayor", "crew"]);
  let didInit = false;
  let lastIdleTime = 0;

  const run = async (cmd) => {
    try {
      await $`/bin/sh -lc ${cmd}`.cwd(directory);
    } catch (err) {
      console.error(`[gastown] ${cmd} failed`, err?.message || err);
    }
  };

  const onSessionCreated = async () => {
    if (didInit) return;
    didInit = true;
    await run("gt prime");
    if (autonomousRoles.has(role)) {
      await run("gt mail check --inject");
    }
    await run("gt nudge deacon session-started");
  };

  const onUserMessage = async () => {
    if (interactiveRoles.has(role)) {
      await run("gt mail check --inject");
    }
  };

  const onPreCompact = async () => {
    await run("gt prime");
  };

  const onIdle = async () => {
    const now = Date.now();
    if (now - lastIdleTime > 5000) {
      await run("gt costs record");
      lastIdleTime = now;
    }
  };

  return {
    // Event-based hooks
    event: async ({ event }) => {
      switch (event.type) {
        case "session.created":
          await onSessionCreated();
          break;
        
        case "message.updated":
          if (event.properties.info?.role === "user") {
            await onUserMessage();
          }
          break;
        
        case "session.idle":
          await onIdle();
          break;
      }
    },
    
    // Pre-compaction hook
    "experimental.session.compacting": async (input, output) => {
      await onPreCompact();
    },
  };
};
```

---

## Hook Comparison Table

| Claude Hook | OpenCode Solution | Method | Status |
|-------------|------------------|---------|--------|
| **SessionStart** | `session.created` event | `event` handler | ✅ Implemented |
| **UserPromptSubmit** | `message.updated` + role filter | `event` handler | ✅ Solution ready |
| **PreCompact** | `experimental.session.compacting` | Hook | ✅ Solution ready |
| **Stop** | `session.idle` event | `event` handler | ✅ Solution ready |

**Result**: ✅ **100% Feature Parity Achievable**

---

## Implementation Steps

### Step 1: Update Plugin File

Update `internal/opencode/plugin/gastown.js` with enhanced implementation:

```bash
# File: internal/opencode/plugin/gastown.js
# Replace entire file with complete implementation above
```

### Step 2: Test Event Handlers

Create test script to verify events fire:

```javascript
// test-plugin.js
export const TestPlugin = async ({ $ }) => {
  return {
    event: async ({ event }) => {
      console.log(`[test] Event: ${event.type}`, event.properties);
    },
    "experimental.session.compacting": async (input, output) => {
      console.log(`[test] Compacting session: ${input.sessionID}`);
    },
  };
};
```

### Step 3: Deploy and Test

1. Copy plugin to `.opencode/plugin/gastown.js`
2. Set `GT_ROLE=mayor` environment variable
3. Run test session:
   ```bash
   export GT_ROLE=mayor
   opencode run --model opencode/gpt-5-nano "test message"
   ```
4. Verify log output shows:
   - `session.created` fires
   - `message.updated` fires for user message
   - `gt mail check --inject` runs

### Step 4: Integration Test

Test complete workflow:
```bash
# Test autonomous role
GT_ROLE=polecat opencode run "create test file"
# Should see: gt prime, gt mail check --inject, gt nudge deacon

# Test interactive role
GT_ROLE=mayor opencode run "list files"
# First message: gt prime, gt nudge deacon
# Second message: gt mail check --inject (on user message)
```

---

## Event Discovery Script

Use this to discover all events in a session:

```javascript
// discover-events.js
export const EventDiscovery = async () => {
  const events = new Set();
  
  return {
    event: async ({ event }) => {
      if (!events.has(event.type)) {
        events.add(event.type);
        console.log(`[discovery] New event type: ${event.type}`);
        console.log(`[discovery] Properties:`, JSON.stringify(event.properties, null, 2));
      }
    },
    "experimental.session.compacting": async (input, output) => {
      console.log(`[discovery] Hook: experimental.session.compacting`);
      console.log(`[discovery] Input:`, input);
    },
  };
};
```

Run with:
```bash
opencode run --plugin ./discover-events.js "test session"
```

---

## Message Object Structure

For reference, `message.updated` event includes:

```typescript
{
  type: "message.updated",
  properties: {
    info: {
      id: string,           // Message ID
      sessionID: string,    // Session ID
      role: "user" | "assistant",  // ✅ Key field for filtering
      time: {
        created: number,    // Unix timestamp
        updated: number     // Unix timestamp
      },
      // ... other fields
    }
  }
}
```

---

## Session Status Types

For `session.status` event:

```typescript
SessionStatus = 
  | { type: "idle" }         // ✅ Can use for Stop hook
  | { type: "retry" }
  | { type: "waiting" }
  | { type: "streaming" }
```

---

## Best Practices

### 1. Error Handling

Always wrap `gt` commands in try-catch:
```javascript
const run = async (cmd) => {
  try {
    await $`/bin/sh -lc ${cmd}`.cwd(directory);
  } catch (err) {
    console.error(`[gastown] ${cmd} failed`, err?.message || err);
  }
};
```

### 2. Debouncing

Use debouncing for frequent events:
```javascript
let lastRun = 0;
const DEBOUNCE_MS = 5000;

if (Date.now() - lastRun > DEBOUNCE_MS) {
  await run(cmd);
  lastRun = Date.now();
}
```

### 3. Idempotency

Ensure operations are idempotent:
```javascript
let didInit = false;

if (!didInit) {
  await onInit();
  didInit = true;
}
```

### 4. Role Filtering

Always filter by role when needed:
```javascript
const autonomousRoles = new Set(["polecat", "witness", "refinery", "deacon"]);
if (autonomousRoles.has(role)) {
  // Autonomous-specific logic
}
```

---

## Testing Checklist

- [ ] SessionStart fires on new session
- [ ] Mail injection works for autonomous roles
- [ ] Mail injection works for interactive roles on user message
- [ ] PreCompact fires before compaction
- [ ] gt prime runs before compaction
- [ ] Stop/idle handler fires after session work
- [ ] gt costs record runs on stop
- [ ] No duplicate executions (idempotency works)
- [ ] Error handling prevents crashes
- [ ] Works with all role types

---

## Performance Considerations

**Event Frequency**:
- `message.updated` fires frequently during streaming
- Use role filter to minimize unnecessary checks
- Debounce expensive operations

**Async Operations**:
- All handlers are async
- OpenCode waits for handlers to complete
- Keep operations fast (<1 second ideal)

**Resource Usage**:
- Each `gt` command spawns a process
- Cache results when possible
- Avoid redundant operations

---

## Troubleshooting

### Plugin Not Loading

**Check**:
1. Plugin file in correct location: `.opencode/plugin/gastown.js`
2. Export format correct: `export const GasTown = async ({ $ }) => { ... }`
3. OpenCode config includes plugin

**Debug**:
```bash
opencode --help  # Check if plugin loaded
```

### Events Not Firing

**Check**:
1. Event handler registered: `event: async ({ event }) => { ... }`
2. Event type spelled correctly
3. Console.log to verify events received

**Debug**:
```javascript
event: async ({ event }) => {
  console.log(`Event: ${event.type}`);
  // ... rest of handler
}
```

### Commands Not Running

**Check**:
1. `gt` binary in PATH
2. Working directory correct
3. Shell configuration loaded (`-lc` flag)

**Debug**:
```javascript
const run = async (cmd) => {
  console.log(`Running: ${cmd}`);
  try {
    const result = await $`/bin/sh -lc ${cmd}`.cwd(directory);
    console.log(`Success: ${result.stdout}`);
  } catch (err) {
    console.error(`Failed: ${err.message}`);
  }
};
```

---

## Conclusion

**All three plugin gaps have native OpenCode solutions**:

1. ✅ **UserPromptSubmit** → `message.updated` event with role filter
2. ✅ **PreCompact** → `experimental.session.compacting` hook
3. ✅ **Stop** → `session.idle` event

**No workarounds needed** - OpenCode's event system and hooks provide everything required for 100% Claude parity.

**Next Steps**:
1. Update plugin implementation (5 minutes)
2. Deploy and test (15 minutes)
3. Verify all roles work correctly (30 minutes)
4. Mark as complete ✅

---

## References

- **OpenCode Repository**: https://github.com/anomalyco/opencode
- **Plugin SDK**: `packages/plugin/src/index.ts`
- **Event Types**: `packages/sdk/js/src/gen/types.gen.ts`
- **Example Plugin**: `packages/opencode/src/plugin/copilot.ts`
- **Hooks Documentation**: SDK type definitions

---

## Related Files

- `internal/opencode/plugin/gastown.js` - Current plugin (needs update)
- `docs/opencode/INTEGRATION_TEST_RESULTS.md` - Gap analysis
- `internal/claude/config/settings-*.json` - Claude hooks for comparison
