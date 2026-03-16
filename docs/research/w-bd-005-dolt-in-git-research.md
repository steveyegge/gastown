# w-bd-005: Dolt-in-Git Integration Research Report

**Date**: 2026-03-15
**Author**: zhora (gastown crew)
**Status**: Research complete — feature has shipped upstream

---

## 1. What is "Dolt-in-Git"?

"Dolt-in-Git" refers to Dolt's ability to use Git repositories as Dolt remotes.
Introduced in **Dolt v1.81.10** (February 2026), this feature allows a Dolt
database to push and pull its data to/from a standard Git repository (GitHub,
GitLab, or local bare repos) without requiring DoltHub or DoltLab.

Dolt stores its data on a custom Git ref (`refs/dolt/data` by default). This ref
is invisible to normal Git operations — `git clone`, `git fetch`, and `git pull`
will not touch it. The Dolt data coexists with source code in the same Git repo
without interference.

**Key blog posts:**
- [Announcing Git remote support in Dolt](https://www.dolthub.com/blog/2026-02-13-announcing-git-remote-support-in-dolt/) (2026-02-13)
- [Supporting Git remotes as Dolt remotes](https://www.dolthub.com/blog/2026-02-19-supporting-git-remotes-as-dolt-remotes/) (2026-02-19)

The first blog post explicitly mentions beads as a motivating use case:
> "The first reason for adding Git remote support in Dolt was to better support
> Dolt's integration into Beads, Steve Yegge's foundational agentic memory
> implementation."

---

## 2. Current Status: SHIPPED (with caveats)

The feature is **shipped and available** in Dolt >= v1.81.10. Gas Town already
requires Dolt >= 1.82.4 (see `internal/deps/dolt.go:14`), so every Gas Town
installation has this capability today.

### Known issues (from dolthub/dolt GitHub)

| Issue | Status | Impact |
|-------|--------|--------|
| [#10537](https://github.com/dolthub/dolt/issues/10537) — Push perf: 45-80s for tiny DBs | **Fixed** in 1.82.4 (down to ~15s) | Still slow for frequent syncs |
| [#10651](https://github.com/dolthub/dolt/issues/10651) — Fetch fails on large refs/dolt/data | **Fixed** | 6.7GB+ databases hit Git packfile limits |
| [#10486](https://github.com/dolthub/dolt/issues/10486) — Fails when Git needs STDIN creds | **Open** | Requires SSH key or credential helper |
| [#10525](https://github.com/dolthub/dolt/issues/10525) — No Git UI visibility | **Open** | Users can't see Dolt data in GitHub UI |

The feature is functional but still maturing. Performance for large databases and
Git provider limits on ref sizes are active areas of work.

---

## 3. How It Works (Technical Architecture)

### Storage model

```
your-git-repo/
├── .git/
│   ├── refs/
│   │   ├── heads/main          # Normal Git branches (source code)
│   │   └── dolt/data           # Dolt database storage (hidden from Git)
│   └── objects/                # Git object store (shared with Dolt blobs)
├── src/                        # Your source code (untouched)
└── ...
```

Dolt maps its internal Noms Block Store (NBS) onto Git objects:
- **Blobs** = individual Dolt tablefiles and manifest files
- **Trees** = directory structures containing blob references
- **Commits** = snapshots with parent pointers
- Files exceeding provider limits are automatically chunked into sub-trees

### Concurrency model

Uses optimistic concurrency control (compare-and-swap):
1. Fetch remote state into `remoteTrackingRef`
2. Build new commit on top of remote head
3. Push with `git push --force-with-lease` (atomic)
4. On conflict: refetch + retry with exponential backoff

### Commands

**CLI mode:**
```bash
dolt remote add origin https://github.com/org/repo.git
dolt push origin main
dolt pull origin main
```

**SQL server mode (via stored procedures):**
```sql
CALL dolt_clone('https://github.com/org/repo.git');
CALL dolt_push('origin', 'main');
```

### Requirements
- Git binary must be on PATH
- Target Git repository must already exist with at least one branch
- SSH key or credential helper auth (no STDIN password prompts)
- Configurable ref name via `--ref` flag on remote add/clone

---

## 4. What Would Integration with Beads Mean?

### Current architecture

Gas Town runs a standalone Dolt SQL server on port 3307 serving all rig databases
from `~/gt/.dolt-data/`:

```
~/gt/.dolt-data/
├── hq/           # Town beads (hq-*)
├── gastown/      # Gastown rig (gt-*)
├── beads/        # Beads rig (bd-*)
├── config.yaml   # Server config (port, timeouts)
├── dolt.pid      # PID file
└── sql-server.log
```

Each rig database is a separate Dolt repository within this directory. The server
handles multi-client access, connection pooling, and concurrent writes. Sync is
handled by `gt dolt sync` which calls `dolt push` per database.

Key touchpoints in the codebase:
- `internal/doltserver/doltserver.go` — Server lifecycle (Start/Stop/Status)
- `internal/doltserver/sync.go` — Push databases to remotes
- `internal/beads/` — Beads CLI wrapper (bd commands)
- `internal/beads/routes.go` — Prefix-to-path routing (gt-, bd-, hq-)
- `internal/deps/dolt.go` — MinDoltVersion = 1.82.4

### Integration scenario: Git remotes for rig databases

Instead of syncing to DoltHub or file-based remotes, each rig database would
push to a Git remote (e.g., the gastown GitHub repo itself):

```bash
# Current: each DB has its own DoltHub/file remote
cd ~/.dolt-data/gastown && dolt remote -v
# origin  file:///path/to/backup  {}

# Proposed: point to the Git repo
cd ~/.dolt-data/gastown && dolt remote add origin git@github.com:steveyegge/gastown.git
# Data stored under refs/dolt/gastown (custom --ref per database)
```

This means beads data would travel with the Git repo on clone/fork — no separate
DoltHub account or backup infrastructure needed.

### Integration scenario: Embedded mode (no server)

A more ambitious integration would eliminate the standalone Dolt server entirely.
Each `.beads/` directory in a rig could be a Dolt database backed by the repo's
own `.git/` directory. This would require Dolt's embedded (non-server) mode,
which is single-writer and incompatible with Gas Town's multi-agent architecture.

**This scenario is NOT feasible** with the current feature. The Git remote
support is a remote storage backend, not an embedded database mode.

---

## 5. Benefits

### Immediate (using Git remotes for sync)

1. **No DoltHub dependency** — Sync to GitHub/GitLab instead of DoltHub. One
   fewer service to manage, no DoltHub account needed.

2. **Data travels with code** — When someone clones/forks the gastown repo, they
   could optionally pull down beads data too. Useful for disaster recovery.

3. **Unified backup** — GitHub's backup infrastructure covers both code and data.
   No separate backup pipeline for Dolt databases.

4. **Simpler onboarding** — `gt upgrade` could configure Git remotes
   automatically since the Git repo URL is already known.

5. **Federated ledger access** — As noted in `docs/design/ledger-export-triggers.md`,
   dolt-in-git enables federated ledger queries across towns without a central
   DoltHub instance.

### Not available (would require embedded mode)

- Eliminating the standalone Dolt server (still needed for multi-client SQL)
- Zero-config beads (still need `gt dolt start`)
- Git-native storage (data is still in `~/.dolt-data/`, just synced to Git)

---

## 6. Risks

### Performance

- **Push latency**: ~15s per push for small databases (post-1.82.4 fix). With 7
  databases, a full sync takes ~2 minutes. Acceptable for periodic sync, too slow
  for real-time.
- **Large database failure**: Databases > 6GB hit Git packfile limits
  ([#10651](https://github.com/dolthub/dolt/issues/10651)). Beads databases are
  currently small, but this is a growth ceiling.
- **Git provider limits**: GitHub has repo size recommendations (< 5GB). Dolt
  data under `refs/dolt/data` counts toward this.

### Operational

- **Git repo bloat**: Dolt data is append-only. Old table chunks accumulate as
  Git objects. `git gc` may not prune `refs/dolt/` data effectively. Over months,
  this could bloat clone times for all repo users.
- **Credential complexity**: Requires SSH key or credential helper. STDIN
  password auth is broken ([#10486](https://github.com/dolthub/dolt/issues/10486)).
  Gas Town already uses SSH keys, so low risk here.
- **Multi-database refs**: Each rig database needs its own ref
  (e.g., `refs/dolt/hq`, `refs/dolt/gastown`). This is supported via `--ref`
  but adds configuration surface area.

### Migration

- **Not a drop-in replacement**: Requires changes to `internal/doltserver/sync.go`
  to configure Git remotes instead of file/DoltHub remotes.
- **Backward compatibility**: Existing installations with DoltHub remotes would
  need migration logic in `gt upgrade`.
- **No rollback story**: Once beads data is on a Git remote, removing it
  requires `git push origin :refs/dolt/data` (force-delete a ref). Not
  catastrophic but not trivial either.

### Feature maturity

- Shipped 1 month ago (Feb 2026). Only 3 issues filed so far, but early adopter
  base is small.
- The DoltHub team is actively iterating (performance fixes landed in 1.82.4).
- No stability guarantees on the `refs/dolt/data` format yet.

---

## 7. Recommendation

### Plan for it. Do not wait, but do not rush.

**Short term (now):** Do nothing. The feature works but has rough edges
(performance, large DB limits, no UI visibility). The standalone Dolt server
architecture is stable and well-tested.

**Medium term (Q2 2026):** Add Git remote support as an **optional sync backend**
alongside the existing DoltHub/file remotes. Implementation steps:

1. Add `git` as a remote type in `SyncDatabases()` (sync.go)
2. Add `gt dolt remote add --git <repo-url>` command
3. Configure per-database refs: `--ref refs/dolt/<dbname>`
4. Update `gt upgrade` to offer Git remote migration
5. Test with gastown's own GitHub repo as the remote

This is a ~2-3 day implementation task, mostly in `internal/doltserver/sync.go`
and the `gt dolt remote` CLI commands.

**Long term (when Dolt stabilizes the format):** Make Git remotes the default
sync backend for new installations. Keep DoltHub support for users who want it.

**What to ignore:** The embedded-mode dream (no Dolt server) is not what this
feature delivers. The standalone Dolt SQL server is still required for
multi-agent concurrent access. Do not architect around eliminating it.

### Specific action items

- [ ] Monitor [dolthub/dolt#10525](https://github.com/dolthub/dolt/issues/10525)
  (Git UI visibility) — this would make the feature much more useful
- [ ] Monitor push performance improvements — 15s/push is the current floor
- [ ] Test `refs/dolt/data` with gastown repo to measure size impact
- [ ] File issue for `gt dolt sync --backend=git` feature
- [ ] Update `docs/design/ledger-export-triggers.md` Phase 4 with concrete plan

---

## Appendix: Source References

### External
- [Announcing Git remote support in Dolt](https://www.dolthub.com/blog/2026-02-13-announcing-git-remote-support-in-dolt/) — Feature announcement
- [Supporting Git remotes as Dolt remotes](https://www.dolthub.com/blog/2026-02-19-supporting-git-remotes-as-dolt-remotes/) — Technical deep dive
- [dolthub/dolt#10537](https://github.com/dolthub/dolt/issues/10537) — Push performance (fixed)
- [dolthub/dolt#10651](https://github.com/dolthub/dolt/issues/10651) — Large ref fetch failure (fixed)
- [dolthub/dolt#10486](https://github.com/dolthub/dolt/issues/10486) — STDIN credential bug (open)
- [dolthub/dolt#10525](https://github.com/dolthub/dolt/issues/10525) — Git UI visibility (open)

### Codebase
- `internal/doltserver/doltserver.go` — Server lifecycle, port 3307, ~/.dolt-data/ layout
- `internal/doltserver/sync.go` — Database sync (push/pull) implementation
- `internal/beads/routes.go` — Prefix-to-path routing
- `internal/deps/dolt.go` — MinDoltVersion = 1.82.4 (Git remotes available)
- `docs/design/ledger-export-triggers.md` — References dolt-in-git in Phase 4
