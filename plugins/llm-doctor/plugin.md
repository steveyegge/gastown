+++
name = "llm-doctor"
description = "Local LLM-assisted troubleshooting when Claude API is unreachable"
version = 1

[gate]
type = "cooldown"
duration = "5m"

[tracking]
labels = ["plugin:llm-doctor", "category:resilience"]
digest = true

[execution]
timeout = "2m"
notify_on_failure = true
severity = "high"
+++

# LLM Doctor

Monitors LLM API health and uses a local Ollama model to diagnose failures
when the remote API is unreachable. Escalates to the Overseer with a
structured diagnosis.

## How It Works

1. Probe the configured LLM provider (Anthropic direct, Bedrock, Vertex)
2. If healthy: exit quietly
3. If unhealthy: gather diagnostics (DNS, network, key validity, error codes)
4. Feed diagnostics to local Ollama model for classification and suggested fix
5. Escalate via `gt escalate` + mail to Overseer

Falls back to shell-only diagnosis if Ollama is not available.

## Requirements

- Ollama installed and running (`brew install ollama && ollama serve`)
- No Ollama? Plugin still works — just produces shell-based diagnosis instead

## Model Discovery

The plugin auto-discovers the best available Ollama model via `resolve-model.sh`:

1. If `LLM_DOCTOR_OLLAMA_MODEL` is set, use that (explicit override)
2. Walk a preference list (smallest first): llama3.2:1b, phi4-mini:3.8b,
   llama3.2:3b, llama3.1:8b, qwen2.5:7b, gemma2:9b, qwen2.5:32b
3. If no preferred model is pulled, use whatever IS pulled
4. If nothing is pulled, auto-pull the smallest preferred model (llama3.2:1b)
5. Override preference list: `LLM_DOCTOR_MODEL_PREFS="model1:model2:model3"`

The doctor only needs basic classification — a 1B model suffices.

## Relationship to rate-limit-watchdog

The rate-limit-watchdog handles 429s with estop/thaw. This plugin handles
everything else: network failures, auth errors, API outages, DNS issues.
It does NOT duplicate rate-limit handling — it defers to the watchdog for 429s.

## Testing

```bash
./test.sh              # Run full test suite (mocked, no real API calls)
./test.sh --verbose    # Verbose output
./run.sh --dry-run --force   # Live test (probes real API, no escalation)
```
