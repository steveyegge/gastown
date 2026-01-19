# Agent Runtime Feature Comparison

This document compares feature support across different agent runtimes in Gastown.
Update this table when features are verified through testing or official documentation.

**Legend**: ✅ Supported | ⚠️ Partial/Workaround | ❌ Not supported | ❓ Unverified

---

## Quick Reference

### CLI Feature Matrix

| Feature | Claude Code | OpenCode | Codex | Gemini | Notes |
|---------|-------------|----------|-------|--------|-------|
| **Session Management** |
| Session start | ✅ | ✅ | ✅ | ✅ | |
| Session resume | ✅ `--resume` | ✅ `--session` | ✅ `resume` | ✅ `--resume` | Different flags/syntax |
| Session fork | ✅ `--fork-session` | ✅ SDK/API | ❌ | ❌ | OC requires messageID |
| Session export | ❌ | ✅ `opencode export` | ❌ | ❌ | OpenCode exports to JSON |
| **Hooks/Plugins** |
| Session start hook | ✅ `SessionStart` | ✅ `session.created` | ❌ | ✅ `SessionStart` | |
| User message hook | ✅ `UserPromptSubmit` | ✅ `message.updated` | ❌ | ⚠️ `AfterModel`? | |
| Pre-compact hook | ✅ `PreCompact` | ✅ `session.compacting` | ❌ | ❌ | |
| Stop/idle hook | ✅ `Stop` | ✅ `session.idle` | ❌ | ✅ `SessionEnd` | |
| **Execution Modes** |
| Interactive TUI | ✅ | ✅ | ✅ | ✅ | |
| Non-interactive | ✅ `-p` | ✅ `run` | ✅ `exec` | ✅ `-p` | |
| Server mode | ❌ | ✅ `opencode web` | ❌ | ❌ | Also `opencode acp` |
| **Auto-approval** |
| Skip permissions | ✅ `--dangerously-skip` | ✅ config | ✅ `--full-auto` | ✅ `--yolo` | |
| **Advanced** |
| MCP support | ✅ | ✅ | ✅ | ✅ | |
| MCP tool search | ✅ `MCPSearch` | ⚠️ ecosystem | ❌ | ❌ | Auto lazy load >10% context |
| Custom agents | ✅ `--agent` | ✅ skills | ❌ | ✅ extensions | |
| HTTP API | ❌ | ✅ | ❌ | ❌ | Full REST API |

---

## Gastown Integration

### Role Support Matrix

| Role | Min Requirements | Claude | OpenCode | Codex | Gemini |
|------|-----------------|--------|----------|-------|--------|
| **Mayor** | SessionStart + Resume | ✅ | ✅ | ⚠️ | ✅ |
| **Deacon** | SessionStart + Stop | ✅ | ✅ | ⚠️ | ✅ |
| **Witness** | SessionStart + PreCompact + Resume | ✅ | ✅ | ❌ | ⚠️ |
| **Polecat** | SessionStart + UserMessage + Resume | ✅ | ✅ | ⚠️ | ⚠️ |
| **Refinery** | SessionStart + Resume | ✅ | ✅ | ⚠️ | ✅ |
| **Crew** | Interactive + UserMessage | ✅ | ✅ | ⚠️ | ⚠️ |

### Hook → Command Mapping

| Claude Hook | OpenCode Event | Codex | Gemini | gt Command | Purpose |
|-------------|---------------|-------|--------|------------|---------|
| `SessionStart` | `session.created` | - | `SessionStart` | `gt prime` | Inject role context |
| `SessionStart` | `session.created` | - | `SessionStart` | `gt mail check --inject` | Mail (autonomous) |
| `UserPromptSubmit` | `message.updated` | - | `AfterModel`? | `gt mail check --inject` | Mail (interactive) |
| `PreCompact` | `session.compacting` | - | - | `gt prime` | Context recovery |
| `Stop` | `session.idle` | - | `SessionEnd` | `gt costs record` | Cost tracking |

### Degraded Functionality

**Codex**: No hooks. Workarounds: manual `gt prime`, env vars, `.codex/` files.

**Gemini**: Hooks via extensions. No PreCompact. UserMessage hook unclear.

---

## SDK Comparison

### Language Support

| Language | Claude Code | OpenCode | Codex | Gemini |
|----------|-------------|----------|-------|--------|
| **TypeScript** | ✅ `@anthropic-ai/claude-agent-sdk` | ✅ `@opencode-ai/sdk` | ✅ `@openai/codex-sdk` | ✅ `@google/gemini-cli-core` |
| **Python** | ✅ `anthropic-claude-agent` | ✅ `opencode` | ⚠️ unofficial | ✅ `google-genai` |
| **Go** | ⚠️ unofficial | ✅ `opencode-sdk-go` | ❌ | ❌ |

