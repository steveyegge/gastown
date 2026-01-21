# Mol Mall Design

> A marketplace for Gas Town formulas

## Vision

**Mol Mall** is a registry for sharing formulas across Gas Town installations. Think npm for molecules, or Terraform Registry for workflows.

```
"Cook a formula, sling it to a polecat, the witness watches, refinery merges."

What if you could browse a mall of formulas, install one, and immediately
have your polecats executing world-class workflows?
```

### The Network Effect

A well-designed formula for "code review" or "security audit" or "deploy to K8s" can spread across thousands of Gas Town installations. Each adoption means:
- More agents executing proven workflows
- More structured, trackable work output
- Better capability routing (agents with track records on a formula get similar work)

## Architecture

### Registry Types

```
┌─────────────────────────────────────────────────────────────────┐
│                      MOL MALL REGISTRIES                         │
└─────────────────────────────────────────────────────────────────┘

PUBLIC REGISTRY (molmall.gastown.io)
├── Community formulas (MIT licensed)
├── Official Gas Town formulas (blessed)
├── Verified publisher formulas
└── Open contribution model

PRIVATE REGISTRY (self-hosted)
├── Organization-specific formulas
├── Proprietary workflows
├── Internal deployment patterns
└── Enterprise compliance formulas

FEDERATED REGISTRY (HOP future)
├── Cross-organization discovery
├── Skill-based search
└── Attribution chain tracking
└── hop:// URI resolution
```

### URI Scheme

```
hop://molmall.gastown.io/formulas/mol-polecat-work@4.0.0
       └──────────────────┘         └──────────────┘ └───┘
           registry host              formula name   version

# Short forms
mol-polecat-work                    # Default registry, latest version
mol-polecat-work@4                  # Major version
mol-polecat-work@4.0.0              # Exact version
@acme/mol-deploy                    # Scoped to publisher
hop://acme.corp/formulas/mol-deploy # Full HOP URI
```

### Registry API

```yaml
# OpenAPI-style specification

GET /formulas
  # List all formulas
  Query:
    - q: string          # Search query
    - capabilities: string[]   # Filter by capability tags
    - author: string     # Filter by author
    - limit: int
    - offset: int
  Response:
    formulas:
      - name: mol-polecat-work
        version: 4.0.0
        description: "Full polecat work lifecycle..."
        author: steve@gastown.io
        downloads: 12543
        capabilities: [go, testing, code-review]

GET /formulas/{name}
  # Get formula metadata
  Response:
    name: mol-polecat-work
    versions: [4.0.0, 3.2.1, 3.2.0, ...]
    latest: 4.0.0
    author: steve@gastown.io
    repository: https://github.com/steveyegge/gastown
    license: MIT
    capabilities:
      primary: [go, testing]
      secondary: [git, code-review]
    stats:
      downloads: 12543
      stars: 234
      used_by: 89  # towns using this formula

GET /formulas/{name}/{version}
  # Get specific version
  Response:
    name: mol-polecat-work
    version: 4.0.0
    checksum: sha256:abc123...
    signature: <optional PGP signature>
    content: <base64 or URL to .formula.toml>
    changelog: "Added self-cleaning model..."
    published_at: 2026-01-10T00:00:00Z

POST /formulas
  # Publish formula (authenticated)
  Body:
    name: mol-my-workflow
    version: 1.0.0
    content: <formula TOML>
    changelog: "Initial release"
  Auth: Bearer token (linked to HOP identity)

GET /formulas/{name}/{version}/download
  # Download formula content
  Response: raw .formula.toml content
```

## Formula Package Format

### Simple Case: Single File

Most formulas are single `.formula.toml` files:

```bash
gt formula install mol-polecat-code-review
# Downloads mol-polecat-code-review.formula.toml to ~/gt/.beads/formulas/
```

### Complex Case: Formula Bundle

Some formulas need supporting files (scripts, templates, configs):

```
mol-deploy-k8s.formula.bundle/
├── formula.toml              # Main formula
├── templates/
│   ├── deployment.yaml.tmpl
│   └── service.yaml.tmpl
├── scripts/
│   └── healthcheck.sh
└── README.md
```

