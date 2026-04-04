# Cursor runtime plan — Beads tasks (handoff)

These issues track **Cursor runtime parity**, **user-facing documentation clarity** (preset `cursor` vs CLI `cursor-agent` / `agent`), and **`.cursor/` onboarding**. Issue IDs vary by database.

**Create issues (idempotent — skips if open `cursor-runtime`+`plan` issues exist):**

```bash
./scripts/cursor-runtime-bd-tasks.sh
```

**Full task scope:** see **§10a** and **§4b** in the Cursor parity plan (`cursor_runtime_parity_df5a36d7.plan.md` in your editor plans folder).

**T5 (docs + CLI)** explicitly covers:

- `gt config` / `internal/cmd/config.go` help — list **all** built-in presets, not only claude/gemini/codex.
- **README** prerequisites — optional **Cursor Agent CLI** install; clarify **preset `cursor`** vs binaries.
- **docs/INSTALLING.md**, **docs/reference.md** — same built-in lists as README; short note on **`cursor`** → `cursor-agent`.

**Contributing:** [`CONTRIBUTING.md`](../CONTRIBUTING.md). Do not add `.beads/issues.jsonl` at repo root (CI). `bd vc commit` when persisting beads DB changes.

**Migration:** If you seeded tasks with an older script, **retitle T5** in `bd` to match the table in the plan §10a, or close duplicates.
