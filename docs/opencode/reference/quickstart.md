# OpenCode Quick Start

> **Status**: OpenCode is supported as an alternative to Claude Code  
> **Primary Runtime**: Claude Code (recommended for production)

## Source of Truth

- **Official Docs**: [opencode.ai/docs](https://opencode.ai/docs/)
- **GitHub**: [github.com/anomalyco/opencode](https://github.com/anomalyco/opencode)

---

## Installation

```bash
# Install OpenCode CLI
npm install -g opencode-ai

# Verify installation
opencode version
```

## Configuration in Gastown

### Set as Default Agent

```bash
gt config default-agent opencode
```

### Per-Role Configuration

```bash
# Override for specific role
gt mayor start --agent opencode

# Spawn polecat with OpenCode
gt sling issue-123 myproject --agent opencode
```

### Per-Rig Configuration

In `<rig>/settings/config.json`:
```json
{
  "agent": "opencode"
}
```

---

## Key Differences from Claude Code

| Feature | Claude Code | OpenCode |
|---------|-------------|----------|
| **Session Resume** | `--resume <id>` | `--session <id>` |
| **Non-Interactive** | `claude -p "query"` | `opencode run "query"` |
| **Hooks** | JSON settings | JS plugin |
| **Fork Session** | `--fork-session` CLI | SDK/API with messageID |
| **Server Mode** | ❌ | ✅ `opencode web` |

---

## Further Reading

| Topic | Document |
|-------|----------|
| **Runtime Comparison** | [agent-features.md](../../agent-features.md) |
| **Configuration** | [configuration.md](configuration.md) |
| **Integration Guide** | [integration-guide.md](integration-guide.md) |
| **Events Reference** | [events.md](events.md) |
| **External Resources** | [external-resources.md](external-resources.md) |
| **Maintenance** | [../design/maintenance.md](../design/maintenance.md) |

---

## Troubleshooting

### Plugin Not Loading

```bash
# Check if plugin installed
ls -la .opencode/plugin/gastown.js

# Reinstall via Gastown
gt prime
```

### Session Issues

```bash
# List sessions
opencode session list

# Resume specific session
opencode --session <session-id>
```

See [../design/maintenance.md](../design/maintenance.md) for more troubleshooting guidance.
