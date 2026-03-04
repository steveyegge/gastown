+++
name = "dolt-snapshots"
description = "Tag Dolt databases at convoy boundaries for audit, diff, and rollback"
version = 2

[gate]
type = "cooldown"
duration = "5m"

[tracking]
labels = ["plugin:dolt-snapshots", "category:data-safety"]
digest = true

[execution]
timeout = "2m"
notify_on_failure = true
severity = "low"
+++

# Dolt Snapshots v2

Snapshots Dolt databases at convoy lifecycle boundaries using **tags** (immutable)
and optionally **branches** (mutable, for working diffs).

Tags are the primary artifact — they point to a specific commit and can't be
moved. Branches are secondary and optional (for interactive diffing).

## What this enables

**Convoy audit** — verify agents did what they were supposed to:
```sql
SELECT * FROM dolt_diff('staged/pi-rust-bug-fixes-hq-cv-xrwki', 'HEAD', 'issues')
SELECT * FROM dolt_diff_stat('staged/pi-rust-bug-fixes-hq-cv-xrwki', 'HEAD')
```

**Convoy rollback** — revert a database to pre-convoy state:
```sql
CALL DOLT_CHECKOUT('staged/pi-rust-bug-fixes-hq-cv-xrwki');          -- whole DB
CALL DOLT_CHECKOUT('staged/pi-rust-bug-fixes-hq-cv-xrwki', 'issues'); -- single table
```

**Cross-convoy comparison** — track progress between runs:
```sql
SELECT * FROM dolt_diff('staged/pi-rust-bug-fixes-hq-cv-xrwki', 'staged/otel-dashboard-hq-cv-7q3vi', 'issues')
```

**Data loss investigation** — when backup alerts fire, diff against last snapshot:
```sql
SELECT * FROM dolt_diff('staged/pi-rust-bug-fixes-hq-cv-xrwki', 'HEAD', 'issues')
WHERE diff_type = 'removed'
```

## What branches enable (mutable sandboxes)

Branches are writable copies of the database at snapshot time. Unlike tags,
you can commit to them — making them useful for:

**Dry-run convoy work** — test bulk operations without touching main:
```sql
CALL DOLT_CHECKOUT('convoy/pi-rust-bug-fixes-hq-cv-xrwki');
UPDATE issues SET status = 'in_progress' WHERE id IN ('st-31h7', 'st-9tid');
SELECT * FROM dolt_diff('convoy/pi-rust-bug-fixes-hq-cv-xrwki', 'main', 'issues');
CALL DOLT_CHECKOUT('main');  -- discard, main untouched
```

**Isolated convoy writes** — agents write to branch, refinery merges (like a DB PR):
```sql
CALL DOLT_CHECKOUT('convoy/pi-rust-bug-fixes-hq-cv-xrwki');
CALL DOLT_COMMIT('-Am', 'st-31h7: fixed pi-rust timeout');
-- Later, refinery merges:
CALL DOLT_CHECKOUT('main');
CALL DOLT_MERGE('convoy/pi-rust-bug-fixes-hq-cv-xrwki');
```

**What-if analysis** — test theories without risk:
```sql
CALL DOLT_CHECKOUT('convoy/pi-rust-bug-fixes-hq-cv-xrwki');
UPDATE issues SET status = 'closed' WHERE updated_at < '2026-02-01';
SELECT * FROM dolt_diff('convoy/pi-rust-bug-fixes-hq-cv-xrwki', 'main', 'dependencies');
CALL DOLT_CHECKOUT('main');  -- throw it away
```

**Parallel convoy isolation** — two convoys write to separate branches,
merge sequentially — no race conditions on main.

## Why tags over branches

- A branch is just a pointer that moves with new commits — not a true snapshot
- A tag is immutable: `staged/pi-rust-bug-fixes-hq-cv-xrwki` always points to
  the exact state when the convoy was staged
- Tags survive branch cleanup and are cheaper to keep long-term
- `dolt diff staged/convoy-A staged/convoy-B` works with tags