Bundle format:
```bash
# Bundles are tarballs
mol-deploy-k8s-1.0.0.bundle.tar.gz
```

Installation:
```bash
gt formula install mol-deploy-k8s
# Extracts to ~/gt/.beads/formulas/mol-deploy-k8s/
# formula.toml is at mol-deploy-k8s/formula.toml
```

## Installation Flow

### Basic Install

```bash
$ gt formula install mol-polecat-code-review

Resolving mol-polecat-code-review...
  Registry: molmall.gastown.io
  Version:  1.2.0 (latest)
  Author:   steve@gastown.io
  Skills:   code-review, security

Downloading... ████████████████████ 100%
Verifying checksum... ✓

Installed to: ~/gt/.beads/formulas/mol-polecat-code-review.formula.toml
```

### Version Pinning

```bash
$ gt formula install mol-polecat-work@4.0.0

Installing mol-polecat-work@4.0.0 (pinned)...
✓ Installed

$ gt formula list --installed
  mol-polecat-work           4.0.0   [pinned]
  mol-polecat-code-review    1.2.0   [latest]
```

### Upgrade Flow

```bash
$ gt formula upgrade mol-polecat-code-review

Checking for updates...
  Current: 1.2.0
  Latest:  1.3.0

Changelog for 1.3.0:
  - Added security focus option
  - Improved test coverage step

Upgrade? [y/N] y

Downloading... ✓
Installed: mol-polecat-code-review@1.3.0
```

### Lock File

```json
// ~/gt/.beads/formulas/.lock.json
{
  "version": 1,
  "formulas": {
    "mol-polecat-work": {
      "version": "4.0.0",
      "pinned": true,
      "checksum": "sha256:abc123...",
      "installed_at": "2026-01-10T00:00:00Z",
      "source": "hop://molmall.gastown.io/formulas/mol-polecat-work@4.0.0"
    },
    "mol-polecat-code-review": {
      "version": "1.3.0",
      "pinned": false,
      "checksum": "sha256:def456...",
      "installed_at": "2026-01-10T12:00:00Z",
      "source": "hop://molmall.gastown.io/formulas/mol-polecat-code-review@1.3.0"
    }
  }
}
```

## Publishing Flow

### First-Time Setup

```bash
$ gt formula publish --init

Setting up Mol Mall publishing...

1. Create account at https://molmall.gastown.io/signup
2. Generate API token at https://molmall.gastown.io/settings/tokens
3. Run: gt formula login

$ gt formula login
Token: ********
Logged in as: steve@gastown.io
```

### Publishing

```bash
$ gt formula publish mol-polecat-work

Publishing mol-polecat-work...

Pre-flight checks:
  ✓ formula.toml is valid
  ✓ Version 4.0.0 not yet published
  ✓ Required fields present (name, version, description)
  ✓ Skills declared

Publish to molmall.gastown.io? [y/N] y

Uploading... ✓
Published: hop://molmall.gastown.io/formulas/mol-polecat-work@4.0.0

View at: https://molmall.gastown.io/formulas/mol-polecat-work
```

### Verification Levels

```
┌─────────────────────────────────────────────────────────────────┐
│                    FORMULA TRUST LEVELS                          │
└─────────────────────────────────────────────────────────────────┘

UNVERIFIED (default)
  Anyone can publish
  Basic validation only
  Displayed with ⚠️ warning

VERIFIED PUBLISHER
  Publisher identity confirmed
  Displayed with ✓ checkmark
  Higher search ranking

OFFICIAL
  Maintained by Gas Town team
  Displayed with 🏛️ badge
  Included in embedded defaults

AUDITED
  Security review completed
  Displayed with 🔒 badge
  Required for enterprise registries
```

## Capability Tagging

### Formula Capability Declaration

