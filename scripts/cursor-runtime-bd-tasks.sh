#!/usr/bin/env bash
# Create beads issues for the Cursor runtime parity + onboarding plan.
# Idempotent: skips if an open epic with label cursor-runtime+plan already exists.
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || true)"
if [[ -z "$ROOT" ]] || [[ ! -d "$ROOT/.beads" ]]; then
  echo "Run from a gastown clone with beads initialized (bd init or bd bootstrap)." >&2
  exit 1
fi
cd "$ROOT"

if ! command -v bd >/dev/null 2>&1; then
  echo "bd (beads) not on PATH. Install: https://github.com/steveyegge/beads" >&2
  exit 1
fi

existing="$(bd count --status=open -l cursor-runtime -l plan 2>/dev/null | tr -d '[:space:]')"
if [[ -n "${existing:-}" ]] && [[ "${existing}" != "0" ]]; then
  echo "Open issues with labels cursor-runtime+plan already exist (count=$existing); skip."
  exit 0
fi

EPIC="$(bd create --silent "Epic: Cursor runtime parity + onboarding (plan)" --type=epic -p 1 -l cursor-runtime -l plan \
  -d "Umbrella: Go parity, user-facing docs (Cursor clarity), .cursor onboarding, smoke test. Plan: .cursor/plans/cursor_runtime_parity_df5a36d7.plan.md")"
echo "Created epic $EPIC"

T1="$(bd create --silent "cursor-runtime: Verify cursor-agent CLI (binary, -f, resume, hooks path)" --type=task -p 2 -l cursor-runtime --parent "$EPIC" \
  -d "Match AgentCursor in internal/config/agents.go to current Cursor docs and cursor-agent --help.")"
T2="$(bd create --silent "cursor-runtime: Tune ReadyDelayMs and/or ReadyPromptPrefix in agents.go" --type=task -p 2 -l cursor-runtime --parent "$EPIC" \
  -d "Avoid race on cold start + nudge poller; see internal/tmux/tmux.go WaitForRuntimeReady. Plan §3.")"
T3="$(bd create --silent "cursor-runtime: Process hygiene — orphan, doctor (YOLO signatures), down" --type=task -p 2 -l cursor-runtime --parent "$EPIC" \
  -d "orphan.go + down.go: cursor-agent/copilot. doctor: per-agent YOLO args. Plan §1.")"
T4="$(bd create --silent "cursor-runtime: Web API — detect cursor-agent and copilot in pane" --type=task -p 2 -l cursor-runtime --parent "$EPIC" \
  -d "internal/web/api.go isClaudeRunningInSession. Plan §2.")"
T5="$(bd create --silent "cursor-runtime: Docs + CLI — Cursor clarity and full built-in lists" --type=task -p 2 -l cursor-runtime --parent "$EPIC" \
  -d "§4+§4b: internal/cmd/config.go help (full built-ins incl. cursor). README prerequisites (Cursor Agent CLI). docs/INSTALLING.md, docs/reference.md — preset cursor vs binaries cursor-agent/agent; align lists with README (pi, omp). Optional otel GT_AGENT. Plan §4, §4b.")"
T6="$(bd create --silent "cursor-runtime: Tests for cursor preset + related packages (§11)" --type=task -p 2 -l cursor-runtime --parent "$EPIC" \
  -d "TestEnsureSettingsForRole_CursorUsesWorkDir; orphan/down, web, doctor tests; go test ./internal/config/... ./internal/hooks/... ./internal/crew/... ./internal/tmux/... ./internal/runtime/... Plan §6, §11.")"
T7="$(bd create --silent "cursor-runtime: Add .cursor/skills/ (Gas Town + cursor-agent ops)" --type=task -p 2 -l cursor-runtime --parent "$EPIC" \
  -d "Project skills per plan §7.")"
T8="$(bd create --silent "cursor-runtime: Add .cursor/README.md onboarding + manual smoke test" --type=task -p 2 -l cursor-runtime --parent "$EPIC" \
  -d "Primary entry for Cursor users; plan §8, §9.")"
T9="$(bd create --silent "cursor-runtime: Link beads coordination doc from .cursor/README.md" --type=task -p 3 -l cursor-runtime --parent "$EPIC" \
  -d "Link docs/cursor-runtime-beads-tasks.md from .cursor/README.md. Plan §10.")"

bd dep add "$T2" "$T1"
bd dep add "$T6" "$T3"
bd dep add "$T6" "$T4"
bd dep add "$T8" "$T7"

echo "Created tasks under $EPIC: $T1 $T2 $T3 $T4 $T5 $T6 $T7 $T8 $T9"
echo "Next: bd vc commit (and your team's Dolt/git backup workflow)."
