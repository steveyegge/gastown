# Chain Constitution Parameter Questionnaire

**Task**: w-hop-002
**Date**: 2026-03-04
**Author**: nullpriest (via Claude)
**Directive**: G2 (Brendan)

## Overview

This questionnaire defines every parameter needed to configure a new Wasteland chain. Answers populate the chain constitution stored in `_meta` and govern chain behavior.

---

## Section 1: Identity

### 1.1 Chain Name
**Parameter**: `chain.name`
**Type**: String (3-64 chars, alphanumeric + hyphens)

> What is the human-readable name of your chain?
>
> Examples: `wl-commons`, `gastown-dev`, `acme-bounties`

### 1.2 Chain Description
**Parameter**: `chain.description`
**Type**: Text (max 500 chars)

> Describe the purpose of this chain in 1-2 sentences.

### 1.3 Chain Type
**Parameter**: `chain.type`
**Type**: Enum

> What type of chain is this?
>
> - [ ] `public` — Open to anyone, fully transparent
> - [ ] `consortium` — Limited membership, shared governance
> - [ ] `private` — Single organization, internal use
> - [ ] `sandbox` — Testing/development only

### 1.4 Parent Chain (Optional)
**Parameter**: `chain.parent`
**Type**: URI or null

> Is this chain a fork of an existing chain?
>
> If yes, provide the parent chain URI: `hop://org/chain`

---

## Section 2: Membership & Registration

### 2.1 Registration Mode
**Parameter**: `membership.registration`
**Type**: Enum

> How can new rigs join?
>
> - [ ] `open` — Anyone can register
> - [ ] `invite` — Existing members must invite
> - [ ] `application` — Submit application for review
> - [ ] `closed` — No new registrations

### 2.2 Initial Trust Level
**Parameter**: `membership.initial_trust`
**Type**: Integer (0-4)

> What trust level do new rigs start at?
>
> - [ ] `0` — Outsider (read-only)
> - [ ] `1` — Registered (can claim/complete)
> - [ ] `2` — Contributor (recognized)

### 2.3 Required Rig Fields
**Parameter**: `membership.required_fields`
**Type**: Array of strings

> Which fields are required for registration?
>
> - [ ] `handle` (always required)
> - [ ] `display_name`
> - [ ] `email`
> - [ ] `dolthub_org`
> - [ ] `verification_proof`

### 2.4 Rig Types Allowed
**Parameter**: `membership.rig_types`
**Type**: Array of enums

> What types of rigs are allowed?
>
> - [ ] `human` — Individual humans
> - [ ] `agent` — AI agents
> - [ ] `org` — Organizations
> - [ ] `team` — Teams within organizations

---

## Section 3: Trust & Governance

### 3.1 Trust Tier Names
**Parameter**: `trust.tier_names`
**Type**: Object

> Customize the names for each trust tier:
>
> | Level | Default Name | Your Name |
> |-------|--------------|-----------|
> | 0 | Outsider | _________ |
> | 1 | Drifter | _________ |
> | 2 | Road Warrior | _________ |
> | 3 | War Chief | _________ |
> | 4 | Immortan | _________ |

### 3.2 Trust Escalation Method
**Parameter**: `trust.escalation`
**Type**: Enum

> How are trust levels increased?
>
> - [ ] `automatic` — Based on stamp accumulation
> - [ ] `vote` — Existing tier members vote
> - [ ] `admin` — Chain admin promotes manually
> - [ ] `hybrid` — Automatic with admin override

### 3.3 Automatic Escalation Thresholds
**Parameter**: `trust.auto_thresholds`
**Type**: Object (if escalation = automatic/hybrid)

> Configure automatic promotion thresholds:
>
> | Promotion | Stamps Required | Avg Quality | Time in Tier |
> |-----------|-----------------|-------------|--------------|
> | 1 → 2 | _____ | ≥ _____ | _____ days |
> | 2 → 3 | _____ | ≥ _____ | _____ days |
> | 3 → 4 | _____ | ≥ _____ | _____ days |

### 3.4 Validator Minimum Trust
**Parameter**: `trust.validator_min`
**Type**: Integer (0-4)

> What minimum trust level can issue stamps?
>
> Default: `3` (War Chief)

### 3.5 Governance Model
**Parameter**: `governance.model`
**Type**: Enum

> How are chain-level decisions made?
>
> - [ ] `dictator` — Single admin decides
> - [ ] `council` — Trust level 4 members vote
> - [ ] `democratic` — All registered rigs vote
> - [ ] `stake-weighted` — Votes weighted by reputation

---

## Section 4: Work & Compensation

### 4.1 Currency Type
**Parameter**: `currency.type`
**Type**: Enum

> What currency is used for compensation?
>
> - [ ] `none` — Reputation only, no compensation
> - [ ] `token` — Blockchain token
> - [ ] `fiat` — Traditional currency
> - [ ] `points` — Internal points system