```toml
[formula.capabilities]
# What capabilities does this formula exercise? Used for agent routing.
primary = ["go", "testing", "code-review"]
secondary = ["git", "ci-cd"]

# Capability weights (optional, for fine-grained routing)
[formula.capabilities.weights]
go = 0.3           # 30% of formula work is Go
testing = 0.4      # 40% is testing
code-review = 0.3  # 30% is code review
```

### Capability-Based Search

```bash
$ gt formula search --capabilities="security,go"

Formulas matching capabilities: security, go

  mol-security-audit           v2.1.0   ⭐ 4.8   📥 8,234
    Capabilities: security, go, code-review
    "Comprehensive security audit workflow"

  mol-dependency-scan          v1.0.0   ⭐ 4.2   📥 3,102
    Capabilities: security, go, supply-chain
    "Scan Go dependencies for vulnerabilities"
```

### Agent Accountability

When a polecat completes a formula, the execution is tracked:

```
Polecat: beads/amber
Formula: mol-polecat-code-review@1.3.0
Completed: 2026-01-10T15:30:00Z
Capabilities exercised:
  - code-review (primary)
  - security (secondary)
  - go (secondary)
```

This execution record enables:
1. **Routing** - Agents with successful track records get similar work
2. **Debugging** - Trace which agent did what, when
3. **Quality metrics** - Track success rates by agent and formula

## Private Registries

### Enterprise Deployment

```yaml
# ~/.gtconfig.yaml
registries:
  - name: acme
    url: https://molmall.acme.corp
    auth: token
    priority: 1  # Check first

  - name: public
    url: https://molmall.gastown.io
    auth: none
    priority: 2  # Fallback
```

### Self-Hosted Registry

```bash
# Docker deployment
docker run -d \
  -p 8080:8080 \
  -v /data/formulas:/formulas \
  -e AUTH_PROVIDER=oidc \
  gastown/molmall-registry:latest

# Configuration
MOLMALL_STORAGE=s3://bucket/formulas
MOLMALL_AUTH=oidc
MOLMALL_OIDC_ISSUER=https://auth.acme.corp
```

## Federation

Federation enables formula sharing across organizations using the Highway Operations Protocol (HOP).

### Cross-Registry Discovery

```bash
$ gt formula search "deploy kubernetes" --federated

Searching across federated registries...

  molmall.gastown.io:
    mol-deploy-k8s           v3.0.0   🏛️ Official

  molmall.acme.corp:
    @acme/mol-deploy-k8s     v2.1.0   ✓ Verified

  molmall.bigco.io:
    @bigco/k8s-workflow      v1.0.0   ⚠️ Unverified
```

### HOP URI Resolution

The `hop://` URI scheme provides cross-registry entity references:

```bash
# Full HOP URI
gt formula install hop://molmall.acme.corp/formulas/@acme/mol-deploy@2.1.0

# Resolution via HOP (Highway Operations Protocol)
1. Parse hop:// URI
2. Resolve registry endpoint (DNS/HOP discovery)
3. Authenticate (if required)
4. Download formula
5. Verify checksum/signature
6. Install to town-level
```

## Implementation Phases

### Phase 1: Local Commands (Now)

- `gt formula list` with tier display
- `gt formula show --resolve`
- Formula resolution order (project → town → system)

### Phase 2: Manual Sharing

- Formula export/import
- `gt formula export mol-polecat-work > mol-polecat-work.formula.toml`
- `gt formula import < mol-polecat-work.formula.toml`
- Lock file format

### Phase 3: Public Registry

- molmall.gastown.io launch
- `gt formula install` from registry
- `gt formula publish` flow
- Basic search and browse

### Phase 4: Enterprise Features

- Private registry support
- Authentication integration
- Verification levels
- Audit logging

### Phase 5: Federation (HOP)

- Capability tags in schema
- Federation protocol (Highway Operations Protocol)
- Cross-registry search
- Agent execution tracking for accountability

## Related Documents

- [Formula Resolution](formula-resolution.md) - Local resolution order
- [molecules.md](molecules.md) - Formula lifecycle (cook, pour, squash)
- [understanding-gas-town.md](../../../docs/understanding-gas-town.md) - Gas Town architecture
