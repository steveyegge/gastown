# OpenCode Tools Reference

> **Purpose**: Complete reference for OpenCode built-in tools and custom tool options  
> **Official Docs**: [opencode.ai/docs/tools](https://opencode.ai/docs/tools/)  
> **Source Code**: [packages/opencode/src/tool](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/tool)

---

## Quick Reference

| Tool | Permission | Description |
|------|------------|-------------|
| `bash` | `bash` | Execute shell commands |
| `read` | `read` | Read file contents |
| `write` | `edit` | Create/overwrite files |
| `edit` | `edit` | Modify files with exact replacements |
| `patch` | `edit` | Apply patch diffs |
| `grep` | `grep` | Search file contents (regex) |
| `glob` | `glob` | Find files by pattern |
| `list` | `list` | List directory contents |
| `lsp` | `lsp` | LSP code intelligence (experimental) |
| `skill` | `skill` | Load SKILL.md files |
| `todowrite` | `todowrite` | Manage todo lists |
| `todoread` | `todoread` | Read todo lists |
| `webfetch` | `webfetch` | Fetch web content |
| `question` | `question` | Ask user questions |

---

## Built-in Tools

### bash

Execute shell commands in your project environment.

```jsonc
{ "permission": { "bash": "allow" } }
```

**Examples**: `npm install`, `git status`, `make build`

### read

Read file contents from your codebase. Supports reading specific line ranges.

```jsonc
{ "permission": { "read": "allow" } }
```

### write / edit / patch

File modification tools - all controlled by the `edit` permission:

| Tool | Purpose |
|------|---------|
| `write` | Create new files or overwrite existing |
| `edit` | Modify files using exact string replacements |
| `patch` | Apply unified diff patches |

```jsonc
{ "permission": { "edit": "allow" } }
```

### grep

Search file contents using regular expressions. Uses [ripgrep](https://github.com/BurntSushi/ripgrep) internally.

```jsonc
{ "permission": { "grep": "allow" } }
```

Respects `.gitignore` patterns by default.

### glob

Find files by pattern matching (e.g., `**/*.js`, `src/**/*.ts`).

```jsonc
{ "permission": { "glob": "allow" } }
```

Returns paths sorted by modification time.

### list

List files and directories in a given path. Supports glob filtering.

```jsonc
{ "permission": { "list": "allow" } }
```

### lsp (experimental)

LSP code intelligence features. Requires `OPENCODE_EXPERIMENTAL_LSP_TOOL=true`.

```jsonc
{ "permission": { "lsp": "allow" } }
```

**Supported operations**:
- `goToDefinition`, `findReferences`
- `hover`, `documentSymbol`, `workspaceSymbol`
- `goToImplementation`
- `prepareCallHierarchy`, `incomingCalls`, `outgoingCalls`

See [LSP configuration](https://opencode.ai/docs/lsp).

### skill

Load a SKILL.md file and return its content in the conversation.

```jsonc
{ "permission": { "skill": "allow" } }
```

### todowrite / todoread

Manage todo lists during complex operations. Disabled for subagents by default.

```jsonc
{ "permission": { "todowrite": "allow", "todoread": "allow" } }
```

### webfetch

Fetch and read web pages. Useful for documentation lookup.

```jsonc
{ "permission": { "webfetch": "allow" } }
```

### question

Ask the user questions during execution with options.

```jsonc
{ "permission": { "question": "allow" } }
```

---

## Adding Custom Tools

There are **3 methods** to add custom tools to OpenCode:

### Method 1: Plugin Tools (Recommended)

**Pros**: Dynamic, can use external modules, full control
**Cons**: Requires JavaScript, runs in OpenCode process

**Location**: `.opencode/plugin/<name>.js`

```javascript
export const MyPlugin = async ({ $, directory }) => ({
  tool: {
    my_tool: {
      description: "My custom tool description",
      parameters: {
        type: "object",
        properties: {
          query: { type: "string", description: "The query input" }
        },
        required: ["query"]
      },
      execute: async (params, context) => {
        // Your tool logic here
        const result = await someOperation(params.query);
        return { result };
      }
    },
    another_tool: {
      description: "Another tool",
      parameters: { type: "object", properties: {} },
      execute: async () => ({ status: "done" })
    }
  }
});
```

**Permission control**:
```jsonc
{ "permission": { "my_tool": "ask" } }
```

### Method 2: Config-Based Custom Tools

**Pros**: Declarative, no code required
**Cons**: Limited to shell commands

**Location**: `.opencode/config.jsonc`

```jsonc
{
  "tools": {
    "deploy": {
      "description": "Deploy to production",
      "command": "./scripts/deploy.sh",
      "args": ["$environment"]
    },
    "lint_fix": {
      "description": "Run linter with auto-fix",
      "command": "npm",
      "args": ["run", "lint:fix"]
    }
  }
}
```

See [custom tools docs](https://opencode.ai/docs/custom-tools).

### Method 3: MCP Servers

**Pros**: Standard protocol, reusable across agents, external process isolation  
**Cons**: Requires MCP server setup, may add latency

**When to use MCP**: For **external services** that OpenCode doesn't have built-in access to.

| Use Case | MCP Needed? | Reason |
|----------|-------------|--------|
| Read/write files | ❌ No | Built-in: `read`, `write`, `edit` |
| Search codebase | ❌ No | Built-in: `grep`, `glob`, `list` |
| Run shell commands | ❌ No | Built-in: `bash` |
| **Query database** | ✅ Yes | No built-in database access |
| **GitHub API** | ✅ Yes | Richer than `bash` + `curl` |
| **Memory/RAG** | ✅ Yes | Persistent knowledge base |

**Location**: `.opencode/config.jsonc`

```jsonc
{
  "mcp": {
    "servers": {
      // GitHub API - richer than shell + curl
      "github": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-github"],
        "env": { "GITHUB_TOKEN": "${GITHUB_TOKEN}" }
      },
      
      // Database access - no built-in equivalent
      "postgres": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-postgres"],
        "env": { "DATABASE_URL": "${DATABASE_URL}" }
      },
      
      // Persistent memory - knowledge base
      "memory": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-memory"]
      }
    }
  }
}
```

**Permission control** (use server name prefix):
```jsonc
{ "permission": { "github_*": "allow", "postgres_*": "ask" } }
```

See [customization.md#mcp-servers](customization.md#mcp-servers) for how OpenCode MCP config differs from other runtimes.

---

## Comparison of Custom Tool Methods

| Aspect | Plugin Tools | Config Tools | MCP Servers |
|--------|--------------|--------------|-------------|
| **Setup** | JS file in plugin folder | JSON in config | External process |
| **Language** | JavaScript/TypeScript | Shell commands | Any (separate process) |
| **Isolation** | Same process | Subprocess | Separate process |
| **Reusable** | Per-project | Per-project | Cross-agent |
| **Parameters** | Full JSON schema | Simple args | MCP protocol |
| **Async** | ✅ Native | ❌ | ✅ Native |
| **State** | ✅ In-memory | ❌ | ✅ Server-side |
| **Best For** | Complex logic, API calls | Simple scripts | External services, DBs |

---

## Permission Wildcards

Control multiple tools at once:

```jsonc
{
  "permission": {
    "*": "ask",           // Default for all tools
    "read": "allow",      // Override for read
    "mymcp_*": "ask",     // All tools from "mymcp" MCP server
    "edit": "deny"        // Deny all file edits
  }
}
```

---

## Source Code References

| Component | Location |
|-----------|----------|
| Tool definitions | [/src/tool](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/tool) |
| Bash tool | [/src/tool/bash](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/tool/bash) |
| Edit tools | [/src/tool/edit](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/tool/edit) |
| MCP integration | [/src/mcp](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/mcp) |

---

## Further Reading

| Topic | Document |
|-------|----------|
| **Configuration** | [configuration.md](configuration.md) |
| **Plugins** | [integration-guide.md#plugins](integration-guide.md#plugins) |
| **Events** | [events.md](events.md) |
| **MCP Protocol** | [modelcontextprotocol.io](https://modelcontextprotocol.io/) |