### 4.2 Token Configuration (if currency.type = token)
**Parameter**: `currency.token`
**Type**: Object

> Token details:
>
> - Chain: [ ] Solana [ ] Ethereum [ ] Base [ ] Other: _____
> - Contract/Mint Address: _________________________
> - Symbol: _____
> - Decimals: _____
> - Treasury Address: _________________________

### 4.3 Compensation Distribution
**Parameter**: `currency.distribution`
**Type**: Enum

> When is compensation distributed?
>
> - [ ] `per_stamp` — On each stamp received
> - [ ] `per_completion` — On completion validation
> - [ ] `periodic` — Weekly/monthly distribution
> - [ ] `manual` — Admin distributes manually

### 4.4 Compensation Formula (if automatic)
**Parameter**: `currency.formula`
**Type**: Expression

> How is compensation calculated?
>
> Variables available:
> - `stamp.quality` — Quality dimension (1-5)
> - `stamp.reliability` — Reliability dimension (1-5)
> - `wanted.effort` — Effort level multiplier
> - `wanted.priority` — Priority multiplier
>
> Example: `base_rate * wanted.effort * avg(stamp.quality, stamp.reliability)`
>
> Your formula: _________________________

### 4.5 Effort Level Multipliers
**Parameter**: `work.effort_multipliers`
**Type**: Object

> Set multipliers for effort levels:
>
> | Effort | Multiplier |
> |--------|------------|
> | trivial | _____ (default: 0.25) |
> | small | _____ (default: 0.5) |
> | medium | _____ (default: 1.0) |
> | large | _____ (default: 2.0) |
> | epic | _____ (default: 4.0) |

### 4.6 Sandbox Requirements
**Parameter**: `work.sandbox`
**Type**: Object

> Configure sandboxed work:
>
> - Allow sandboxed tasks: [ ] Yes [ ] No
> - Minimum trust for sandbox work: _____
> - Sandbox provider: [ ] Firecracker [ ] gVisor [ ] Wasm [ ] None

---

## Section 5: Stamps & Reputation

### 5.1 Stamp Dimensions
**Parameter**: `stamps.dimensions`
**Type**: Array of objects

> Which reputation dimensions are tracked?
>
> - [ ] `quality` — Work quality (accuracy, completeness)
> - [ ] `reliability` — On-time delivery, commitments
> - [ ] `communication` — Updates, documentation
> - [ ] `expertise` — Domain knowledge
> - [ ] `collaboration` — Team interaction
> - [ ] Custom: _____ (description: _____)

### 5.2 Stamp Confidence Decay
**Parameter**: `stamps.decay`
**Type**: Object

> How does stamp confidence decay over time?
>
> - Enable decay: [ ] Yes [ ] No
> - Half-life (days): _____ (default: 90)
> - Minimum confidence: _____ (default: 0.1)

### 5.3 Self-Stamp Prevention
**Parameter**: `stamps.self_stamp`
**Type**: Enum

> How strictly is self-stamping prevented?
>
> - [ ] `strict` — No stamps within same org
> - [ ] `moderate` — No direct self-stamps only
> - [ ] `relaxed` — Allow with disclosure

### 5.4 Stamp Disclosure
**Parameter**: `stamps.disclosure`
**Type**: Enum

> What stamp information is public?
>
> - [ ] `full` — All dimensions visible
> - [ ] `aggregate` — Only averages visible
> - [ ] `existence` — Only stamp existence visible
> - [ ] `private` — Only visible to subject

---

## Section 6: Federation

### 6.1 Federation Mode
**Parameter**: `federation.mode`
**Type**: Enum

> How does this chain federate?
>
> - [ ] `isolated` — No federation
> - [ ] `read` — Can read other chains
> - [ ] `write` — Can write to other chains
> - [ ] `full` — Bidirectional federation

### 6.2 Trusted Chains
**Parameter**: `federation.trusted`
**Type**: Array of URIs

> Which chains are trusted for cross-chain stamps?
>
> - `hop://hop/wl-commons`
> - `hop://_____/_____`
> - `hop://_____/_____`

### 6.3 Cross-Chain Trust Discount
**Parameter**: `federation.trust_discount`
**Type**: Float (0-1)

> How much are cross-chain stamps discounted?
>
> - `1.0` — Full weight
> - `0.5` — Half weight
> - `0.0` — Ignored
>
> Your value: _____

### 6.4 Data Sovereignty
**Parameter**: `federation.sovereignty`
**Type**: Enum

> What data sovereignty tier?
>
> - [ ] `T1` — No restrictions
> - [ ] `T2` — Organization-level compliance
> - [ ] `T3` — Pseudonymous
> - [ ] `T4` — Anonymous

---

