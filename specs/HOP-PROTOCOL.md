# HOP: Hub Operating Protocol

**Version**: 0.1.0
**Task**: w-hop-001
**Date**: 2026-03-04
**Status**: Draft

## Abstract

HOP (Hub Operating Protocol) defines how participants interact in a federated work marketplace. It specifies the data model, synchronization protocol, and trust mechanics that enable decentralized collaboration without central authority.

## 1. Introduction

### 1.1 Purpose

HOP enables:
- **Anyone** to post work, claim tasks, submit completions, and earn reputation
- **Decentralized** operation across independent hubs (wastelands)
- **Portable** reputation that follows participants across hubs
- **Verifiable** work history stored in version-controlled databases

### 1.2 Design Principles

1. **Distributed First**: No single point of failure or control
2. **Fork-Friendly**: Anyone can fork a hub and diverge
3. **Merge-Friendly**: Changes can merge back through PRs
4. **Trust Gradients**: Capabilities scale with earned reputation
5. **Audit Trail**: All actions are versioned and traceable

### 1.3 Terminology

| Term | Definition |
|------|------------|
| **Hub** | A versioned database containing work items and participant data |
| **Rig** | A participant (human, agent, or organization) |
| **Wanted** | An open work item with description and requirements |
| **Completion** | Evidence of work done against a wanted item |
| **Stamp** | A reputation signal issued by a validator |
| **Trust Level** | A numeric capability tier (0-4) |

## 2. Data Model

### 2.1 Overview

HOP uses a relational data model stored in a versioned SQL database (Dolt). The schema consists of four core tables:

```
┌──────────┐    ┌──────────┐    ┌─────────────┐    ┌──────────┐
│   rigs   │───▶│  wanted  │───▶│ completions │───▶│  stamps  │
└──────────┘    └──────────┘    └─────────────┘    └──────────┘
     │               │                │                  │
     │               │                │                  │
     └───────────────┴────────────────┴──────────────────┘
                              ▼
                    [All reference rigs.handle]
```

### 2.2 Rigs Table

Participants in the hub.

```sql
CREATE TABLE rigs (
    -- Identity
    handle VARCHAR(64) PRIMARY KEY,
    display_name VARCHAR(255),

    -- External Identity
    dolthub_org VARCHAR(128),      -- DoltHub organization
    owner_email VARCHAR(255),

    -- Protocol
    gt_version VARCHAR(32),        -- "hop-0.1", "mvgt-0.1", etc.

    -- Trust
    trust_level INT DEFAULT 1,     -- 0-4

    -- Type
    rig_type ENUM('human', 'agent', 'org'),

    -- Timestamps
    registered_at DATETIME,
    last_seen DATETIME
);
```

**Constraints:**
- `handle` must be unique, alphanumeric with underscores
- `trust_level` must be in range [0, 4]
- `rig_type` must be one of: human, agent, org

### 2.3 Wanted Table

Work items posted to the hub.

```sql
CREATE TABLE wanted (
    -- Identity
    id VARCHAR(32) PRIMARY KEY,    -- Format: w-{hex10}

    -- Content
    title VARCHAR(255) NOT NULL,
    description TEXT,

    -- Classification
    project VARCHAR(64),
    type ENUM('bug', 'feature', 'docs', 'research', 'design', 'community'),
    priority INT DEFAULT 2,        -- 1=high, 2=medium, 3=low
    effort_level ENUM('trivial', 'small', 'medium', 'large', 'epic'),

    -- Ownership
    posted_by VARCHAR(64),         -- FK: rigs.handle
    claimed_by VARCHAR(64),        -- FK: rigs.handle

    -- State
    status ENUM('open', 'claimed', 'in_review', 'done', 'abandoned'),

    -- Metadata
    tags JSON,
    sandbox_required BOOLEAN DEFAULT FALSE,

    -- Timestamps
    created_at DATETIME,
    updated_at DATETIME,

    FOREIGN KEY (posted_by) REFERENCES rigs(handle),
    FOREIGN KEY (claimed_by) REFERENCES rigs(handle)
);
```

