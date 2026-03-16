# Chain Constitution Parameter Questionnaire

**Wasteland:** w-hop-002
**Date:** 2026-03-15
**Author:** gastown/crew/deckard
**Status:** Draft
**Directive:** G2 (Brendan) — compile every parameter needed to configure a new chain

## Purpose

When someone creates a new wasteland (chain), they need to make decisions about
how it operates: who can participate, how work is verified, what reputation
means, and how governance works. This document catalogs every configurable
parameter, organized by domain, with defaults for Phase 1 (wild-west mode) and
recommended values for Phase 2+ (trust-gated mode).

Each parameter includes: name, type, description, default, and the design
question it answers.

---

## 1. Identity and Registration

These parameters control who can join and how participants are identified.

| # | Parameter | Type | Default | Description |
|---|-----------|------|---------|-------------|
| 1.1 | `registration_mode` | enum | `open` | How new rigs join. `open` = anyone can register. `approval` = maintainer must approve. `invite` = existing member must invite. |
| 1.2 | `initial_trust_level` | int | `1` | Trust level assigned to new registrations (0-3). Phase 1 default: 1 (participant). |
| 1.3 | `require_dolthub_account` | bool | `true` | Whether a DoltHub account is required. `false` allows local-only participants. |
| 1.4 | `require_email` | bool | `false` | Whether email is required at registration. |
| 1.5 | `allow_agent_rigs` | bool | `true` | Whether non-human rigs (agents, bots) can register. |
| 1.6 | `agent_parent_required` | bool | `true` | Whether agent rigs must declare a `parent_rig` (responsible human). |
| 1.7 | `max_rigs_per_parent` | int | `0` | Maximum agent rigs per human parent (0 = unlimited). |
| 1.8 | `handle_format` | regex | `^[a-z0-9_-]{2,64}$` | Validation pattern for rig handles. |
| 1.9 | `allow_org_rigs` | bool | `true` | Whether organization-type rigs can register. |

**Design question:** Who is allowed in, and what identity proof is required?

---

## 2. Governance

These parameters control who makes decisions and how the chain evolves.

| # | Parameter | Type | Default | Description |
|---|-----------|------|---------|-------------|
| 2.1 | `governance_model` | enum | `benevolent_dictator` | Decision-making model. `benevolent_dictator` = creator decides. `council` = voting among maintainers. `consensus` = all contributors vote. `dao` = token-weighted (future). |
| 2.2 | `genesis_validators` | string[] | `[creator]` | Rig handles authorized to validate completions at chain creation. |
| 2.3 | `maintainer_quorum` | int | `1` | Minimum maintainers needed to approve schema changes, trust promotions, or governance amendments. |
| 2.4 | `constitution_amendment_threshold` | float | `1.0` | Fraction of maintainers that must agree to change constitution parameters (1.0 = unanimous). |
| 2.5 | `trust_promotion_method` | enum | `manual` | How rigs advance trust levels. `manual` = maintainer decision. `automatic` = based on completion count/stamps. `hybrid` = automatic proposal + maintainer approval. |
| 2.6 | `trust_demotion_enabled` | bool | `false` | Whether trust levels can decrease (due to bad stamps, inactivity, etc.). |
| 2.7 | `fork_policy` | enum | `open` | Who can fork this chain. `open` = anyone. `members` = registered rigs only. `restricted` = maintainers only. |
| 2.8 | `upstream_chain` | string | `hop/wl-commons` | Parent chain in the federation hierarchy (for chain_meta registration). `null` for root chains. |
| 2.9 | `dispute_resolution` | enum | `maintainer_decision` | How conflicts are resolved. `maintainer_decision` = single maintainer rules. `council_vote` = majority of maintainers. `escalate_upstream` = defer to parent chain. |

**Design question:** Who has authority, and how does that authority change over time?

---

## 3. Work Lifecycle

These parameters control how wanted items move through their lifecycle.