## Config

```bash
DOLT_HOST="127.0.0.1"
DOLT_PORT=3307
DOLT_USER="root"
ROUTES_FILE="$HOME/gt/.beads/routes.jsonl"
```

## Step 1: Snapshot convoys at lifecycle boundaries

Two snapshots per convoy, at different lifecycle points:

- **open** — convoy created, issues wired, pre-work baseline
  - Tag: `open/<slug>-<id>` — immutable snapshot of state before any work starts
- **launched** — after staging validates DAG and dispatches Wave 1
  - Tag: `staged/<slug>-<id>` — immutable snapshot at launch time (audit, rollback)
  - Branch: `convoy/<slug>-<id>` — mutable workspace (review, edit, dry-run)

Example: `open/formula-plugin-audit-hq-cv-8kq` then `staged/formula-plugin-audit-hq-cv-8kq`

```bash
echo "=== Convoy Snapshots ==="
TAGS_CREATED=0
BRANCHES_CREATED=0
TAGS_FAILED=0
COMMITS_MADE=0

# Slugify: lowercase, replace spaces/special chars with hyphens, trim
slugify() {
  echo "$1" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/-/g; s/--*/-/g; s/^-//; s/-$//' | cut -c1-40
}

# Get all rig databases
DATABASES=$(mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u "$DOLT_USER" -N -B \
  -e "SHOW DATABASES" 2>/dev/null \
  | grep -v '^information_schema$\|^mysql$\|^witness$')

# Find convoys needing snapshots at two lifecycle points:
#   1. open — needs open/ tag (pre-work baseline)
#   2. staged_ready/open(launched)/closed — needs staged/ tag (launch baseline)
# Includes recently-closed convoys (24h) that completed before plugin ran.
# Only convoys with tracked issues (wired up) are eligible.
CONVOY_ROWS=$(mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u "$DOLT_USER" -N -B hq \
  -e "
    SELECT i.id, i.title, i.status,
      CASE WHEN EXISTS (SELECT 1 FROM dolt_tags t WHERE t.tag_name LIKE CONCAT('open/%-', i.id))
           THEN 1 ELSE 0 END AS has_open_tag,
      CASE WHEN EXISTS (SELECT 1 FROM dolt_tags t WHERE t.tag_name LIKE CONCAT('staged/%-', i.id))
           THEN 1 ELSE 0 END AS has_staged_tag
    FROM issues i
    WHERE i.issue_type = 'convoy'
      AND (
        i.status IN ('staged_ready', 'open')
        OR (i.status = 'closed' AND i.updated_at >= NOW() - INTERVAL 24 HOUR)
      )
      AND EXISTS (
        SELECT 1 FROM dependencies d
        WHERE d.issue_id = i.id AND d.type = 'tracks'
      )
    HAVING has_open_tag = 0 OR has_staged_tag = 0
  " 2>/dev/null)

if [ -z "$CONVOY_ROWS" ]; then
  echo "  No convoys need snapshots"
else
  echo "$CONVOY_ROWS" | while IFS=$'\t' read -r CONVOY_ID CONVOY_TITLE CONVOY_STATUS HAS_OPEN HAS_STAGED; do
    SLUG=$(slugify "$CONVOY_TITLE")
    SAFE_TITLE=$(echo "$CONVOY_TITLE" | sed "s/'/''/g")  # escape single quotes for SQL

    # Determine which snapshots to take
    NEED_OPEN=false
    NEED_STAGED=false
    if [ "$HAS_OPEN" = "0" ]; then
      NEED_OPEN=true
    fi
    # staged/ tag only after convoy has been launched (staged_ready or beyond)
    if [ "$HAS_STAGED" = "0" ] && [ "$CONVOY_STATUS" != "open" ]; then
      NEED_STAGED=true
    fi

    TAG_NAME=""
    BRANCH_NAME=""
    if [ "$NEED_OPEN" = "true" ]; then
      TAG_NAME="open/${SLUG}-${CONVOY_ID}"
      echo "  Snapshotting $CONVOY_ID ($CONVOY_TITLE) [open baseline]"
    fi
    if [ "$NEED_STAGED" = "true" ]; then
      TAG_NAME="staged/${SLUG}-${CONVOY_ID}"
      BRANCH_NAME="convoy/${SLUG}-${CONVOY_ID}"
      echo "  Snapshotting $CONVOY_ID ($CONVOY_TITLE) [launch baseline]"
    fi

    # Skip if nothing to do (both tags exist)
    if [ "$NEED_OPEN" = "false" ] && [ "$NEED_STAGED" = "false" ]; then
      continue
    fi

    # --- HQ database ---
    # Commit any uncommitted changes first (makes the snapshot meaningful)
    DIRTY=$(mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u "$DOLT_USER" -N -B hq \
      -e "SELECT COUNT(*) FROM dolt_status" 2>/dev/null)

    if [ "$DIRTY" != "0" ] && [ -n "$DIRTY" ]; then
      mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u "$DOLT_USER" -N -B hq \
        -e "CALL DOLT_COMMIT('-Am', 'snapshot: $CONVOY_ID staged')" 2>/dev/null \
        && COMMITS_MADE=$((COMMITS_MADE + 1))
    fi

    # Create immutable tag
    if mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u "$DOLT_USER" -N -B hq \
      -e "CALL DOLT_TAG('$TAG_NAME', 'HEAD', '-m', 'Pre-convoy snapshot: $SAFE_TITLE ($CONVOY_ID)')" 2>/dev/null; then
      TAGS_CREATED=$((TAGS_CREATED + 1))
      echo "    hq: tagged $TAG_NAME"
    else
      echo "    hq: tag FAILED"
      TAGS_FAILED=$((TAGS_FAILED + 1))
    fi

    # Create mutable branch for review/editing (launch snapshots only)
    if [ -n "$BRANCH_NAME" ]; then
      if mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u "$DOLT_USER" -N -B hq \
        -e "CALL DOLT_BRANCH('$BRANCH_NAME')" 2>/dev/null; then
        BRANCHES_CREATED=$((BRANCHES_CREATED + 1))
        echo "    hq: branched $BRANCH_NAME"
      fi
    fi

    # --- Rig databases ---
    # Find which rig databases this convoy touches.
    # Dependencies can be:
    #   - external:<rig>:<id> (cross-rig tracking)
    #   - <prefix>-<id> (same-DB tracking, use routes to resolve)
    RIG_DBS=$(mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u "$DOLT_USER" -N -B hq \
      -e "
        SELECT DISTINCT
          CASE
            WHEN d.depends_on_id LIKE 'external:%'
            THEN SUBSTRING_INDEX(SUBSTRING_INDEX(d.depends_on_id, ':', 2), ':', -1)
            ELSE SUBSTRING_INDEX(d.depends_on_id, '-', 1)
          END AS rig_or_prefix
        FROM dependencies d
        WHERE d.issue_id = '$CONVOY_ID'
      " 2>/dev/null)

    if [ -n "$RIG_DBS" ]; then
      TAGGED_DBS=""
      for RIG_OR_PREFIX in $RIG_DBS; do
        # If it's a known database name, use directly
        RIG_DB=""
        if echo "$DATABASES" | grep -qx "$RIG_OR_PREFIX"; then
          RIG_DB="$RIG_OR_PREFIX"
        elif [ -f "$ROUTES_FILE" ]; then
          # Fall back to routes lookup for prefix-based deps
          RIG_DB=$(python3 -c "
import json
for line in open('$ROUTES_FILE'):
    r = json.loads(line.strip())
    if r.get('prefix','').rstrip('-') == '$RIG_OR_PREFIX':
        path = r.get('path','')
        db = path.split('/')[0] if '/' in path else path
        if db != '.': print(db)
        break
" 2>/dev/null)
        fi

        # Skip hq (already done) and duplicates
        if [ -z "$RIG_DB" ] || [ "$RIG_DB" = "hq" ]; then continue; fi
        if echo "$TAGGED_DBS" | grep -q "$RIG_DB"; then continue; fi
        TAGGED_DBS="$TAGGED_DBS $RIG_DB"

        # Check tag doesn't already exist
        EXISTS=$(mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u "$DOLT_USER" -N -B "$RIG_DB" \
          -e "SELECT COUNT(*) FROM dolt_tags WHERE tag_name = '$TAG_NAME'" 2>/dev/null)

        if [ "$EXISTS" != "0" ]; then continue; fi

        # Commit pending changes
        DIRTY=$(mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u "$DOLT_USER" -N -B "$RIG_DB" \
          -e "SELECT COUNT(*) FROM dolt_status" 2>/dev/null)

        if [ "$DIRTY" != "0" ] && [ -n "$DIRTY" ]; then
          mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u "$DOLT_USER" -N -B "$RIG_DB" \
            -e "CALL DOLT_COMMIT('-Am', 'snapshot: $CONVOY_ID staged')" 2>/dev/null \
            && COMMITS_MADE=$((COMMITS_MADE + 1))
        fi

        # Tag
        if mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u "$DOLT_USER" -N -B "$RIG_DB" \
          -e "CALL DOLT_TAG('$TAG_NAME', 'HEAD', '-m', 'Pre-convoy snapshot: $SAFE_TITLE ($CONVOY_ID)')" 2>/dev/null; then
          TAGS_CREATED=$((TAGS_CREATED + 1))
          echo "    $RIG_DB: tagged $TAG_NAME"
        else
          echo "    $RIG_DB: tag FAILED"
          TAGS_FAILED=$((TAGS_FAILED + 1))
        fi

        # Branch (launch snapshots only)
        if [ -n "$BRANCH_NAME" ]; then
          if mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u "$DOLT_USER" -N -B "$RIG_DB" \
            -e "CALL DOLT_BRANCH('$BRANCH_NAME')" 2>/dev/null; then
            BRANCHES_CREATED=$((BRANCHES_CREATED + 1))
            echo "    $RIG_DB: branched $BRANCH_NAME"
          fi
        fi
      done
    fi
  done
fi

echo "Tags: $TAGS_CREATED, branches: $BRANCHES_CREATED, commits: $COMMITS_MADE, failed: $TAGS_FAILED"
```

