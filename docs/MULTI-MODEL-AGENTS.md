# Multi-Model Agents

Gastown supports multiple AI models through specialized agent wrappers. Each agent is optimized for different tasks and cost profiles.

## Requirements

- **curl** - For API requests (all agents)
- **jq** - For JSON parsing (all agents)
- **bc** - For cost calculations (usage tracking)
- **gemini** - Gemini CLI (for `gemini-agent`)
- **codex** - OpenAI Codex CLI, optional (for `gpt4o`, `gpt4o-mini`)

## Available Agents

| Agent | Model | Best For | Cost |
|-------|-------|----------|------|
| `gemini-agent` | Gemini Pro | Complex reasoning, long context | $1.25/$5.00 per 1M tokens |
| `gemini-flash` | Gemini Flash | Fast responses, high volume | $0.075/$0.30 per 1M tokens |
| `gpt4o` | GPT-4o | Documentation, code scaffolding | $2.50/$10.00 per 1M tokens |
| `gpt4o-mini` | GPT-4o Mini | Simple tasks, high volume | $0.15/$0.60 per 1M tokens |
| `perplexity` | Perplexity | Search-augmented queries | Free |
| `claude-tracked` | Claude (via CLI) | Complex coding tasks | $3.00/$15.00 per 1M tokens |

## Usage

All agents accept the same basic interface:

```bash
# Interactive mode
./agents/gpt4o

# With a prompt
./agents/gpt4o "Explain how this code works"

# Piped input
cat file.py | ./agents/gemini-flash "Review this code"
```

## Agent Selection Guide

### Use `gemini-flash` or `gpt4o-mini` for:
- Simple questions and explanations
- Boilerplate code generation
- High-volume, cost-sensitive tasks
- Quick formatting or refactoring

### Use `gemini-agent` or `gpt4o` for:
- Complex reasoning tasks
- Long document analysis
- Detailed code reviews
- Architecture decisions

### Use `perplexity` for:
- Questions requiring current information
- Research and fact-checking
- API/library documentation lookup
- When you need citations

### Use `claude-tracked` for:
- Complex multi-file coding tasks
- Nuanced code understanding
- Tasks requiring careful reasoning

## Environment Variables

Each agent requires its respective API key:

```bash
export GEMINI_API_KEY="your-key"      # For gemini-agent, gemini-flash
export OPENAI_API_KEY="your-key"      # For gpt4o, gpt4o-mini
export PERPLEXITY_API_KEY="your-key"  # For perplexity
# Claude uses the claude CLI which handles auth separately
```

## Usage Tracking

All agents automatically track token usage and costs. View reports with:

```bash
./bin/gt-usage           # Today's summary
./bin/gt-usage week      # Last 7 days
./bin/gt-usage summary   # One-line summary
```

See [GETTING-STARTED.md](../GETTING-STARTED.md#usage-tracking) for detailed usage tracking documentation.

## Integration with Gastown Workflows

Agents can be used in formulas and workflows:

```toml
# In a formula
[steps.research]
agent = "perplexity"
prompt = "Find documentation for ${library}"

[steps.implement]
agent = "gpt4o-mini"
prompt = "Generate implementation based on: ${steps.research.output}"
```

## Adding New Agents

To add a new model/provider:

1. Create a new agent script in `agents/`
2. Source the usage tracker: `source "$SCRIPT_DIR/lib/usage-tracker.sh"`
3. Call `track_usage` after each API call with token counts
4. Follow the existing agent scripts as templates

See [agents/gpt4o](../agents/gpt4o) for a complete example.