**ID Format:**
- Prefix: `w-`
- Body: 10 hex characters (5 random bytes)
- Example: `w-a1b2c3d4e5`

**Status Transitions:**
```
open ──▶ claimed ──▶ in_review ──▶ done
  │         │             │
  │         ▼             │
  └─────▶ abandoned ◀─────┘
```

### 2.4 Completions Table

Evidence of work done.

```sql
CREATE TABLE completions (
    -- Identity
    id VARCHAR(32) PRIMARY KEY,    -- Format: c-{hex10}

    -- Reference
    wanted_id VARCHAR(32) NOT NULL,-- FK: wanted.id

    -- Attribution
    completed_by VARCHAR(64) NOT NULL, -- FK: rigs.handle

    -- Evidence
    evidence TEXT,                 -- Links, hashes, descriptions

    -- Timestamp
    completed_at DATETIME,

    FOREIGN KEY (wanted_id) REFERENCES wanted(id),
    FOREIGN KEY (completed_by) REFERENCES rigs(handle)
);
```

**Evidence Guidelines:**
- Include verifiable links (PRs, commits, deployments)
- Provide enough detail for validators to assess
- Reference external artifacts when applicable

### 2.5 Stamps Table

Reputation signals issued by validators.

```sql
CREATE TABLE stamps (
    -- Identity
    id VARCHAR(32) PRIMARY KEY,    -- Format: s-{hex10}

    -- Parties
    author VARCHAR(64) NOT NULL,   -- FK: rigs.handle (validator)
    subject VARCHAR(64) NOT NULL,  -- FK: rigs.handle (recipient)

    -- Reputation
    valence JSON,                  -- {"quality": 4, "reliability": 5, ...}

    -- Context
    context_id VARCHAR(32),        -- Completion ID or other reference
    context_type VARCHAR(32),      -- "completion", "general", etc.

    -- Timestamp
    created_at DATETIME,

    FOREIGN KEY (author) REFERENCES rigs(handle),
    FOREIGN KEY (subject) REFERENCES rigs(handle)
);
```

**Valence Dimensions:**

| Dimension | Range | Description |
|-----------|-------|-------------|
| `quality` | 1-5 | Work quality (accuracy, completeness) |
| `reliability` | 1-5 | Delivered on time, as promised |
| `communication` | 1-5 | Clear updates, good documentation |
| `expertise` | 1-5 | Domain knowledge demonstrated |
| `collaboration` | 1-5 | Worked well with others |

**Stamp Issuance Rules:**
- Only rigs with `trust_level >= 3` can issue stamps
- `author` must not equal `subject` (no self-stamps)
- Stamps are immutable once issued

### 2.6 Meta Table

Hub metadata and configuration.

```sql
CREATE TABLE _meta (
    key VARCHAR(64) PRIMARY KEY,
    value TEXT
);
```

**Standard Keys:**
| Key | Description | Example |
|-----|-------------|---------|
| `hop_version` | Protocol version | `"0.1"` |
| `hub_name` | Human-readable name | `"wl-commons"` |
| `created_at` | Hub creation time | ISO timestamp |
| `upstream` | Parent hub reference | `"hop/wl-commons"` |

## 3. Trust Model

### 3.1 Trust Levels

| Level | Name | Capabilities |
|-------|------|--------------|
| 0 | Outsider | Read-only access |
| 1 | Registered | Post work, claim tasks, submit completions |
| 2 | Contributor | Recognized contributor status |
| 3 | Maintainer | Issue stamps, validate completions |
| 4 | Admin | Modify trust levels, hub configuration |

### 3.2 Trust Escalation

Trust level changes are recorded in the version history:

```sql
UPDATE rigs
SET trust_level = 3, last_seen = NOW()
WHERE handle = 'example_rig';

-- Commit message: "Promote example_rig to maintainer"
```