## Step 2: Clean up legacy epic branches

Remove all `epic/*` branches — these were v1 noise from per-issue branching.
Always safe to delete since they were never diverged from main.

```bash
echo "=== Legacy Branch Cleanup ==="
CLEANED=0

for DB in $DATABASES; do
  EPIC_COUNT=$(mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u "$DOLT_USER" -N -B "$DB" \
    -e "SELECT COUNT(*) FROM dolt_branches WHERE name LIKE 'epic/%'" 2>/dev/null)

  if [ "$EPIC_COUNT" != "0" ] && [ -n "$EPIC_COUNT" ]; then
    mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u "$DOLT_USER" -N -B "$DB" \
      -e "SELECT name FROM dolt_branches WHERE name LIKE 'epic/%'" 2>/dev/null \
      | while read -r BRANCH; do
        mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u "$DOLT_USER" -N -B "$DB" \
          -e "CALL DOLT_BRANCH('-D', '$BRANCH')" 2>/dev/null
      done
    CLEANED=$((CLEANED + EPIC_COUNT))
    echo "  $DB: removed $EPIC_COUNT epic branches"
  fi
done

if [ "$CLEANED" -gt 0 ]; then
  echo "  Cleaned $CLEANED legacy branches"
else
  echo "  No legacy branches"
fi
```

