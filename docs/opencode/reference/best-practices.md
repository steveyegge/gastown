# OpenCode Best Practices

> **Purpose**: Performance, compatibility, and context optimization guidance  
> **Focus**: Evidence-based recommendations, not opinions

---

## Tool Count Management

### The Problem

Each tool adds to the system prompt, consuming context window and potentially confusing the LLM with too many options. OpenCode's 14+ built-in tools + MCP servers can quickly become overwhelming.

### Strategies

#### 1. Use Agent Profiles with Limited Tools

Define agents with only the tools they need:

```jsonc
// .opencode/config.jsonc
{
  "agents": {
    // Minimal agent - file operations only
    "editor": {
      "tools": ["read", "write", "edit", "list", "glob", "grep"]
    },
    
    // Research agent - read + web only
    "researcher": {
      "tools": ["read", "list", "grep", "webfetch", "skill"]
    },
    
    // Full agent - all tools (default)
    "full": {
      "tools": null  // null = all tools
    }
  }
}
```

**Usage**: `opencode --agent editor`

#### 2. Limit MCP Servers per Agent

Don't add every MCP server globally. Assign them per-agent:

```jsonc
{
  "agents": {
    "analyst": {
      "mcp": ["postgres", "memory"],  // Only DB + memory
      "tools": ["read", "grep", "list"]
    },
    "developer": {
      "mcp": ["github"],  // Only GitHub
      "tools": null       // All built-in tools
    }
  }
}
```

#### 3. Use Tool Permissions as Gatekeeping

Deny tools you don't need rather than removing them:

```jsonc
{
  "permission": {
    "webfetch": "deny",      // No web access
    "todowrite": "deny",     // No todo management
    "postgres_*": "deny"     // Disable DB tools by default
  }
}
```

The LLM still knows the tool exists but won't attempt to use it.

---

## Context Optimization

### Keep Sessions Short

Long sessions accumulate context. Use:
- **New sessions** for distinct tasks
- **Session forking** to branch from clean points
- **Compaction** (automatic) to summarize old messages

### Minimize Injected Context

In plugins, be selective about what you inject:

```javascript
// ❌ Bad - injects too much every time
"experimental.session.compacting": async (input, output) => {
  output.context.push(fs.readFileSync("entire-project-docs.md"));
}

// ✅ Good - inject only what's relevant
"experimental.session.compacting": async (input, output) => {
  output.context.push("Remember: This project uses Go 1.21 and PostgreSQL.");
}
```

### Use Skills for Complex Instructions

Instead of long system prompts, use SKILL.md files the LLM can load on-demand:

```markdown
<!-- .agent/skills/testing/SKILL.md -->
---
description: Testing conventions for this project
---

## Testing Rules
1. Use table-driven tests
2. Name tests as Test<Function>_<Scenario>
3. Use testify for assertions
```

The LLM loads this only when needed via the `skill` tool.

---

## Recommended MCP Servers

Based on compatibility testing and performance:

### ✅ Recommended

| Server | Use Case | Status | Notes |
|--------|----------|--------|-------|
| `@modelcontextprotocol/server-github` | GitHub API | ✅ Tested | Richer than curl |
| `@modelcontextprotocol/server-postgres` | PostgreSQL | ✅ Tested | Query DBs directly |
| `@modelcontextprotocol/server-memory` | Persistent memory | ✅ Tested | RAG/knowledge base |
| `@modelcontextprotocol/server-brave-search` | Web search | ✅ Tested | Better than webfetch for search |

### ⚠️ Use Carefully

| Server | Use Case | Status | Notes |
|--------|----------|--------|-------|
| `@modelcontextprotocol/server-filesystem` | File access | ⚠️ Redundant | OpenCode has built-in file tools |
| `@modelcontextprotocol/server-puppeteer` | Browser automation | ⚠️ Heavy | High latency, complex |
| `@modelcontextprotocol/server-slack` | Slack integration | ⚠️ Niche | Only if needed |

### ❌ Avoid (for performance)

| Server | Issue |
|--------|-------|
| Multiple similar servers | Tool confusion, context bloat |
| Servers with many tools | Each tool adds to prompt |
| Slow/unreliable servers | Blocks LLM execution |

---

## Plugin Best Practices

### Event Handler Performance

```javascript
// ❌ Bad - blocks on every event
event: async ({ event }) => {
  await expensiveOperation();  // Blocks all events
}

// ✅ Good - quick filter, async background
event: async ({ event }) => {
  if (event.type !== "session.idle") return;
  
  // Fire and forget for non-critical work
  setImmediate(() => recordCosts());
}
```

### Minimize Plugin Tools

Each plugin tool adds to the prompt. Only expose what's necessary:

```javascript
// ❌ Bad - too many tools
tool: {
  gt_prime: { ... },
  gt_mail: { ... },
  gt_costs: { ... },
  gt_status: { ... },
  gt_logs: { ... },
}

// ✅ Good - one tool with subcommands, or event-driven
// Use events for background work, one tool for user-facing actions
tool: {
  gt: {
    description: "Gastown operations: prime, mail, status",
    parameters: { command: { type: "string" } },
    execute: ({ command }) => { ... }
  }
}
```

---

## Gastown-Specific Recommendations

### Role-Based Tool Sets

| Role | Recommended Tools | MCP Servers |
|------|-------------------|-------------|
| Polecat | All built-in | None (task-focused) |
| Witness | `read`, `list`, `grep`, `bash` | None |
| Mayor | All built-in | `github` if needed |
| Crew | All built-in | Per-task basis |

### Avoid Tool Overlap

If using Gastown plugin tools, don't add MCP servers with overlapping functionality:
- Gastown handles file watching → don't add `filesystem` MCP
- Gastown handles git → use `github` MCP only for API, not file ops

---

## Performance Checklist

- [ ] Agent has only the tools it needs
- [ ] MCP servers limited to essential external services
- [ ] No redundant MCP servers (e.g., filesystem with OpenCode)
- [ ] Plugin event handlers are fast and non-blocking
- [ ] Plugin tools are minimal and well-described
- [ ] Skills used for complex instructions (not injected context)
- [ ] Sessions kept focused, compacted regularly

---

## Monitoring Tool Usage

Check which tools the LLM is using:

```javascript
export const ToolMonitor = async () => ({
  event: async ({ event }) => {
    if (event.type === "tool.execute.after") {
      console.log(`Tool: ${event.properties.info?.tool}`);
    }
  }
});
```

If the LLM rarely uses a tool, consider removing it.

---

## Further Reading

| Topic | Document |
|-------|----------|
| **Custom Agents** | [customization.md#custom-agents](customization.md#custom-agents) |
| **MCP Configuration** | [customization.md#mcp-servers](customization.md#mcp-servers) |
| **Tools Reference** | [tools.md](tools.md) |
| **Events** | [events.md](events.md) |
