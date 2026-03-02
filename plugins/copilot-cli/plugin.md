+++
name = "copilot-cli"
description = "GitHub Copilot CLI integration hooks for Gas Town multi-agent orchestration"
version = 1

[gate]
type = "manual"

[execution]
timeout = "5m"
+++

# Copilot CLI Plugin

Provides GitHub Copilot CLI lifecycle hooks for Gas Town agents:

- **sessionStart**: Runs `gt prime --hook` to inject role context and mail
- **preToolUse**: Guard script enforcing PR workflow policy via `gt tap`

Hook templates are embedded in the `gt` binary (`internal/copilot/plugin/`) and
provisioned at agent startup by `EnsureHooksAt()`. This plugin directory serves
as documentation and metadata — the hooks themselves are compiled in.
