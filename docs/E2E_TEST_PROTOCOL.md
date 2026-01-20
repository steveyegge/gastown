# Gas Town E2E Test Protocol

**Last Updated:** 2026-01-20
**Status:** All tests passing (18/18)

This document describes how to run end-to-end tests for Gas Town, including
testing the custom types fix for multi-repo routing and the role slot cross-database fix.

---

## Bugs Fixed in This Release

### 1. Custom Types Not Configured in Routed Database

**Symptom:**
```
Error: validation failed: invalid issue type: agent
```

**Root Cause:**
When creating agent beads, the bead ID's prefix routes the write to a different database than expected. If that target database doesn't have `types.custom` configured, bd rejects the "agent" type.

**Fix:**
- Created `internal/beads/beads_types.go` with:
  - `FindTownRoot()` - walks up directory tree to find town root
  - `ResolveRoutingTarget()` - determines which .beads dir a bead ID routes to
  - `EnsureCustomTypes()` - configures custom types with two-level caching
- Modified `CreateAgentBead()` to ensure custom types in target db before creating
- Two-level caching: in-memory map + `.gt-types-configured` sentinel file

### 2. Role Slot Cross-Database Lookup Failure

**Symptom:**
```
Warning: could not set role slot: issue not found
```

**Root Cause:**
When creating a crew agent bead:
1. The agent bead (e.g., `fr-flow_rig-crew-testworker`) is created in the **rig's** beads database
2. The code tried to set a slot linking to `hq-crew-role` (the town-level role bead)
3. The `slot set` command ran from the Beads wrapper's default directory
4. bd couldn't find the agent bead because it was routed to a different database

**Fix:**
- Added `runSlotSet()` and `runSlotClear()` helpers in `beads_agent.go`
- These helpers run bd from the correct target directory where the bead exists
- `CreateAgentBead` and `CreateOrReopenAgentBead` now use `targetDir` for slot operations

**Verification:**
```bash
cd /code/e2e-flow-test/flow_rig/.beads && bd slot show fr-flow_rig-crew-testworker
# Output shows: role: hq-crew-role
```

---

## Quick Start

```bash
# Build gt and run simple E2E test
cd /code/gt-pr
go build -o /tmp/gt-test ./cmd/gt
chmod +x scripts/e2e-simple.sh
./scripts/e2e-simple.sh /code/2
```

## Test Scripts

### Simple E2E Test (`scripts/e2e-simple.sh`)

A streamlined test that covers the core functionality:

1. Install new town
2. Add a rig
3. Add crew worker (KEY TEST for custom types fix)
4. Show bead (existing agent bead)
5. Verify custom types sentinel files
6. Ready work
7. Trail

```bash
./scripts/e2e-simple.sh [TEST_DIR]
# Example: ./scripts/e2e-simple.sh /tmp/gt-test-town
```

### Full E2E Test (`scripts/e2e-test.sh`)

A comprehensive test with detailed logging:

```bash
./scripts/e2e-test.sh [TEST_DIR] [GT_BINARY]
# Example: ./scripts/e2e-test.sh /code/2 /tmp/gt-test
```

Features:
- Detailed logging to `TEST_DIR/e2e-test.log`
- Pass/fail/skip tracking
- Multi-repo routing test
- Color-coded output

## Testing the Custom Types Fix

The custom types fix addresses the "invalid issue type: agent" error that occurs
when creating agent beads (crew workers, polecats) in a multi-repo routing scenario.

### The Bug

When `gt crew add` creates an agent bead, the bead ID's prefix determines which
database the write routes to. If that target database doesn't have `types.custom`
configured, bd fails with:

```
Error: validation failed: invalid issue type: agent
```

### How to Test the Fix

1. **Basic Test**: Run the E2E test - the crew worker creation will fail if the
   fix doesn't work:

   ```bash
   ./scripts/e2e-simple.sh /code/2
   # Watch for: "Test 3: Add Crew Worker (Custom Types Fix Test)"
   ```

