# Gas Town — Agent Skills

Project-specific skills live in `.claude/skills/` and are available to any agent
working in this repository.

| Skill | Trigger phrases | File |
|---|---|---|
| `handoff` | `/handoff`, context full, session cycle | `.claude/skills/handoff/SKILL.md` |
| `deploy-ai-marketplace` | deploy ai-marketplace, azd up, provision infra, push to Azure | `.claude/skills/deploy-ai-marketplace/SKILL.md` |

## deploy-ai-marketplace

Full deployment runbook for `projects/ai-marketplace`.
Covers `azd up`, Cosmos DB provisioning (10 containers), Azure Functions Flex Consumption,
Container Apps (Next.js web), Key Vault secret management, local Cosmos Emulator setup,
and common error fixes.

See `.claude/skills/deploy-ai-marketplace/SKILL.md` for the complete guide.
