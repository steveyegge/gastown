# Example Agent Configurations

This document provides example `agents.json` configurations for common LLM CLI tools that aren't built into Gas Town.

## Basic Example

Create `settings/agents.json` in your town root or rig directory:

```json
{
  "version": 1,
  "agents": {
    "my-custom-agent": {
      "command": "my-agent-cli",
      "args": ["--autonomous"],
      "process_names": ["my-agent-cli"],
      "resume_flag": "--resume",
      "resume_style": "flag"
    }
  }
}
```

## Aider Configuration

[Aider](https://aider.chat) is a popular AI pair programming tool.

```json
{
  "version": 1,
  "agents": {
    "aider": {
      "command": "aider",
      "args": [
        "--yes",
        "--no-auto-commits",
        "--no-suggest-shell-commands"
      ],
      "process_names": ["aider", "python"],
      "resume_flag": "--restore-chat-history",
      "resume_style": "flag",
      "non_interactive": {
        "prompt_flag": "--message",
        "output_flag": ""
      }
    }
  }
}
```

**Notes:**
- `--yes` enables autonomous mode (auto-accepts suggestions)
- `--no-auto-commits` prevents automatic git commits
- Process names include `python` since Aider runs as a Python script

## OpenHands Configuration

[OpenHands](https://github.com/All-Hands-AI/OpenHands) (formerly OpenDevin) is an autonomous AI agent.

```json
{
  "version": 1,
  "agents": {
    "openhands": {
      "command": "openhands",
      "args": ["--no-confirm"],
      "process_names": ["openhands", "python"],
      "session_id_env": "OPENHANDS_SESSION_ID",
      "resume_flag": "--session",
      "resume_style": "flag"
    }
  }
}
```

## Devon Configuration

[Devon](https://github.com/entropy-research/Devon) is an open-source AI software engineer.

```json
{
  "version": 1,
  "agents": {
    "devon": {
      "command": "devon",
      "args": ["--headless", "--auto-approve"],
      "process_names": ["devon"],
      "resume_flag": "--continue",
      "resume_style": "flag"
    }
  }
}
```

## SWE-agent Configuration

[SWE-agent](https://github.com/princeton-nlp/SWE-agent) from Princeton NLP.

```json
{
  "version": 1,
  "agents": {
    "swe-agent": {
      "command": "swe-agent",
      "args": ["--model", "gpt-4", "--config", "default"],
      "process_names": ["swe-agent", "python"],
      "non_interactive": {
        "prompt_flag": "--problem-statement",
        "output_flag": "--output-format json"
      }
    }
  }
}
```

## Multi-Agent Configuration

You can define multiple agents in a single file:

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
    },
    "devon": {
      "command": "devon",
      "args": ["--headless", "--auto-approve"],
      "process_names": ["devon"],
      "resume_flag": "--continue",
      "resume_style": "flag"
    },
    "custom-internal": {
      "command": "/opt/company/ai-assistant",
      "args": ["--mode", "autonomous", "--trust"],
      "process_names": ["ai-assistant"],
      "session_id_env": "AI_ASSISTANT_SESSION",
      "resume_flag": "--resume",
      "resume_style": "flag"
    }
  }
}
```

## Overriding Built-in Defaults

Override Claude's default settings for a specific rig:

```json
{
  "version": 1,
  "agents": {
    "claude": {
      "command": "claude",
      "args": [
        "--dangerously-skip-permissions",
        "--max-tokens", "100000",
        "--model", "claude-3-opus-20240229"
      ]
    }
  }
}
```

## Using Different Agent Versions

Point to specific binary versions:

```json
{
  "version": 1,
  "agents": {
    "claude-beta": {
      "command": "/opt/claude/beta/claude",
      "args": ["--dangerously-skip-permissions"],
      "process_names": ["claude", "node"],
      "session_id_env": "CLAUDE_SESSION_ID",
      "resume_flag": "--resume",
      "resume_style": "flag",
      "supports_hooks": true,
      "supports_fork_session": true
    },
    "gemini-nightly": {
      "command": "/opt/gemini/nightly/gemini",
      "args": ["--approval-mode", "yolo"],
      "process_names": ["gemini"],
      "session_id_env": "GEMINI_SESSION_ID",
      "resume_flag": "--resume",
      "resume_style": "flag"
    }
  }
}
```

## Agent with Full Non-Interactive Support

Complete configuration with all non-interactive options:

```json
{
  "version": 1,
  "agents": {
    "full-featured": {
      "command": "ai-agent",
      "args": ["--autonomous", "--no-confirm"],
      "process_names": ["ai-agent"],
      "session_id_env": "AI_AGENT_SESSION",
      "resume_flag": "--resume",
      "resume_style": "flag",
      "supports_hooks": true,
      "supports_fork_session": false,
      "non_interactive": {
        "subcommand": "exec",
        "prompt_flag": "-p",
        "output_flag": "--format json"
      }
    }
  }
}
```

## Validating Configuration

Validate your configuration with:

```bash
# Check JSON syntax
jq . settings/agents.json

# Test agent is recognized
gt config show | grep agent

# Run doctor to check for issues
gt doctor
```

## See Also

- [Custom Agent Configuration Guide](../custom-agents.md)
- [Configuration Reference](../reference.md)