2. **Manual Reproduction Test**:

   ```bash
   # 1. Create a fresh town
   rm -rf /tmp/gt-test-town
   mkdir /tmp/gt-test-town
   cd /tmp/gt-test-town
   gt install --name "test-town" .

   # 2. Check if types.custom is configured
   cd .beads && bd config get types.custom

   # 3. Remove sentinel to test fresh behavior
   rm -f .gt-types-configured

   # 4. Remove custom types to reproduce the bug
   bd config set types.custom ""

   # 5. Try to add a crew worker
   cd /tmp/gt-test-town
   gt crew add testworker
   # With the fix: succeeds
   # Without the fix: "invalid issue type: agent"
   ```

3. **Multi-Repo Routing Test**:

   ```bash
   # 1. Set up a town with a rig that routes to a different beads DB
   # (This requires a more complex setup with routes.jsonl configured)

   # 2. Add a crew worker - the bead ID will route to the rig's beads DB
   gt crew add bob

   # 3. The fix ensures custom types are configured in the TARGET db
   #    before creating the bead
   ```

### Verifying the Fix

After running the fix, check for:

1. **Sentinel file**: `.beads/.gt-types-configured` should exist in the target
   beads directory

2. **Custom types**: `bd config get types.custom` should show the custom types list

3. **Successful bead creation**: `gt crew add <name>` should succeed

## Test Coverage

| Component | Test | Status |
|-----------|------|--------|
| Town Install | `gt install` | E2E |
| Rig Management | `gt rig add` | E2E |
| Crew Workers | `gt crew add` | E2E (tests custom types fix) |
| Bead Show | `gt show` | E2E |
| Custom Types | sentinel file check | E2E |
| Ready Work | `gt ready` | E2E |
| Trail | `gt trail` | E2E |

## Unit Tests

The custom types fix has dedicated unit tests:

```bash
cd /code/gt-pr
go test ./internal/beads/ -run "TestFindTownRoot|TestResolveRoutingTarget|TestEnsureCustomTypes|TestBeads_getTownRoot" -v
```

Run all beads tests:

```bash
go test ./internal/beads/... -v
```

## Troubleshooting

### "invalid issue type: agent" Error

This error means custom types weren't configured in the target beads database.

1. Check which database the bead is routing to (based on ID prefix)
2. Verify the sentinel file exists in that database's `.beads/` directory
3. Check if `types.custom` is configured: `bd config get types.custom`

### E2E Test Failures

1. Check the log file: `TEST_DIR/e2e-test.log`
2. Run individual commands manually to isolate the failure
3. Ensure `bd` is in your PATH and working

### Build Failures

```bash
# Verify compilation
cd /code/gt-pr
go build ./...

# Check for issues
go vet ./...
```

---

## Comprehensive Bead Flow Test

### Purpose

The `e2e-bead-flow.sh` script provides comprehensive end-to-end testing that verifies the complete lifecycle of beads flowing through Gas Town.

### Location

```bash
/code/gt-pr/scripts/e2e-bead-flow.sh
```

### Test Phases

#### Phase 1: Setup
- Build gt binary
- Create test directory
- Create source git repository for rig
- Install Gas Town (`gt install`)
- Add test rig (`gt rig add`)
- Verify custom types configured (sentinel files)

#### Phase 2: Work Bead Creation
- Create a task bead with correct rig prefix
- Verify bead exists via `bd show`
- Check bead status is OPEN
- Check bead type is task

#### Phase 3: Agent Bead Verification
- List all agent beads in rig
- Verify witness agent exists with correct ID pattern
- Verify witness has role slot set
- Verify refinery agent exists
- Verify refinery has role slot set

#### Phase 4: Crew Worker Test
- Add crew worker via `gt crew add`
- Verify crew agent bead created
- Verify crew has role slot set (the critical cross-database test)

#### Phase 5: Polecat Sling Test
- Attempt to sling work to rig (auto-spawns polecat)
- Verify polecat agent bead created (if daemon running)
- Skip gracefully in CI environments

