# Gastown Plugin Conversion Notes

This skill could be converted to an official Claude plugin. Here's how:

## Why a Plugin?

1. **MCP Server Integration** - gt/bd commands could be exposed as MCP tools
2. **Persistent State** - Plugin could maintain connection to beads database
3. **Cross-Session Context** - Plugin could automatically load hook state on startup
4. **Rich UI** - IDE integration for convoy dashboards, polecat monitoring

## Proposed MCP Server

```typescript
// gastown-mcp/src/index.ts
import { Server } from "@modelcontextprotocol/sdk/server/index.js";

const server = new Server({
  name: "gastown",
  version: "1.0.0"
});

// Core tools
server.tool("gt_hook", "Check current work on hook", async () => {
  return execSync("gt hook").toString();
});

server.tool("gt_sling", "Dispatch work to agent", async ({ issue, rig }) => {
  return execSync(`gt sling ${issue} --rig=${rig}`).toString();
});

server.tool("bd_list", "List issues", async ({ status }) => {
  const cmd = status ? `bd list --status=${status}` : "bd list";
  return execSync(cmd).toString();
});

server.tool("bd_show", "Show issue details", async ({ id }) => {
  return execSync(`bd show ${id}`).toString();
});

server.tool("gt_convoy_status", "Check convoy progress", async ({ convoy }) => {
  return execSync(`gt convoy status ${convoy}`).toString();
});

server.tool("gt_mail_inbox", "Check agent mail", async () => {
  return execSync("gt mail inbox").toString();
});

server.tool("tmux_polecat_output", "Get polecat output", async ({ rig, polecat, lines }) => {
  return execSync(`tmux capture-pane -t gt-${rig}-${polecat} -p | tail -${lines || 50}`).toString();
});
```

## Plugin Manifest

```json
{
  "name": "gastown",
  "version": "1.0.0",
  "description": "Multi-agent Claude Code orchestration",
  "author": "csellis",
  "mcpServers": {
    "gastown": {
      "command": "node",
      "args": ["~/.claude/plugins/gastown/dist/index.js"],
      "env": {
        "PATH": "~/go/bin:$PATH"
      }
    }
  },
  "skills": ["gastown"],
  "requiredBinaries": ["gt", "bd", "tmux"]
}
```

## Migration Path

1. **Phase 1**: Keep as skill (current)
   - Works today with bash commands
   - Documentation-driven guidance

2. **Phase 2**: Add MCP server alongside skill
   - Skill provides context/documentation
   - MCP tools provide structured command execution
   - Better error handling and output parsing

3. **Phase 3**: Full plugin
   - Package for distribution
   - Auto-install gt/bd binaries
   - IDE integration (VS Code extension)
   - Web dashboard for convoy monitoring

## Local Config Requirements

Plugin would need to handle:

```yaml
config:
  townRoot: ~/gt
  defaultRig: <your-rig>
  polecatNames:
    - <polecat-1>
    - <polecat-2>
  overseer:
    email: <your-email>
```

## Startup Hook

Plugin should implement startup hook for propulsion principle:

```typescript
// On session start
const hook = await execSync("gt hook").toString();
if (hook.trim()) {
  // Inject into context: "You have work on your hook: {hook}"
  // Trigger autonomous execution
}
```

## Dependencies

- Go binaries: `gt`, `bd` (installed at ~/go/bin/)
- tmux (for polecat interaction)
- git (for beads sync)