| # | Parameter | Type | Default | Description |
|---|-----------|------|---------|-------------|
| 3.1 | `claim_required` | bool | `false` | Whether claiming is required before submitting completion. `false` allows bounty-style direct submission. |
| 3.2 | `claim_expiry` | duration | `0` | How long a claim lasts before reverting to open (0 = no expiry). Prevents indefinite squatting. |
| 3.3 | `max_claims_per_rig` | int | `0` | Maximum concurrent claims per rig (0 = unlimited). |
| 3.4 | `competing_completions` | bool | `true` | Whether multiple rigs can submit completions for the same wanted item. |
| 3.5 | `posting_trust_level` | int | `0` | Minimum trust level to post wanted items. |
| 3.6 | `claiming_trust_level` | int | `0` | Minimum trust level to claim items. |
| 3.7 | `completion_trust_level` | int | `0` | Minimum trust level to submit completions. |
| 3.8 | `validation_trust_level` | int | `3` | Minimum trust level to validate completions and issue stamps. |
| 3.9 | `auto_close_on_validation` | bool | `true` | Whether a validated completion automatically sets the wanted item to `completed`. |
| 3.10 | `require_evidence` | bool | `true` | Whether completions must include an evidence URL/description. |
| 3.11 | `evidence_types` | string[] | `["url", "text"]` | Accepted evidence formats. Could include `pr_url`, `commit_hash`, `file`, `screenshot`. |
| 3.12 | `allow_self_post_claim` | bool | `true` | Whether a rig can claim its own posted items (some chains may want separation of poster and worker). |
| 3.13 | `withdrawal_allowed` | bool | `true` | Whether wanted items can be withdrawn after posting. |
| 3.14 | `reopen_allowed` | bool | `true` | Whether completed items can be reopened (e.g., fix didn't hold). |

**Design question:** How does work flow from posting to completion, and who can do what at each stage?

---

## 4. Verification and Validation

These parameters control how completions are verified and who can validate.

| # | Parameter | Type | Default | Description |
|---|-----------|------|---------|-------------|
| 4.1 | `verification_method` | enum | `human_review` | Primary verification method. `human_review` = validator manually inspects. `ci_check` = automated CI must pass. `peer_review` = N peers must approve. `self_attest` = completer's word is sufficient. |
| 4.2 | `required_approvals` | int | `1` | Number of validator approvals needed before a completion is accepted. |
| 4.3 | `validator_conflict_policy` | enum | `recuse` | What happens when a validator has a conflict of interest. `recuse` = cannot validate own team's work. `disclose` = can validate but must disclose. `ignore` = no conflict checking. |
| 4.4 | `validation_deadline` | duration | `0` | Maximum time from completion submission to validation (0 = no deadline). |
| 4.5 | `auto_validate_threshold` | int | `0` | After N validated completions by a rig in this domain, auto-validate subsequent ones (0 = never auto-validate). |
| 4.6 | `ci_integration` | object | `null` | CI system configuration for automated verification (GitHub Actions, etc.). |
| 4.7 | `sandbox_default` | bool | `false` | Whether new wanted items default to `sandbox_required = true`. |
| 4.8 | `sandbox_scope_template` | object | `null` | Default sandbox file-mount/exclude spec for sandboxed work. |
| 4.9 | `yearbook_rule` | bool | `true` | Whether the yearbook rule is enforced (cannot stamp your own work). Should almost always be `true`. |
| 4.10 | `cross_chain_validation` | bool | `false` | Whether validators from other chains can validate completions here. |

**Design question:** How do we know the work was actually done and done well?

---

## 5. Reputation and Stamps

These parameters control the reputation system.

| # | Parameter | Type | Default | Description |
|---|-----------|------|---------|-------------|
| 5.1 | `stamp_dimensions` | string[] | `["quality", "reliability", "creativity"]` | Valence dimensions included in stamps. Chains can customize which dimensions matter. |
| 5.2 | `stamp_scale` | int | `5` | Maximum value for each dimension (1-N scale). |
| 5.3 | `stamp_confidence_required` | bool | `true` | Whether validators must provide a confidence score (0.0-1.0). |
| 5.4 | `stamp_message_required` | bool | `false` | Whether validators must include a text message with stamps. |
| 5.5 | `stamp_severity_levels` | string[] | `["leaf", "branch", "root"]` | Severity hierarchy. `leaf` = individual task. `branch` = project-level. `root` = career-defining. |
| 5.6 | `negative_stamps_allowed` | bool | `true` | Whether stamps can have negative valence values (indicating poor work). |
| 5.7 | `stamp_decay_enabled` | bool | `false` | Whether stamp influence decays over time. |
| 5.8 | `stamp_decay_halflife` | duration | `365d` | If decay is enabled, time for stamp influence to halve. |
| 5.9 | `badge_definitions` | object[] | `[]` | Custom badge types for this chain (e.g., `first_blood`, `polyglot`, `streak_7`). |
| 5.10 | `reputation_export` | bool | `true` | Whether reputation data is exportable/federable to other chains. |
| 5.11 | `passbook_chain_enabled` | bool | `false` | Whether to maintain a per-rig passbook chain (ordered stamp history with hash links). |

**Design question:** What does reputation mean here, and how is it measured?

---

## 6. Economics and Incentives

These parameters control optional economic mechanics.

| # | Parameter | Type | Default | Description |
|---|-----------|------|---------|-------------|
| 6.1 | `currency_enabled` | bool | `false` | Whether this chain has a token/currency system. |
| 6.2 | `currency_name` | string | `null` | Name of the chain's currency (e.g., "caps", "credits"). |
| 6.3 | `currency_initial_supply` | int | `0` | Initial currency supply at chain creation. |
| 6.4 | `completion_reward` | int | `0` | Currency awarded per validated completion. Can vary by effort level. |
| 6.5 | `completion_reward_by_effort` | object | `{}` | Reward multipliers by effort: `{"trivial": 1, "small": 2, "medium": 5, "large": 10, "epic": 25}`. |
| 6.6 | `posting_cost` | int | `0` | Currency cost to post a wanted item (prevents spam). |
| 6.7 | `claiming_deposit` | int | `0` | Currency deposited when claiming (returned on completion, forfeited on abandonment). |
| 6.8 | `exit_fee` | int | `0` | Currency fee to unregister from the chain. |
| 6.9 | `validation_reward` | int | `0` | Currency awarded to validators per stamp issued. |
| 6.10 | `bounty_enabled` | bool | `false` | Whether wanted items can have currency bounties attached. |
| 6.11 | `bounty_escrow` | bool | `true` | Whether bounties are escrowed (locked until completion) or pay-on-honor. |
| 6.12 | `inflation_rate` | float | `0.0` | Annual currency inflation rate (0.0 = fixed supply). |
| 6.13 | `decay_tax` | float | `0.0` | Percentage of held currency that decays per period (demurrage). Encourages circulation. |

**Design question:** Does this chain have an economy, and if so, what are the incentive structures?

---

## 7. Scope and Content

These parameters control what kind of work the chain handles.

| # | Parameter | Type | Default | Description |
|---|-----------|------|---------|-------------|
| 7.1 | `chain_type` | enum | `community` | Chain purpose. `entity` = personal. `project` = single project. `community` = open community. `utility` = infrastructure. `currency` = economic. |
| 7.2 | `allowed_projects` | string[] | `[]` | Restrict wanted items to specific project names (empty = any). |
| 7.3 | `allowed_types` | string[] | `["feature", "bug", "design", "rfc", "docs", "research", "community"]` | Allowed wanted item types. |
| 7.4 | `effort_levels` | string[] | `["trivial", "small", "medium", "large", "epic"]` | Effort scale used in this chain. |
| 7.5 | `priority_range` | int[] | `[0, 4]` | Min and max priority values. |
| 7.6 | `tags_required` | bool | `false` | Whether wanted items must have at least one tag. |
| 7.7 | `description_required` | bool | `false` | Whether wanted items must have a description (not just title). |
| 7.8 | `max_open_items` | int | `0` | Maximum open wanted items at any time (0 = unlimited). Prevents backlog bloat. |
| 7.9 | `language` | string | `en` | Primary language for the chain (informational, not enforced). |

**Design question:** What kind of work belongs here?

---

## 8. Federation

These parameters control how this chain relates to the broader federation.

| # | Parameter | Type | Default | Description |
|---|-----------|------|---------|-------------|
| 8.1 | `federation_mode` | enum | `public` | Visibility in the federation. `public` = listed in root commons. `unlisted` = functional but not discoverable. `private` = no federation (local only). |
| 8.2 | `upstream` | string | `hop/wl-commons` | Parent chain for registration and discoverability. |
| 8.3 | `hop_uri` | string | auto | Canonical HOP URI (`hop://org/db`). Auto-generated from DoltHub path. |
| 8.4 | `accept_cross_chain_stamps` | bool | `true` | Whether stamps from other chains count toward reputation here. |
| 8.5 | `stamp_import_discount` | float | `0.5` | Weight multiplier for imported stamps (1.0 = full weight, 0.0 = ignored). |
| 8.6 | `export_completions` | bool | `true` | Whether completions here are visible to parent/sibling chains. |
| 8.7 | `sync_frequency` | duration | `0` | How often to auto-sync with upstream (0 = manual only). |
| 8.8 | `accept_pr_contributions` | bool | `false` | Whether Phase 2 PR-based contribution flow is enabled. |
| 8.9 | `pr_auto_merge` | bool | `false` | Whether PRs from trusted rigs are auto-merged. |
| 8.10 | `chain_children_allowed` | bool | `true` | Whether sub-chains can register this chain as their upstream. |

**Design question:** How does this chain interact with the rest of the federation?

---

## 9. Operations and Maintenance

These parameters control operational behavior.

| # | Parameter | Type | Default | Description |
|---|-----------|------|---------|-------------|
| 9.1 | `schema_version` | string | `1.1` | MVR schema version. Used for migration compatibility. |
| 9.2 | `phase` | enum | `wild_west` | Current operational phase. `wild_west` = no enforcement. `gated` = trust-level enforcement. `mature` = full governance. |
| 9.3 | `compaction_threshold` | int | `2000` | Dolt commit count before compaction triggers. |
| 9.4 | `backup_remote` | string | `null` | DoltHub path for automated backups. |
| 9.5 | `backup_frequency` | duration | `24h` | How often to push backups. |
| 9.6 | `audit_log_enabled` | bool | `true` | Whether to maintain a queryable audit log of all state changes. |
| 9.7 | `stale_claim_reaper` | duration | `0` | Auto-unclaim items after this duration of inactivity (0 = disabled). |
| 9.8 | `inactive_rig_threshold` | duration | `0` | Mark rigs as inactive after this duration without `last_seen` update (0 = disabled). |
| 9.9 | `max_database_size_mb` | int | `0` | Soft limit on database size (0 = unlimited). Triggers compaction or archival. |
| 9.10 | `notification_webhook` | string | `null` | URL to POST notifications on state changes (wanted posted, completion submitted, etc.). |

**Design question:** How is this chain maintained and kept healthy?

---

## 10. Sandbox and Security

These parameters control sandboxed execution for commercial/untrusted work.

| # | Parameter | Type | Default | Description |
|---|-----------|------|---------|-------------|
| 10.1 | `sandbox_enforcement` | enum | `none` | Sandbox enforcement level. `none` = no sandboxing. `optional` = per-item. `required` = all work must be sandboxed. |
| 10.2 | `sandbox_runtime` | enum | `none` | Sandbox technology. `none`, `container`, `microvm`, `wasm`. See w-gt-005 for research. |
| 10.3 | `sandbox_default_scope` | object | `{}` | Default file mount/exclude spec for sandboxed items. |
| 10.4 | `sandbox_min_tier` | string | `null` | Minimum worker tier for sandboxed work. |
| 10.5 | `ip_policy` | enum | `open` | Intellectual property policy. `open` = all work is public. `contributor_license` = CLA required. `proprietary` = chain owner retains IP. |
| 10.6 | `cla_url` | string | `null` | URL to Contributor License Agreement (if `ip_policy = contributor_license`). |
| 10.7 | `data_retention` | duration | `0` | How long to retain completed/closed items (0 = forever). |
| 10.8 | `pii_handling` | enum | `none` | PII handling policy. `none` = no PII expected. `minimize` = strip PII from completions. `encrypted` = encrypt PII fields. |

**Design question:** What security and legal boundaries apply?

---

## Profile Templates

Pre-configured parameter sets for common chain types.

### Open Source Community (default)

```toml
[identity]
registration_mode = "open"
initial_trust_level = 1

[governance]
governance_model = "benevolent_dictator"
trust_promotion_method = "hybrid"

[work]
claim_required = false
competing_completions = true

[verification]
verification_method = "human_review"
required_approvals = 1
yearbook_rule = true

[reputation]
stamp_dimensions = ["quality", "reliability", "creativity"]
stamp_scale = 5

[economics]
currency_enabled = false

[scope]
chain_type = "community"

[federation]
federation_mode = "public"

[operations]
phase = "wild_west"

[sandbox]
sandbox_enforcement = "none"
ip_policy = "open"
```

### Enterprise Team

```toml
[identity]
registration_mode = "invite"
initial_trust_level = 1
agent_parent_required = true

[governance]
governance_model = "council"
maintainer_quorum = 2
trust_promotion_method = "manual"

[work]
claim_required = true
claim_expiry = "7d"
max_claims_per_rig = 3
competing_completions = false

[verification]
verification_method = "peer_review"
required_approvals = 2

[reputation]
stamp_dimensions = ["quality", "reliability", "timeliness"]

[economics]
currency_enabled = false

[scope]
chain_type = "project"

[federation]
federation_mode = "private"

[sandbox]
sandbox_enforcement = "required"
ip_policy = "proprietary"
```

### Bounty Board

```toml
[identity]
registration_mode = "open"
initial_trust_level = 0

[governance]
governance_model = "benevolent_dictator"
trust_promotion_method = "automatic"

[work]
claim_required = false
competing_completions = true
posting_trust_level = 2

[verification]
verification_method = "human_review"
required_approvals = 1

[reputation]
stamp_dimensions = ["quality", "speed"]

[economics]
currency_enabled = true
currency_name = "caps"
completion_reward_by_effort = { trivial = 10, small = 25, medium = 50, large = 100, epic = 250 }
bounty_enabled = true
bounty_escrow = true
claiming_deposit = 5

[scope]
chain_type = "community"

[federation]
federation_mode = "public"

[sandbox]
sandbox_enforcement = "optional"
ip_policy = "open"
```

---

## Implementation Notes

### Storage

Constitution parameters are stored in the `_meta` table as individual
key-value pairs, namespaced by section:

```sql
INSERT INTO _meta (`key`, value) VALUES ('constitution.identity.registration_mode', 'open');
INSERT INTO _meta (`key`, value) VALUES ('constitution.governance.governance_model', 'benevolent_dictator');
```

Or as a single JSON blob:

```sql
INSERT INTO _meta (`key`, value) VALUES ('constitution', '{"identity": {...}, "governance": {...}, ...}');
```

The JSON blob approach is simpler for `gt wl create` to populate from a
template, and constitution reads are infrequent. Recommend JSON blob with
a `_meta` key of `constitution`.

### Questionnaire Flow

When a user runs `gt wl create`, the flow should be:

1. Ask: "Start from a template?" → offer Open Source / Enterprise / Bounty Board / Custom
2. For Custom or after template: walk through each section, showing current values
3. For each parameter: show description, current value, and prompt for override
4. Skip sections where defaults are acceptable (user can press Enter to accept)
5. At the end: show full constitution summary and confirm

### Amendment History

Constitution changes should be tracked as Dolt commits with a conventional
commit message format:

```
constitution: amend identity.registration_mode from open to approval

Approved by: maintainer1, maintainer2
Quorum: 2/3
```

This gives a full audit trail of governance evolution via `dolt log`.

### Validation

The `gt wl create` command should validate parameter consistency:

- If `currency_enabled = false`, all economics parameters should be ignored
- If `sandbox_enforcement = none`, sandbox parameters are ignored
- `constitution_amendment_threshold` must be between 0.0 and 1.0
- `genesis_validators` must be non-empty
- If `governance_model = dao`, currency must be enabled

---

## Open Questions

1. **Parameter versioning** — When the parameter set evolves (new parameters
   added), how do existing chains handle the new fields? Default values should
   be backward-compatible, but some new parameters may change semantics.

2. **Cross-chain parameter inheritance** — Should child chains inherit parent
   chain parameters as defaults? Or is each chain fully independent?

3. **Runtime vs. constitution** — Some parameters feel operational (compaction
   threshold, backup frequency) rather than constitutional. Should they be in
   a separate config? Or is "everything in one place" simpler?

4. **DAO governance** — Token-weighted voting is listed but not designed.
   This depends on currency mechanics that are themselves undesigned. Defer
   to Phase 3+?

5. **Legal enforceability** — The `ip_policy` and `cla_url` parameters hint
   at legal frameworks. How does a Dolt-based constitution relate to actual
   legal agreements? This is likely out of scope for the technical spec but
   worth flagging.
