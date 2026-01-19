# Custom Agent Configuration

Gas Town supports multiple LLM CLI tools through a flexible agent configuration system. This guide explains how to use built-in agent presets and create custom agent configurations.

## Overview

Gas Town can work with any LLM CLI tool that supports:
- Command-line invocation
- Autonomous/YOLO mode (non-interactive execution)
- Optional: Session resumption

Built-in presets are provided for popular LLM CLIs, and you can define custom agents for any tool.

## Built-in Agent Presets

The following agents are built-in and tested:

| Agent | Command | Description |
|-------|---------|-------------|
| `claude` | `claude` | Claude Code (default) |
| `gemini` | `gemini` | Gemini CLI |
| `codex` | `codex` | OpenAI Codex |
| `cursor` | `cursor-agent` | Cursor Agent |
| `auggie` | `auggie` | Auggie CLI |
| `amp` | `amp` | Sourcegraph AMP |
| `opencode` | `opencode` | OpenCode CLI |

### Using Built-in Agents

Set the agent in your rig or role configuration:

```yaml
# In rig config (settings/config.yaml)
runtime:
  agent: gemini
```

Or specify when starting a worker:

```bash
gt polecat start myworker --agent gemini
```

## Creating Custom Agents

Custom agents are defined in an `agents.json` file.

### Configuration Locations

- **Town-level**: `<town-root>/settings/agents.json`
- **Rig-level**: `<rig-path>/settings/agents.json`

Rig-level configurations are merged with town-level, allowing per-rig customization.

### Configuration Schema

```json
{
  "version": 1,
  "agents": {
    "my-agent": {
      "command": "my-agent-cli",
      "args": ["--autonomous", "--no-confirm"],
      "process_names": ["my-agent-cli"],
      "session_id_env": "MY_AGENT_SESSION_ID",
      "resume_flag": "--resume",
      "resume_style": "flag",
      "supports_hooks": false,
      "non_interactive": {
        "subcommand": "exec",
        "prompt_flag": "-p",
        "output_flag": "--json"
      }
    }
  }
}
```

### Field Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `command` | string | Yes | CLI binary to invoke |
| `args` | string[] | No | Default arguments for autonomous mode |
| `process_names` | string[] | No | Process names for detection (used by tmux) |
| `session_id_env` | string | No | Environment variable for session ID |
| `resume_flag` | string | No | Flag/subcommand for resuming sessions |
| `resume_style` | string | No | `"flag"` or `"subcommand"` |
| `supports_hooks` | bool | No | Whether agent supports Gas Town hooks |
| `supports_fork_session` | bool | No | Whether agent supports `--fork-session` |
| `non_interactive` | object | No | Settings for non-interactive execution |

### Non-Interactive Configuration

The `non_interactive` object configures how to run the agent for one-off commands:

| Field | Type | Description |
|-------|------|-------------|
| `subcommand` | string | Subcommand for non-interactive mode (e.g., `exec`) |
| `prompt_flag` | string | Flag for passing prompts (e.g., `-p`) |
| `output_flag` | string | Flag for structured output (e.g., `--json`) |

### Resume Styles

Gas Town supports two styles for resuming sessions:

**Flag style** (most common):
```bash
my-agent --resume session-123 --other-args
```

**Subcommand style** (used by Codex, AMP):
```bash
codex resume session-123 --yolo
amp threads continue thread-123 --args
```

## Example Configurations

### Adding Aider

```json
{
  "version": 1,
  "agents": {
    "aider": {
      "command": "aider",
      "args": ["--yes", "--no-auto-commits"],
      "process_names": ["aider", "python"],
      "resume_flag": "--restore-chat-history",
      "resume_style": "flag"
    }
  }
}
```

### Adding Continue.dev

```json
{
  "version": 1,
  "agents": {
    "continue": {
      "command": "continue",
      "args": ["--headless"],
      "process_names": ["continue"],
      "session_id_env": "CONTINUE_SESSION_ID",
      "resume_flag": "--session",
      "resume_style": "flag"
    }
  }
}
```

### Adding a Custom Internal Tool

```json
{
  "version": 1,
  "agents": {
    "internal-llm": {
      "command": "/opt/internal/llm-cli",
      "args": ["--mode", "autonomous", "--trust-all"],
      "process_names": ["llm-cli"],
      "session_id_env": "INTERNAL_LLM_SESSION",
      "resume_flag": "--continue-session",
      "resume_style": "flag",
      "non_interactive": {
        "prompt_flag": "--prompt",
        "output_flag": "--format json"
      }
    }
  }
}
```

## Overriding Built-in Presets

You can override built-in preset defaults by defining an agent with the same name:

```json
{
  "version": 1,
  "agents": {
    "claude": {
      "command": "/usr/local/bin/claude",
      "args": ["--dangerously-skip-permissions", "--custom-flag"]
    }
  }
}
```

This preserves the preset's other settings while overriding `command` and `args`.

## Agent Detection

Gas Town uses `process_names` to detect whether an agent is running in a tmux pane. This is used for:

- Status display in `gt agents list`
- Health checks in `gt agents check`
- Session management

For agents running as interpreters (like Python or Node.js), include both the interpreter and script name:

```json
{
  "process_names": ["node", "python", "my-agent"]
}
```

## Troubleshooting

### Agent not detected as running

1. Check `process_names` matches the actual process name shown in `ps aux`
2. Ensure the process is running in the expected tmux pane

### Session resume not working

1. Verify `resume_flag` and `resume_style` are correct for your agent
2. Check if the agent stores session IDs in the expected location
3. Test resume manually: `<command> <resume_flag> <session_id>`

### Custom agent not loading

1. Verify JSON syntax: `jq . settings/agents.json`
2. Check file permissions
3. Run `gt doctor` to diagnose configuration issues

## API Reference

For programmatic access to agent configuration:

```go
import "github.com/steveyegge/gastown/internal/config"

// Get a preset by name
info := config.GetAgentPresetByName("claude")

// List all available presets
presets := config.ListAgentPresets()

// Build a resume command
cmd := config.BuildResumeCommand("claude", sessionID)

// Check if agent supports resume
if config.SupportsSessionResume("my-agent") {
    // ...
}
```

## See Also

- [Architecture Overview](design/architecture.md)
- [Polecat Lifecycle](concepts/polecat-lifecycle.md)
- [Configuration Reference](reference.md)
