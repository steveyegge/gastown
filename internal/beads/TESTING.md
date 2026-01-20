# BeadsOps Testing

This document describes the testing approach for the `BeadsOps` interface.

## Conformance Testing

BeadsOps uses a conformance test pattern where the same test suite runs against multiple implementations:

1. **Fake** - In-memory test double (`FakeBeadsOps`)
2. **Real/FromRig** - Real `bd` CLI, running from rig directory
3. **Real/FromTown** - Real `bd` CLI, running from town root

This ensures the test double accurately models real `bd` behavior.

## Directory Permutations

The Real implementations test from two directory contexts to catch routing bugs:

### Real/FromRig
- BeadsOps created with `workDir` = rig path
- Direct access to rig's `.beads` database
- Both town and rig databases are initialized (matching production)

### Real/FromTown
- BeadsOps created with `workDir` = town root
- Has its own `.beads` database with `hq-` prefix
- Rig database also exists with `tr-` prefix
- Routes.jsonl is configured

This setup catches bugs where implementations:
1. Check for a local database (find the town db)
2. Then try to use routing (which may behave differently)

## Key Insight: ListByLabel is Rig-Scoped

**`ListByLabel` does NOT use routes.jsonl for routing.** It only queries the local `.beads` database in the working directory.

This means:
- `bd list --label=queued` from town root only sees town beads
- `bd list --label=queued` from rig directory only sees rig beads
- A town-wide query requires iterating over each rig separately

For the Queue implementation, this is acceptable because `--queue` requires a rig target, so we always operate within a single rig context.

## Cross-Context Conformance Testing

The `TestBeadsOps_CrossContext_Conformance` test verifies that beads created in one context (rig) are NOT visible via ListByLabel from another context (town), and vice versa.

### How Fake Models Rig-Scoping

`FakeBeadsOps` models rig-scoping through:
1. **Instance isolation**: Each `FakeBeadsOps` instance has its own in-memory store
2. **Routing support**: Use `AddRoute(prefix, target)` to route creates/gets to another store

For cross-context testing:
- Create a "town" FakeBeadsOps with prefix "hq-": `NewFakeBeadsOpsWithPrefix("hq-")`
- Create a "rig" FakeBeadsOps with prefix "tr-": `NewFakeBeadsOpsWithPrefix("tr-")`
- Add route from town to rig: `townFake.AddRoute("tr-", rigFake)`

This matches Real behavior where:
- `bd list` only queries the local database (rig-scoped)
- `bd create --id=tr-xxx` routes to the rig via routes.jsonl
- `bd show tr-xxx` routes to the rig via routes.jsonl

## Create Routing Conformance Testing

The `TestBeadsOps_CreateRouting_Conformance` test verifies that creates with prefixed IDs route correctly:
- Create from town with rig prefix → routes to rig store
- Create from town with town prefix → stays in town store
- GetBead follows routing (can find routed beads)
- ListByLabel does NOT follow routing (local only)

## Running Tests

```bash
# Run unit tests only (Fake implementation)
go test ./internal/beads/...

# Run integration tests (Fake + Real implementations)
go test -tags=integration ./internal/beads/...

# Run conformance tests specifically
go test -tags=integration ./internal/beads/... -run TestBeadsOps_Conformance -v
```

## Adding New BeadsOps Methods

When adding a new method to `BeadsOps`:

1. Add the method to the `BeadsOps` interface in `beads_ops.go`
2. Implement in `RealBeadsOps` (beads_ops_real.go)
3. Implement in `FakeBeadsOps` (beads_ops_fake.go)
4. Add conformance tests in `beads_ops_test.go` under `runConformanceTests`
5. Run both unit and integration tests to verify parity

## Test Environment Setup

The `setupTestTown` helper creates a production-like environment:

```
tmpdir/town/
├── .beads/
│   ├── config.yaml (prefix: hq)
│   ├── issues.jsonl
│   └── routes.jsonl (tr- -> testrig/mayor/rig)
└── testrig/mayor/rig/
    └── .beads/
        ├── config.yaml (prefix: tr)
        └── issues.jsonl
```

Both `setupRealBeadsOpsFromRig` and `setupRealBeadsOpsFromTown` use this structure, ensuring tests run against a realistic multi-database configuration.
