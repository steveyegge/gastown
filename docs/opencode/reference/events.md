# OpenCode Events Reference

> **Purpose**: Comprehensive event type documentation for plugins and SDK  
> **Source of Truth**: [types.gen.ts](https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts)  
> **Official Docs**: [opencode.ai/docs/plugins](https://opencode.ai/docs/plugins/)

---

## Event Types

> **Type Definitions**: [types.gen.ts](https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts)  
> **Event Type Union**: Search for `Event` in types.gen.ts

### Common Event Structure

All events follow this structure:
```typescript
{
  type: string;           // Event type (e.g., "session.created")
  properties: {
    info?: object;        // Event-specific data
    // ... other properties
  }
}
```

### Session Events

| Event | Description | Key Properties |
|-------|-------------|----------------|
| `session.created` | Session initialized | `info.id`, `info.title`, `info.model` |
| `session.updated` | Session metadata changed | `info.id`, `info.title` |
| `session.deleted` | Session removed | `info.id` |
| `session.status` | Status changed | `info.status`: `idle`, `streaming`, `waiting`, `retry` |
| `session.idle` | Session finished work | `info.id` |
| `session.compacted` | Compaction completed | `info.id`, `info.messageCount` |
| `session.error` | Session error occurred | `info.error`, `info.message` |
| `session.diff` | File diff generated | `info.path`, `info.diff` |

**Example - session.created**:
```javascript
event.properties.info = {
  id: "sess_abc123",
  title: "Fix button styling",
  model: "anthropic/claude-3-5-sonnet-20241022"
}
```

### Message Events

| Event | Description | Key Properties |
|-------|-------------|----------------|
| `message.updated` | Message created/modified | `info.id`, `info.role`, `info.content` |
| `message.removed` | Message deleted | `info.id` |
| `message.part.updated` | Streaming update | `info.messageId`, `info.partIndex`, `info.content` |
| `message.part.removed` | Part deleted | `info.messageId`, `info.partIndex` |

**Example - message.updated** (user message):
```javascript
event.properties.info = {
  id: "msg_xyz789",
  role: "user",           // "user" or "assistant"
  content: "Fix the button"
}
```

**Tip**: Filter for user messages with `event.properties.info?.role === "user"`.

### Tool Events

| Event | Description | Key Properties |
|-------|-------------|----------------|
| `tool.execute.before` | Before tool runs | `info.tool`, `info.input` |
| `tool.execute.after` | After tool completes | `info.tool`, `info.output`, `info.error` |

**Example - tool.execute.after**:
```javascript
event.properties.info = {
  tool: "edit",
  input: { path: "src/app.js", ... },
  output: { success: true },
  error: null
}
```

### Other Events

| Event | Description | Key Properties |
|-------|-------------|----------------|
| `command.executed` | Slash command executed | `info.command`, `info.args` |
| `permission.updated` | Permission changed | `info.tool`, `info.permission` |
| `permission.replied` | User response | `info.tool`, `info.allowed` |
| `file.edited` | File modified | `info.path`, `info.type` |
| `todo.updated` | Todo list modified | `info.todos` |

### TUI Events

| Event | Description | Key Properties |
|-------|-------------|----------------|
| `tui.prompt.append` | Text appended | `info.text` |
| `tui.command.execute` | Command triggered | `info.command` |
| `tui.toast.show` | Toast shown | `info.message`, `info.level` |

---

## Hooks (Interceptors)

Unlike events (read-only notifications), **hooks** can modify behavior. You handle events via the `event` function; you implement hooks as named functions in your plugin:

| Hook | Description |
|------|-------------|
| `chat.message` | Modify chat messages before sending |
| `chat.params` | Modify chat parameters |
| `permission.ask` | Modify permission prompts |
| `tool.execute.before` | Intercept before tool runs |
| `tool.execute.after` | Process tool results |
| `experimental.session.compacting` | Pre-compact hook (Gastown: `PreCompact`) |

---

## Usage

### Plugin Event Handler

```javascript
export const MyPlugin = async ({ $ }) => {
  return {
    event: async ({ event }) => {
      switch (event.type) {
        case "session.created":
          console.log("Session started:", event.properties.info?.id);
          break;
        case "message.updated":
          if (event.properties.info?.role === "user") {
            console.log("User message received");
          }
          break;
        case "session.idle":
          console.log("Session idle");
          break;
      }
    },
  };
};
```

### SDK Event Subscription

```javascript
const events = await client.event.subscribe();
for await (const event of events.stream) {
  console.log("Event:", event.type, event.properties);
}
```

---

## Gastown Hook Mapping

| Claude Hook | OpenCode Event/Hook | Implementation |
|-------------|--------------------| ---------------|
| `SessionStart` | `session.created` event | `event` handler |
| `UserPromptSubmit` | `message.updated` event (filter `role=user`) | `event` handler |
| `PreCompact` | `experimental.session.compacting` hook | Named hook |
| `Stop` | `session.idle` event | `event` handler with debounce |

See [integration-guide.md](integration-guide.md#plugins) for plugin implementation details and [../design/gastown-plugin.md](../design/gastown-plugin.md) for Gastown-specific patterns.
