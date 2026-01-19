# OpenCode Customization Reference

> **Purpose**: Guide for extending OpenCode beyond default capabilities  
> **Official Docs**: [opencode.ai/docs](https://opencode.ai/docs/)  
> **See Also**: [tools.md](tools.md) for custom tools, [events.md](events.md) for plugins

---

## Quick Reference

| Customization | Location | Purpose |
|---------------|----------|---------|
| **Custom Agents** | `.opencode/config.jsonc` | Different model/permission profiles |
| **Custom Commands** | `.opencode/commands/<name>.md` | Slash commands for common tasks |
| **Custom Tools** | Plugin or config | Extend LLM capabilities |
| **MCP Servers** | `.opencode/config.jsonc` | External services (DB, API) |
| **Plugins** | `.opencode/plugin/<name>.js` | Event handling, hooks |
| **Skills** | `.agent/skills/*.md` or `SKILL.md` | Reusable instructions |

---

## Custom Agents

Define custom agent profiles with different models, permissions, and instructions.

### Key Pattern: Separate Permissions from Instructions

OpenCode allows you to **separate configuration (permissions) from content (instructions)** by referencing external markdown files:

```jsonc
// .opencode/config.jsonc
{
  "agents": {
    "polecat": {
      "model": "anthropic/claude-sonnet-4-20250514",
      "instructions": ["roles/polecat.md"],   // Role-specific instructions
      "permission": { "*": "allow" }           // Autonomous permissions
    },
    "witness": {
      "model": "anthropic/claude-sonnet-4-20250514",
      "instructions": ["roles/witness.md"],
      "permission": {
        "file.read": "allow",
        "file.write": "ask",
        "shell.exec": "ask"
      }
    }
  }
}
```

This approach:
- **Keeps instructions as Markdown** - Easier to version, review, and share
- **Keeps permissions in config** - Machine-readable, can be templated
- **Enables reuse** - Same instructions, different permission profiles
- **Works with CLAUDE.md** - Can reference existing Claude Code instructions

### Configuration Examples

```jsonc
// .opencode/config.jsonc
{
  "agents": {
    // Autonomous agent - full permissions, role instructions
    "polecat": {
      "model": "anthropic/claude-sonnet-4-20250514",
      "instructions": ["CLAUDE.md"],           // Reuse existing CLAUDE.md
      "permission": { "*": "allow" }
    },
    
    // Interactive agent - confirm dangerous ops
    "crew": {
      "model": "anthropic/claude-sonnet-4-20250514",
      "instructions": ["roles/crew.md"],
      "permission": {
        "read": "allow",
        "list": "allow",
        "edit": "ask",
        "bash": "ask"
      }
    },
    
    // Fast agent for quick tasks
    "quick": {
      "model": "anthropic/claude-3-5-haiku-20241022",
      "permission": { "*": "allow" }
    },
    
    // Research agent with web access
    "researcher": {
      "model": "anthropic/claude-sonnet-4-20250514",
      "instructions": ["roles/researcher.md"],
      "permission": {
        "*": "allow",
        "webfetch": "allow"
      },
      "tools": ["webfetch", "read", "write"]  // Limit tool access
    }
  }
}
```

### Instructions File Format

Instructions files are plain Markdown:

```markdown
<!-- roles/polecat.md -->
# Polecat Worker

You are an autonomous worker agent.

## Your Role
- Execute assigned work from your hook
- Complete tasks and report done
- Do NOT wait idly - work or escalate

## Commands
- `gt prime` - Recover context
- `gt mail check --inject` - Get work assignments  
- `gt done` - Complete current work
```

### Usage

```bash
opencode --agent polecat    # Use polecat profile with its instructions
opencode --agent researcher # Use researcher profile
```

### Gastown Integration

Map Gastown roles to OpenCode agents:
```bash
gt sling issue-123 myrig --agent polecat
gt mayor start --agent crew
```

---

## Custom Commands

Create slash commands for common tasks. Commands are Markdown files with optional frontmatter.

### Location

```
.opencode/commands/<name>.md
```

### Basic Command

```markdown
<!-- .opencode/commands/review.md -->
---
description: Review the current changes
---

Review all staged changes for:
1. Code quality issues
2. Potential bugs
3. Security concerns
4. Performance problems

Then suggest improvements.
```

**Usage**: `/review`

### Command with Arguments

```markdown
<!-- .opencode/commands/explain.md -->
---
description: Explain a file or function
---

Explain the following in detail: $ARGUMENTS

Focus on:
- What it does
- How it works
- Why it's designed this way
```

**Usage**: `/explain src/utils/parser.go`

### Command with File Reference

```markdown
<!-- .opencode/commands/test.md -->
---
description: Generate tests for a file
---

Generate comprehensive unit tests for: $ARGUMENTS

Follow the patterns in @tests/example_test.go
```

### Available Variables

| Variable | Description |
|----------|-------------|
| `$ARGUMENTS` | Everything after the command name |
| `@path/to/file` | Include file contents inline |

### Same Pattern as Agents

Just like custom agents, commands can **reference external files** for their content:

```markdown
<!-- .opencode/commands/polecat-work.md -->
---
description: Start polecat work mode
---

@roles/polecat.md

Now check your hook and start working:
- Run `gt prime && bd prime`
- Check `gt hook` for assigned work
- Execute steps with `bd ready` / `bd close`
```

This keeps role-specific instructions in one place (`roles/*.md`) that both agents and commands can reference.

---

## Skills (SKILL.md)

Skills are reusable instruction sets the LLM can load via the `skill` tool.

### Location

```
.agent/skills/<name>/SKILL.md   # Standard location
SKILL.md                         # Project root
```

### Example Skill

```markdown
<!-- .agent/skills/testing/SKILL.md -->
---
name: testing
description: How to write tests for this project
---

## Testing Conventions

1. Use Go's standard testing package
2. Table-driven tests preferred
3. Name tests as `Test<Function>_<Scenario>`
4. Use testify for assertions

## Example

```go
func TestParse_ValidInput(t *testing.T) {
    // ...
}
```
```

**Usage**: LLM calls `skill` tool to load instructions when needed.

---

## MCP Servers

MCP (Model Context Protocol) servers provide access to external services. Unlike built-in tools, MCP is for **external data and APIs** that OpenCode doesn't have natively.

### When to Use MCP

| Use Case | Use MCP? | Reason |
|----------|----------|--------|
| Read/write files | ❌ No | Use built-in `read`, `write`, `edit` |
| Search codebase | ❌ No | Use built-in `grep`, `glob`, `list` |
| Run shell commands | ❌ No | Use built-in `bash` |
| Query database | ✅ Yes | No built-in database access |
| Access GitHub API | ✅ Yes | Richer than `bash` + `curl` |
| Browse web | ⚠️ Maybe | Built-in `webfetch` may suffice |

### Configuration

```jsonc
// .opencode/config.jsonc
{
  "mcp": {
    "servers": {
      // GitHub API access
      "github": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-github"],
        "env": { "GITHUB_TOKEN": "${GITHUB_TOKEN}" }
      },
      
      // Database access
      "postgres": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-postgres"],
        "env": { "DATABASE_URL": "${DATABASE_URL}" }
      },
      
      // Memory/knowledge base
      "memory": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-memory"]
      },
      
      // Custom MCP server
      "myservice": {
        "command": "./scripts/mcp-myservice.sh",
        "args": [],
        "env": { "API_KEY": "${MY_API_KEY}" }
      }
    }
  }
}
```

### Permission Control

Control MCP tools with wildcards:
```jsonc
{
  "permission": {
    "github_*": "allow",     // All GitHub tools
    "postgres_*": "ask",     // Confirm database queries
    "myservice_*": "deny"    // Block custom service
  }
}
```

### How OpenCode MCP Differs from Other Runtimes

| Aspect | OpenCode | Claude Code | Gemini |
|--------|----------|-------------|--------|
| **Config location** | `.opencode/config.jsonc` | `~/.claude/mcp_config.json` | `.gemini/settings.json` |
| **Config format** | JSONC with `mcp.servers` | JSON with `mcpServers` | YAML/JSON |
| **Environment vars** | `"${VAR}"` syntax | Direct or `env` object | Direct |
| **Permission control** | `permission.server_*` | Global MCP setting | Per-server |
| **Hot reload** | ✅ On config change | ❌ Restart required | ✅ |

---

## Plugins

Plugins are JavaScript files that hook into OpenCode's event system.

### Location

```
.opencode/plugin/<name>.js
```

### Basic Plugin

```javascript
export const MyPlugin = async ({ $, directory }) => ({
  // Handle events
  event: async ({ event }) => {
    if (event.type === "session.created") {
      console.log("Session started:", event.properties.info?.id);
    }
  },
  
  // Register custom tools
  tool: {
    my_tool: {
      description: "My custom tool",
      parameters: { type: "object", properties: {} },
      execute: async () => ({ result: "done" })
    }
  },
  
  // Pre-compaction hook
  "experimental.session.compacting": async (input, output) => {
    output.context.push("Remember to check tests");
  }
});
```

See [integration-guide.md#plugins](integration-guide.md#plugins) for full plugin development guide.

---

## Configuration Precedence

Settings are merged with later sources overriding earlier:

1. **Defaults** - OpenCode built-in defaults
2. **Global config** - `~/.config/opencode/opencode.jsonc`
3. **Project config** - `.opencode/config.jsonc`
4. **Environment variables** - `OPENCODE_PERMISSION`, etc.
5. **CLI flags** - `--model`, `--agent`, etc.

---

## Further Reading

| Topic | Document |
|-------|----------|
| **Configuration** | [configuration.md](configuration.md) |
| **Tools** | [tools.md](tools.md) |
| **Events** | [events.md](events.md) |
| **Plugins** | [integration-guide.md#plugins](integration-guide.md#plugins) |