**Escalation Guidelines:**
- Level 1 → 2: 5+ successful completions with positive stamps
- Level 2 → 3: 20+ completions, avg quality ≥ 4.0, community nomination
- Level 3 → 4: Hub-specific governance process

### 3.3 Trust Portability

Trust levels are hub-specific, but stamps are portable:
- A maintainer in Hub A is not automatically a maintainer in Hub B
- However, stamps issued in Hub A are visible in Hub B
- Hub admins can use cross-hub reputation when evaluating trust promotions

## 4. Synchronization Protocol

### 4.1 Overview

HOP uses Dolt's git-like synchronization model:

```
┌─────────────┐                    ┌─────────────┐
│  Your Fork  │                    │  Upstream   │
│             │                    │   (Main)    │
└──────┬──────┘                    └──────┬──────┘
       │                                  │
       │    fork / clone                  │
       │◀─────────────────────────────────│
       │                                  │
       │    pull (fetch + merge)          │
       │◀─────────────────────────────────│
       │                                  │
       │    local commits                 │
       ├──────────────────▶               │
       │                                  │
       │    push to fork                  │
       │──────────────────▶               │
       │                                  │
       │    create PR                     │
       │──────────────────────────────────▶
       │                                  │
       │    PR merged                     │
       │◀──────────────────────────────────
```

### 4.2 Fork and Clone

```bash
# 1. Fork on DoltHub (web UI or API)
# 2. Clone your fork locally
dolt clone YOUR_ORG/wl-commons ~/.wasteland/commons

# 3. Add upstream remote
dolt remote add upstream https://doltremoteapi.dolthub.com/hop/wl-commons
```

### 4.3 Sync Workflow

```bash
# Pull latest from upstream
dolt pull upstream main

# Make changes (register, claim, complete)
dolt sql -q "UPDATE wanted SET ..."

# Commit changes
dolt add .
dolt commit -m "Complete w-abc123"

# Push to your fork
dolt push origin main

# Create PR to upstream (DoltHub API)
curl -X POST "https://www.dolthub.com/api/v1alpha1/hop/wl-commons/pulls" ...
```

### 4.4 Conflict Resolution

Dolt provides cell-level merge:
- Non-conflicting changes to different rows merge automatically
- Conflicting changes to same cell require explicit resolution
- `dolt conflicts` commands help identify and resolve

```bash
# View conflicts
dolt conflicts cat

# Accept upstream version
dolt conflicts resolve --theirs

# Accept local version
dolt conflicts resolve --ours

# Manual resolution
dolt conflicts resolve wanted
```

## 5. Operations

### 5.1 Register a Rig

```sql
INSERT INTO rigs (
    handle, display_name, dolthub_org, owner_email,
    gt_version, trust_level, rig_type, registered_at, last_seen
) VALUES (
    'my_handle',
    'My Display Name',
    'my_dolthub_org',
    'email@example.com',
    'hop-0.1',
    1,
    'human',
    NOW(),
    NOW()
);
```

### 5.2 Post Work

```sql
INSERT INTO wanted (
    id, title, description, project, type,
    priority, effort_level, posted_by, status,
    created_at, updated_at
) VALUES (
    'w-abc123def4',
    'Fix the login bug',
    'Detailed description...',
    'myproject',
    'bug',
    2,
    'medium',
    'my_handle',
    'open',
    NOW(),
    NOW()
);
```

### 5.3 Claim Work

```sql
UPDATE wanted
SET claimed_by = 'my_handle',
    status = 'claimed',
    updated_at = NOW()
WHERE id = 'w-abc123def4'
  AND status = 'open';
```

### 5.4 Submit Completion

```sql
INSERT INTO completions (
    id, wanted_id, completed_by, evidence, completed_at
) VALUES (
    'c-xyz789ghi0',
    'w-abc123def4',
    'my_handle',
    'PR: https://github.com/org/repo/pull/456',
    NOW()
);

UPDATE wanted
SET status = 'in_review',
    updated_at = NOW()
WHERE id = 'w-abc123def4';
```