## Section 7: Operations

### 7.1 PR-Based Contributions
**Parameter**: `operations.pr_mode`
**Type**: Boolean

> Require PRs for contributions?
>
> - [ ] Yes — All changes via DoltHub PRs
> - [ ] No — Direct writes allowed (wild west)

### 7.2 PR Approval Requirements
**Parameter**: `operations.pr_approvals`
**Type**: Object (if pr_mode = true)

> PR approval configuration:
>
> - Minimum approvals: _____
> - Approver minimum trust: _____
> - Auto-merge on approval: [ ] Yes [ ] No

### 7.3 Commit Signing
**Parameter**: `operations.signing`
**Type**: Enum

> Require signed commits?
>
> - [ ] `none` — No signing required
> - [ ] `optional` — Signing encouraged
> - [ ] `required` — All commits must be signed

### 7.4 Audit Log Retention
**Parameter**: `operations.audit_retention`
**Type**: Duration

> How long to retain audit logs?
>
> - [ ] Forever (Dolt history is immutable)
> - [ ] _____ days (prune old branches)

---

## Section 8: Exit & Migration

### 8.1 Exit Rights
**Parameter**: `exit.rights`
**Type**: Enum

> What can rigs take when they leave?
>
> - [ ] `full` — Complete stamp history
> - [ ] `summary` — Aggregate scores only
> - [ ] `none` — Nothing portable

### 8.2 Data Export
**Parameter**: `exit.export`
**Type**: Enum

> Export format for departing rigs:
>
> - [ ] `dolt` — Full Dolt clone
> - [ ] `json` — JSON export
> - [ ] `csv` — CSV export

### 8.3 Chain Migration
**Parameter**: `exit.migration`
**Type**: Boolean

> Allow chain-level migration?
>
> - [ ] Yes — Chain can migrate to new parent
> - [ ] No — Chain is permanent

---

## Section 9: Extensions

### 9.1 Custom Tables
**Parameter**: `extensions.tables`
**Type**: Array of strings

> List custom tables (must use `x_` prefix):
>
> - `x______________`
> - `x______________`

### 9.2 Plugins
**Parameter**: `extensions.plugins`
**Type**: Array of URIs

> List enabled plugins:
>
> - `plugin://______________`
> - `plugin://______________`

### 9.3 Webhooks
**Parameter**: `extensions.webhooks`
**Type**: Array of objects

> Configure event webhooks:
>
> | Event | URL |
> |-------|-----|
> | `new_wanted` | ______________ |
> | `completion` | ______________ |
> | `stamp` | ______________ |

---

## Output

Completed questionnaires generate a `constitution.yaml`:

```yaml
chain:
  name: "your-chain-name"
  description: "Your chain description"
  type: "public"
  parent: null

membership:
  registration: "open"
  initial_trust: 1
  required_fields: ["handle", "email"]
  rig_types: ["human", "agent"]

trust:
  tier_names:
    0: "Outsider"
    1: "Drifter"
    2: "Road Warrior"
    3: "War Chief"
    4: "Immortan"
  escalation: "automatic"
  auto_thresholds:
    "1_to_2": { stamps: 5, avg_quality: 3.5, days: 7 }
    "2_to_3": { stamps: 20, avg_quality: 4.0, days: 30 }
    "3_to_4": { stamps: 50, avg_quality: 4.5, days: 90 }
  validator_min: 3

governance:
  model: "council"

currency:
  type: "token"
  token:
    chain: "solana"
    address: "FN3xz4ndtLVHU5g3QeY5NVWRNbru23JxEEKxXvrJBAGS"
    symbol: "WASTELAND"
    decimals: 9
  distribution: "per_stamp"
  formula: "10 * wanted.effort * stamp.quality"

work:
  effort_multipliers:
    trivial: 0.25
    small: 0.5
    medium: 1.0
    large: 2.0
    epic: 4.0
  sandbox:
    enabled: false

stamps:
  dimensions: ["quality", "reliability", "expertise"]
  decay:
    enabled: true
    half_life_days: 90
    minimum: 0.1
  self_stamp: "strict"
  disclosure: "full"

federation:
  mode: "full"
  trusted: ["hop://hop/wl-commons"]
  trust_discount: 0.8
  sovereignty: "T1"

operations:
  pr_mode: true
  pr_approvals:
    minimum: 1
    min_trust: 3
    auto_merge: true
  signing: "optional"
  audit_retention: "forever"

exit:
  rights: "full"
  export: "dolt"
  migration: true

extensions:
  tables: []
  plugins: []
  webhooks: []
```

---

## Next Steps

1. Complete this questionnaire
2. Generate `constitution.yaml`
3. Run `gt chain init --constitution constitution.yaml`
4. Push to DoltHub
5. Invite initial members

---

*Build your chain, write your rules.*
