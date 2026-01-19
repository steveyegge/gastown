# OpenCode Configuration Reference

> **Schema**: [opencode.ai/config.json](https://opencode.ai/config.json)  
> **GitHub**: [packages/opencode/src/config](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/config)  
> **Official Docs**: [opencode.ai/docs/configuration](https://opencode.ai/docs/configuration/)

---

## Configuration Files

| File | Location | Purpose |
|------|----------|---------|
| Global config | `~/.config/opencode/opencode.jsonc` | User-level settings |
| Project config | `.opencode/config.jsonc` | Project-level overrides |
| Plugins | `.opencode/plugin/<name>.js` | Plugin code |
| Commands | `.opencode/commands/<name>.md` | Custom slash commands |

---

## Core Settings

### Model Configuration

```jsonc
{
  // Default model for all sessions
  "model": "anthropic/claude-3-5-sonnet-20241022",
  
  // Provider-specific overrides
  "providers": {
    "anthropic": {
      "api_key": "${ANTHROPIC_API_KEY}"
    },
    "openai": {
      "api_key": "${OPENAI_API_KEY}"
    }
  }
}
```

### Available Models

```bash
# List all available models
opencode models

# Use specific model
opencode --model anthropic/claude-3-5-sonnet-20241022
```

Common model patterns:
- `anthropic/claude-3-5-sonnet-20241022`
- `openai/gpt-4o`
- `google/gemini-2.0-flash`

---

## Permissions

```jsonc
{
  // Permission modes: ask, allow, deny
  "permissions": {
    "*": "ask",              // Default: prompt user
    "file.read": "allow",    // Always allow file reads
    "shell.exec": "ask",     // Always ask for shell commands
    "network": "deny"        // Never allow network access
  }
}
```

**Environment variable** (for autonomous agents):
```bash
export OPENCODE_PERMISSION='{"*":"allow"}'
```

---

## Plugins

```jsonc
{
  "plugins": {
    // Local plugin
    "gastown": {
      "path": ".opencode/plugin/gastown.js"
    },
    // NPM plugin
    "my-plugin": {
      "package": "@my-org/opencode-plugin"
    }
  }
}
```

See [integration-guide.md#plugins](integration-guide.md#plugins) for plugin development.

---

## MCP (Model Context Protocol)

```jsonc
{
  "mcp": {
    "servers": {
      "filesystem": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/dir"]
      },
      "github": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-github"],
        "env": {
          "GITHUB_TOKEN": "${GITHUB_TOKEN}"
        }
      }
    }
  }
}
```

**Official MCP Servers**: [modelcontextprotocol.io](https://modelcontextprotocol.io/)

---

## Custom Commands

Create `.opencode/commands/<name>.md`:

```markdown
---
description: My custom command
---

Execute this task: $ARGUMENTS
```

Usage: `/my-command arg1 arg2`

---

## Tools

### Built-in Tools

| Tool | Description |
|------|-------------|
| `read_file` | Read file contents |
| `write_file` | Write/create files |
| `edit_file` | Edit file sections |
| `shell` | Execute shell commands |
| `search` | Search codebase |
| `web_search` | Search the web |

### Custom Tools (via Plugins)

```javascript
export const MyPlugin = async () => ({
  tool: {
    my_tool: {
      description: "My custom tool",
      parameters: {
        type: "object",
        properties: {
          input: { type: "string" }
        }
      },
      execute: async (params) => {
        return { result: `Processed: ${params.input}` }
      }
    }
  }
})
```

---

## Server Mode

```jsonc
{
  "server": {
    "host": "127.0.0.1",
    "port": 4096
  }
}
```

Start server:
```bash
opencode web           # Web server
opencode serve         # Generic server
opencode acp           # ACP protocol
```

---

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `OPENCODE_CONFIG` | Override config file path |
| `OPENCODE_PERMISSION` | JSON permission overrides |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `OPENAI_API_KEY` | OpenAI API key |
| `GOOGLE_API_KEY` | Google API key |

---

## Gastown Integration

For Gastown-specific configuration:

```bash
# Set OpenCode as default agent
gt config default-agent opencode

# Per-rig configuration
# In <rig>/settings/config.json:
{
  "agent": "opencode"
}
```

See [quickstart.md](quickstart.md) for Gastown setup.

---

## Further Reading

| Topic | Document |
|-------|----------|
| **Getting Started** | [quickstart.md](quickstart.md) |
| **Integration Methods** | [integration-guide.md](integration-guide.md) |
| **Events** | [events.md](events.md) |
| **External Resources** | [external-resources.md](external-resources.md) |