### SDK Feature Matrix

| Capability | Claude Code | OpenCode | Codex | Gemini | Notes |
|------------|-------------|----------|-------|--------|-------|
| **Session** |
| Create/Resume | ✅ | ✅ | ✅ | ✅ | |
| List sessions | ⚠️ limited | ✅ `session.list()` | ⚠️ local | ⚠️ | CC lacks list API |
| Fork session | ✅ `forkSession` | ✅ `session.fork()` | ❌ | ❌ | OC requires messageID |
| **Server** |
| Start server | ❌ | ✅ `createOpencode()` | ❌ | ❌ | OpenCode only |
| Connect | ❌ | ✅ `createOpencodeClient()` | ❌ | ❌ | |
| **Execution** |
| Shell/bash | ✅ built-in | ✅ `session.shell()` | ✅ sandboxed | ⚠️ | |
| Prompt/query | ✅ `query()` | ✅ `session.prompt()` | ✅ `run()` | ✅ | |
| Context inject | ✅ | ✅ plugin/SDK/API | ✅ | ⚠️ | OC: noReply, hooks, TUI |
| **Extensibility** |
| Skills (SKILL.md) | ✅ | ✅ | ✅ | ✅ exp | Open standard |
| Custom commands | ✅ `.claude/commands/` | ✅ `.opencode/commands/` | ✅ `~/.codex/prompts/` | ✅ `.gemini/commands/` | Markdown/.toml |
| Plugins | ✅ | ✅ | ⚠️ MCP | ✅ ext | |
| **Safety** |
| OS sandbox | ✅ `/sandbox` | ⚠️ Docker 3rd party | ✅ Seatbelt | ✅ Seatbelt | |
| Permission modes | ✅ settings | ✅ allow/ask/deny | ✅ suggest/auto | ✅ policy | Per-action control |
| Role-based agents | ✅ `--agent` | ✅ agents in config | ❌ | ✅ extensions | Custom permission profiles |
| **Events** |
| Subscribe | ⚠️ hooks | ✅ [20+ events](opencode/reference/events.md) | ⚠️ | ✅ hooks | |

---

## Reference

### Runtime Documentation

| Runtime | Official Docs | Gastown Docs |
|---------|---------------|--------------|
| Claude Code | [docs.anthropic.com](https://docs.anthropic.com/en/docs/claude-code/cli-reference) | Primary runtime |
| OpenCode | [opencode.ai/docs](https://opencode.ai/docs/) | [docs/opencode/](opencode/) |
| Codex | [openai.com/codex](https://openai.com/codex) | Limited support |
| Gemini | [geminicli.com](https://geminicli.com) | [Extensions](https://geminicli.com/extensions/hooks) |

### Updating This Document

When verifying a feature:

1. **Test locally** - Run the feature with the specific runtime
2. **Check official docs** - Verify against runtime documentation
3. **Search the web** - Look for recent updates, blog posts, changelogs
4. **Search GitHub** - Check source code for implementation details:
   - `site:github.com/anthropics/claude-code`
   - `site:github.com/anomalyco/opencode`
   - `site:github.com/openai/codex`
   - `site:github.com/google/gemini-cli`
5. **Cross-reference** - For each claim, ask "does this feature exist in the other runtimes?"
6. **Document findings** - Add to `opencode/archive/` with frontmatter for significant research

---

## Detailed Notes

### Session Fork Implementation

**Claude Code**: CLI flag with `--resume`
```bash
claude --resume abc123 --fork-session
```

**OpenCode**: SDK or HTTP API with messageID
```javascript
// SDK
await client.session.fork({ path: { id: sessionId }, body: { messageID: "msg_xyz" } })
```
```bash
# HTTP API
POST /session/{id}/fork
{ "messageID": "msg_xyz" }
```
Requires server mode (`opencode web` or `opencode acp`)

### Hooks vs Plugins vs Extensions

| Runtime | Terminology | Config Location | Format |
|---------|-------------|-----------------|--------|
| Claude Code | Hooks | `.claude/settings.json` | JSON array |
| OpenCode | Plugins | `.opencode/plugin/<name>.js` | JS handlers |
| Gemini | Extensions | `.gemini/settings.json` | JSON/JS |
| Codex | N/A | Env vars / files | N/A |

### Non-Interactive Execution

| Runtime | Command | Notes |
|---------|---------|-------|
| Claude Code | `claude -p "query"` | Native print mode |
| OpenCode | `opencode run "prompt"` | Subcommand |
| Codex | `codex exec "prompt"` | Sandbox by default |
| Gemini | `gemini -p "query"` | Print mode |
