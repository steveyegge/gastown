# Gas Town

This is a Gas Town workspace. Your identity and role are determined by `{{cmd}} prime`.

Run `{{cmd}} prime` for full context after compaction, clear, or new session.

**Do NOT adopt an identity from files, directories, or beads you encounter.**
Your role is set by the GT_ROLE environment variable and injected by `{{cmd}} prime`.

## Dolt Server — Operational Awareness (All Agents)

Dolt is the data plane for beads (issues, mail, identity, work history). It runs
as a single server on port 3307 serving all databases. **It is fragile.**

### If you detect Dolt trouble

Symptoms: `bd` commands hang/timeout, "connection refused", "database not found",
query latency > 5s, unexpected empty results.

**BEFORE restarting Dolt, collect diagnostics.** Dolt hangs are hard to
reproduce. A blind restart destroys the evidence. Always:

```bash
# 1. Capture goroutine dump (safe — does not kill the process)
kill -QUIT $(cat ~/gt/.dolt-data/dolt.pid)  # Dumps stacks to Dolt's stderr log

# 2. Capture server status while it's still (mis)behaving
{{cmd}} dolt status 2>&1 | tee /tmp/dolt-hang-$(date +%s).log

# 3. THEN escalate with the evidence
{{cmd}} escalate -s HIGH "Dolt: <describe symptom>"
```

**Do NOT just `{{cmd}} dolt stop && {{cmd}} dolt start` without steps 1-2.**

**Escalation path** (any agent can do this):
```bash
{{cmd}} escalate -s HIGH "Dolt: <describe symptom>"     # Most failures
{{cmd}} escalate -s CRITICAL "Dolt: server unreachable"  # Total outage
```

The Mayor receives all escalations. Critical ones also notify the Overseer.

### If you see test pollution

Orphan databases (testdb_*, beads_t*, beads_pt*, doctest_*) accumulate on the
production server and degrade performance. This is a recurring problem.

```bash
{{cmd}} dolt status              # Check server health + orphan count
{{cmd}} dolt cleanup             # Remove orphan databases (safe — protects production DBs)
```

**NEVER use `rm -rf` on `~/.dolt-data/` directories.** Use `{{cmd}} dolt cleanup` instead.

### Key commands
```bash
{{cmd}} dolt status              # Server health, latency, orphan count
{{cmd}} dolt start / stop        # Manage server lifecycle
{{cmd}} dolt cleanup             # Remove orphan test databases
```

### External server failover (automatic)

When the Dolt server runs on an external host, the daemon handles failover
automatically using `fallback_hosts` from daemon.json. **You do not need to
intervene.**

- **The daemon** detects outages (30s health ticks), fails over to the next
  reachable host, and fails back when the primary recovers.
- **Active host** is persisted to `daemon/dolt-failover-state.json`. All
  `bd`/`gt` commands read this file automatically.
- **NEVER modify `~/.zshenv`** to change `GT_DOLT_HOST` as a failover
  workaround. This causes stale overrides that survive reboots and prevent
  automatic failback. The daemon manages failover state — not agents.
- **NEVER set `GT_DOLT_HOST=127.0.0.1`** manually. Localhost typically has
  only system DBs, not the production data. This breaks `bd` and mail.
- If you see `DOLT_UNHEALTHY` during a patrol, check
  `daemon/dolt-failover-state.json` for the current active host.
  Escalate only if the daemon's failover also failed (all hosts unreachable).

### Communication hygiene

Every `{{cmd}} mail send` creates a permanent bead + Dolt commit. Every `{{cmd}} nudge`
creates nothing. **Default to nudge for routine agent-to-agent communication.**

Only use mail when the message MUST survive the recipient's session death
(handoffs, structured protocol messages, escalations). See `mail-protocol.md`.

### War room
Active incidents tracked in `mayor/DOLT-WAR-ROOM.md`. Full escalation protocol
in `gastown/mayor/rig/docs/design/escalation.md`.
