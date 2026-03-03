# AI Asset Marketplace

> **Azure AI Asset Marketplace + Visual Agent Orchestration** — PRD v0.9

Enterprise-grade platform for discovering, deploying, and orchestrating AI agents, MCP servers, models, and workflow templates — built on Azure AI Foundry with a visual multi-agent canvas.

## Architecture

```
apps/
  web/          Next.js 15 frontend (marketplace catalog + React Flow orchestration canvas)
  api/          Azure Functions v4 API (Node.js/TypeScript)
infra/
  main.bicep          Root Bicep template (azd-compatible)
  modules/
    cosmos.bicep      Cosmos DB account + containers (serverless, hierarchical partition keys)
    functions.bicep   Azure Functions (Flex Consumption, Linux)
    appinsights.bicep Application Insights + Log Analytics
    keyvault.bicep    Azure Key Vault (RBAC-enabled)
.beads/
  issues.jsonl        PRD requirements as Gas Town beads (16 issues, 3 sprints)
  convoys.jsonl       Sprint convoy definitions
.gt/
  rig.toml            Gas Town rig configuration
azure.yaml            Azure Developer CLI (azd) configuration
```

## Sprints

| Sprint | Focus | Key Beads |
|--------|-------|-----------|
| Sprint 1 | Marketplace Catalog + Visual Orchestrator | mkt-00001 → mkt-00006 |
| Sprint 2 | Publisher Workflow + Governance | mkt-00007 → mkt-00011 |
| Sprint 3 | Azure Infrastructure + Deployment | mkt-00012 → mkt-00016 |

## Gas Town Setup (Linux/macOS/WSL)

```bash
# One-command setup
./gt-setup.sh ~/gt

# Then
cd ~/gt
gt mayor attach          # main AI coordinator
gt convoy list           # view sprints
gt sling mkt-00006       # assign visual canvas work to an agent
```

> **Windows**: Gas Town requires tmux. Run setup in WSL or a Linux dev container.

## Local Development

```bash
# Frontend
cd apps/web
npm install
npm run dev           # http://localhost:3000

# Backend (requires Azure Functions Core Tools)
cd apps/api
npm install
npm run build
func start            # http://localhost:7071

# Cosmos DB Emulator
# Download from https://aka.ms/cosmosdb-emulator
# Endpoint: https://localhost:8081  Key: in local.settings.json
```

## Azure Deployment

```bash
# Install Azure Developer CLI
winget install Microsoft.Azd   # Windows
brew tap azure/azd && brew install azd  # macOS

# Deploy
azd auth login
azd up   # provisions infra + deploys apps
```

## Key Pages

| Route | Description |
|-------|-------------|
| `/` | Home — featured assets, stats |
| `/marketplace` | Catalog — search, filter, compare all assets |
| `/marketplace/[id]` | Asset detail — versions, dependencies, evaluations |
| `/orchestrator` | Visual canvas — drag-drop multi-agent workflow builder |
| `/publish` | Publisher onboarding + asset submission |
| `/governance` | Review queue + audit trail |

## Related PRD

See [projects/prd/UHGAIMarketPlace.md](../prd/UHGAIMarketPlace.md) for full requirements.
