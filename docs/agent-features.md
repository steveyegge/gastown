# Agent Runtime Feature Comparison

This table documents feature support across different agent runtimes in Gastown.

> **Authoritative Source**: This table should be updated when features are verified. 
> For OpenCode-specific details, see [opencode/](opencode/).

## Feature Matrix

| Feature | Claude Code | OpenCode | Codex | Gemini | Notes |
|---------|-------------|----------|-------|--------|-------|
| **Session Management** |
| Session start | ✅ | ✅ | ✅ | ✅ | All support basic sessions |
| Session resume | ✅ `--resume` | ✅ `--session` | ✅ `resume` | ✅ | Different flags |
| Session fork | ✅ CLI | ✅ HTTP API | ❌ | ❌ | [OpenCode fork](opencode/reference/api-reference.md#sessions) |
| Session export | ❌ | ✅ | ❌ | ❌ | OpenCode can export to JSON |
| **Hooks/Plugins** |
| Session start hook | ✅ | ✅ | ❌ | ✅ | [OpenCode plugins](opencode/reference/plugin-guide.md) |
| User message hook | ✅ | ✅ | ❌ | ❌ | |
| Pre-compact hook | ✅ | ✅ | ❌ | ❌ | |
| Stop/idle hook | ✅ | ✅ | ❌ | ❌ | |
| **Execution Modes** |
| Interactive TUI | ✅ | ✅ | ✅ | ✅ | |
| Non-interactive | ✅ native | ✅ `run` | ✅ `exec` | ✅ `-p` | |
| Server mode | ❌ | ✅ | ❌ | ❌ | [OpenCode server](opencode/reference/api-reference.md#server--http-api) |
| **Auto-approval** |
| Skip permissions | ✅ `--dangerously-skip` | ✅ config | ✅ `--yolo` | ✅ `--approval-mode yolo` | |
| **Gastown Integration** |
| Process detection | ✅ | ✅ | ✅ | ✅ | Via `ProcessNames` config |
| All 6 roles | ✅ | ✅ | ⚠️ partial | ⚠️ partial | Mayor, Deacon, Witness, etc. |
| Work assignment | ✅ | ✅ | ⚠️ | ⚠️ | Via Beads mailbox |

## Legend

- ✅ Supported and tested
- ⚠️ Partial or untested
- ❌ Not supported
- `-` Not applicable

## Updating This Table

When verifying a feature:
1. Test the feature with the specific runtime
2. Update this table with the result
3. Add a link to relevant documentation if available
4. Document the test in `opencode/archive/` if it's a significant finding

## Runtime-Specific Documentation

| Runtime | Documentation |
|---------|---------------|
| OpenCode | [docs/opencode/](opencode/) |
| Claude Code | Primary runtime, see main docs |
| Codex | Limited documentation |
| Gemini | Limited documentation |

## Known Differences

### Session Fork

- **Claude Code**: Uses `--fork-session` CLI flag
- **OpenCode**: Uses HTTP API `POST /session/:id/fork` with `{ messageID: "..." }`
- Both achieve the same result; OpenCode's approach is more flexible for programmatic use

### Hooks vs Plugins

- **Claude Code**: Uses `.claude/settings.json` with hook definitions
- **OpenCode**: Uses `.opencode/plugin/<name>.js` with event handlers
- Both are installed by Gastown's `EnsureSettingsForRole()` function