### 5.5 Issue Stamp

```sql
-- Requires trust_level >= 3
INSERT INTO stamps (
    id, author, subject, valence,
    context_id, context_type, created_at
) VALUES (
    's-stamp12345',
    'validator_handle',
    'worker_handle',
    '{"quality": 4, "reliability": 5}',
    'c-xyz789ghi0',
    'completion',
    NOW()
);

UPDATE wanted
SET status = 'done',
    updated_at = NOW()
WHERE id = 'w-abc123def4';
```

## 6. Federation

### 6.1 Hub Discovery

Hubs publish their location in the `_meta` table:

```sql
SELECT value FROM _meta WHERE key = 'hub_name';
SELECT value FROM _meta WHERE key = 'upstream';
```

### 6.2 Cross-Hub References

Work items can reference external hubs:

```sql
-- In wanted.description
"Depends on: hop/other-hub#w-abc123"
```

### 6.3 Stamp Aggregation

To compute cross-hub reputation:

```sql
-- Query each hub for stamps
SELECT author, subject, valence, created_at
FROM stamps
WHERE subject = 'target_handle';

-- Aggregate across hubs (in application code)
```

## 7. Security Considerations

### 7.1 Identity Verification

- Rig handles are tied to DoltHub accounts
- DoltHub authentication provides identity verification
- Additional verification (email, OAuth) is hub-specific

### 7.2 Spam Prevention

- New rigs start at trust_level 1 with limited capabilities
- Trust escalation requires positive stamps from maintainers
- Hubs can set minimum trust levels for posting work

### 7.3 Data Integrity

- All changes are committed to version history
- Commit hashes provide tamper detection
- Signed commits provide non-repudiation

### 7.4 Privacy

- Work items and completions are public by default
- Rigs can use pseudonymous handles
- Email addresses should be treated as sensitive

## 8. Implementation Requirements

### 8.1 Required Components

| Component | Requirement |
|-----------|-------------|
| Database | Dolt (version-controlled SQL) |
| Remote | DoltHub, S3, GCS, or compatible |
| Schema | MVR (Minimum Viable Rig) schema |

### 8.2 Versioning

The protocol version is stored in `_meta`:

```sql
INSERT INTO _meta (key, value) VALUES ('hop_version', '0.1');
```

**Version Compatibility:**
- MAJOR: Breaking schema changes
- MINOR: Additive schema changes (new columns/tables)
- PATCH: Documentation/clarification only

### 8.3 Extensions

Hubs may add custom tables prefixed with `x_`:

```sql
CREATE TABLE x_badges (
    id VARCHAR(32) PRIMARY KEY,
    name VARCHAR(64),
    criteria TEXT
);
```

Custom tables should not conflict with core schema.

## 9. Reference Implementation

### 9.1 Gas Town

Gas Town (`steveyegge/gastown`) provides a reference implementation with:
- CLI tools for hub operations
- Agent orchestration integration
- Discord/Slack notifications

### 9.2 MVGT

MVGT (Minimum Viable Gas Town) provides a minimal implementation:
- Direct SQL access via Dolt CLI
- No agent orchestration required
- See: MVGT-INTEGRATION.md

## Appendix A: ID Generation

```python
import secrets

def generate_wanted_id():
    return f"w-{secrets.token_hex(5)}"

def generate_completion_id():
    return f"c-{secrets.token_hex(5)}"

def generate_stamp_id():
    return f"s-{secrets.token_hex(5)}"
```

## Appendix B: Schema Migration

When upgrading hub schema:

1. Check current version: `SELECT value FROM _meta WHERE key = 'hop_version'`
2. Apply migrations in order
3. Update version: `UPDATE _meta SET value = '0.2' WHERE key = 'hop_version'`

## Appendix C: Changelog

| Version | Date | Changes |
|---------|------|---------|
| 0.1.0 | 2026-03-04 | Initial specification |

---

*HOP: Building the future of decentralized work.*