#### Phase 6: State Consistency
- Count all agent beads
- Verify no agents have missing role slots
- Verify `bd list` succeeds without errors
- Verify town-level agents (mayor, deacon) exist

### Test Results (2026-01-20)

```
═══════════════════════════════════════════════════════════════
  TEST SUMMARY
═══════════════════════════════════════════════════════════════

  Passed:  18
  Failed:  0
  Total:   18

  Test directory: /code/e2e-flow-test

All tests passed!
```

### Individual Test Results

| Test | Status |
|------|--------|
| Town installed | PASS |
| Rig added | PASS |
| Custom types configured (town) | PASS |
| Custom types configured (rig) | PASS |
| Setup complete | PASS |
| Work bead created: fr-test-001 | PASS |
| Work bead status: open | PASS |
| Work bead type: task | PASS |
| Witness agent exists | PASS |
| Witness role slot set | PASS |
| Refinery agent exists | PASS |
| Refinery role slot set | PASS |
| Crew worker added | PASS |
| Crew agent bead exists | PASS |
| Crew role slot set | PASS |
| All agents have role slots | PASS |
| bd list succeeds without errors | PASS |
| Town has mayor and deacon agents | PASS |

---

## Debugging Process

This section documents issues encountered during testing and their solutions.

### Issue 1: Invalid Rig Name

**Error:**
```
rig name "test-rig" contains invalid characters; hyphens, dots, and spaces are reserved
```

**Fix:** Changed rig name from "test-rig" to "test_rig" (underscores allowed)

### Issue 2: Missing --rig Flag

**Error:**
```
could not determine rig (use --rig flag)
```

**Fix:** Added `--rig flow_rig` to `gt crew add` command when not running from within a rig directory

### Issue 3: Prefix Parsing

**Error:** `bd config get prefix` returned "prefix (not set)" instead of actual prefix

**Fix:** Parse directly from config.yaml:
```bash
grep "^prefix:" config.yaml | awk '{print $2}'
```

### Issue 4: Polecat Spawn Command

**Error:** `gt polecat spawn` command doesn't exist as standalone

**Fix:** Polecats spawn automatically via `gt sling <bead> <rig> --no-attach`

### Issue 5: Script Hanging

**Error:** Script with `set -euo pipefail` would start but produce no output

**Fix:** Simplified to `set -e` only

---

## Key Learnings

### Multi-Repo Routing Architecture

Gas Town uses a sophisticated routing system where bead IDs determine their storage location:
- `hq-*` beads route to town-level `.beads/`
- `<rig-prefix>-*` beads route to rig-level `.beads/`

### Critical Considerations for Bead Operations

1. **Target Database Awareness**: The bead may be written to a different database than the current working directory
2. **Custom Types Prerequisite**: That database may not have custom types configured
3. **Cross-Database Slot Operations**: References (like role slots) need to run bd from the correct directory
4. **Sentinel Files**: `.gt-types-configured` provides fast cache validation

### Test Design Principles

1. Always verify both creation AND slot operations
2. Check sentinel files exist for custom types
3. Test cross-database scenarios (crew agents referencing town-level roles)
4. Handle graceful degradation (polecat tests skip without daemon)
5. Verify state consistency at the end

---

## Files Modified in This Fix

### New Files
- `internal/beads/beads_types.go` - Custom types routing logic
- `internal/beads/beads_types_test.go` - Unit tests
- `scripts/e2e-bead-flow.sh` - Comprehensive E2E test

### Modified Files
- `internal/beads/beads.go` - Added townRoot caching
- `internal/beads/beads_agent.go` - Added runSlotSet/runSlotClear helpers
- `scripts/e2e-simple.sh` - Fixed rig naming and crew add
- `CLAUDE.md` - Bug documentation and mission statement

---

## Running All Tests

```bash
# Quick verification
./scripts/e2e-simple.sh /code/test-dir

# Comprehensive bead flow test
./scripts/e2e-bead-flow.sh /code/e2e-flow-test

# Unit tests for beads module
go test ./internal/beads/...

# Full test suite
go test ./...

# Verify compilation
go build ./...
go vet ./...
```