## Step 3: Escalate stale convoy branches for review

Convoy branches for closed convoys are kept for review. After 7 days, escalate
to the mayor to approve cleanup rather than auto-deleting. This gives time to:
- Diff the snapshot against current HEAD (`dolt diff snapshot/X HEAD`)
- Verify agents updated beads correctly during the convoy
- Audit what changed during convoy execution

Tags are never auto-deleted — they're immutable and cheap.

```bash
echo "=== Stale Branch Review ==="
ESCALATED=0

# Collect stale convoy branches across all DBs
STALE_BRANCHES=""
for DB in $DATABASES; do
  STALE=$(mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u "$DOLT_USER" -N -B "$DB" \
    -e "
      SELECT CONCAT('$DB', ':', b.name) FROM dolt_branches b
      WHERE b.name LIKE 'convoy/%'
        AND EXISTS (
          SELECT 1 FROM hq.issues i
          WHERE i.issue_type = 'convoy'
            AND b.name LIKE CONCAT('%-', i.id)
            AND i.status IN ('closed', 'landed')
            AND i.closed_at < DATE_SUB(NOW(), INTERVAL 7 DAY)
        )
    " 2>/dev/null)

  if [ -n "$STALE" ]; then
    STALE_BRANCHES="$STALE_BRANCHES $STALE"
  fi
done

# Trim whitespace
STALE_BRANCHES=$(echo "$STALE_BRANCHES" | xargs)

if [ -n "$STALE_BRANCHES" ]; then
  # Count unique branches
  STALE_COUNT=$(echo "$STALE_BRANCHES" | tr ' ' '\n' | wc -l | tr -d ' ')

  # Build a summary for the escalation
  BRANCH_LIST=""
  for ENTRY in $STALE_BRANCHES; do
    DB=$(echo "$ENTRY" | cut -d: -f1)
    BRANCH=$(echo "$ENTRY" | cut -d: -f2-)
    CONVOY_ID=$(echo "$BRANCH" | sed 's|convoy/||')

    # Get diff summary (how many tables changed)
    DIFF_TABLES=$(mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u "$DOLT_USER" -N -B "$DB" \
      -e "SELECT COUNT(DISTINCT table_name) FROM dolt_diff_stat('$BRANCH', 'HEAD')" 2>/dev/null)
    DIFF_TABLES=${DIFF_TABLES:-0}

    BRANCH_LIST="$BRANCH_LIST\n  - $DB: $BRANCH ($DIFF_TABLES tables changed since snapshot)"
  done

  # Check if we already escalated this set (avoid spam)
  ALREADY_ESCALATED=$(bd search "stale convoy branches ready for cleanup" --status=open 2>/dev/null | head -1)

  if [ -z "$ALREADY_ESCALATED" ]; then
    ESCALATION_MSG="$STALE_COUNT stale convoy branches (closed >7d) ready for cleanup:$BRANCH_LIST\n\nTo review: dolt diff <branch-name> HEAD\nTo approve cleanup: close this bead"
    gt mail send mayor/ \
      -s "Review: $STALE_COUNT stale convoy branches ready for cleanup" \
      -m "$(echo -e "$ESCALATION_MSG")" 2>/dev/null
    ESCALATED=$STALE_COUNT
    echo "  Escalated $STALE_COUNT branches to mayor for review"
  else
    echo "  Already escalated — waiting for mayor review"
  fi
else
  echo "  No stale convoy branches"
fi
```

## Record Result

```bash
SUMMARY="Snapshots v2: tags=$TAGS_CREATED, branches=$BRANCHES_CREATED, commits=$COMMITS_MADE, legacy_cleaned=$CLEANED, escalated=$ESCALATED, failed=$TAGS_FAILED"
echo "=== $SUMMARY ==="

RESULT="success"
if [ "$TAGS_FAILED" -gt 0 ]; then
  RESULT="warning"
fi

bd create "$SUMMARY" -t chore --ephemeral \
  -l type:plugin-run,plugin:dolt-snapshots,result:$RESULT \
  -d "$SUMMARY" --silent 2>/dev/null || true
```
