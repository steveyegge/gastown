# OpenCode E2E Test Infrastructure History

## Overview
This document tracks the evolution and debugging of the End-to-End (E2E) testing infrastructure for OpenCode integration in Gastown. The goal is to verify that Gastown can orchestrate OpenCode agents (using `opencode` CLI) to perform real coding tasks.

## Key Challenges & Solutions

### 1. Beads Version & Path Management
- **Issue:** Tests were picking up an old `bd` binary (v0.36.0) from system PATH instead of the required dev version (v0.48.0) installed in `~/go/bin`.
- **Symptoms:** Missing features, incorrect behavior.
- **Fix:** Implemented `deps.BeadsPath()` (and `deps.MustBeadsPath()`) to strictly resolve `bd` from `~/go/bin` or `BEADS_BIN` env var. Updated E2E runner to verify version on startup.

### 2. Prefix Mismatch (The "Te vs Test" Bug)
- **Issue:** Beads v0.48.0 CLI has a bug where `bd create` defaults to `test` prefix even if the local config defines a different prefix (e.g., `te` derived from `testrig`).
- **Symptoms:** `gt sling` failed with `issue ID 'te-...' does not match configured prefix 'test'`.
- **Diagnosis:** `gt rig add` configured rig with `te` prefix. `bd create` (run by E2E runner) created `test-xxx` (bug). Gastown generated agent ID `te-testrig...`. Beads validation rejected it against the `test`-corrupted DB.
- **Fix:** Forced consistency by running `gt rig add ... --prefix test`. This aligns the Rig config with the Beads CLI default/bug, ensuring `test` prefix is used everywhere.

### 3. Opencode Session Crash ("Died During Startup")
- **Issue:** `gt sling` reported "session died during startup".
- **Diagnosis:** Gastown was launching `claude` instead of `opencode`, despite `agent=opencode` setting.
- **Root Cause:** E2E runner wrote rig config to `testrig/settings/config.json` (wrong path) instead of `testrig/config.json`. Gastown ignored it and used the Town default (`claude`).
- **Fix:** Updated `configureRigSettings` to write to `testrig/config.json` and merge with existing config to preserve beads settings.

### 4. Tmux Server Isolation
- **Issue:** `opencode` wrapper script (used for debugging crashes) wasn't being executed because `tmux` sessions inherited the environment of the existing user `tmux` server (where PATH didn't include the wrapper).
- **Fix:** Implemented `TMUX_TMPDIR` isolation in E2E runner. Creates a fresh `tmux` server for each test using a short path in `/tmp` (to avoid macOS socket path limits). This ensures the test environment (PATH) is inherited.

### 5. Opencode Command Wrapper
- **Issue:** Need to debug why `opencode` crashes or hangs without seeing its output (hidden in tmux pane).
- **Solution:** Created a wrapper script that intercepts `opencode` calls, logs ARGS/ENV/CWD to a file, and uses `tee` to stream output to both log and stdout (ensuring `WaitForRuntimeReady` can see the prompt).

### 6. Compilation from Source
- **Issue:** E2E tests were running the installed `gt` binary from `~/go/bin`, ignoring local code changes (fixes).
- **Fix:** Updated `NewE2ERunner` to compile `gt` from source into a temporary directory at the start of the test, ensuring code under test is actually used.

## Current Status (as of Jan 19, 2026)
- **CreateFile Test:** PASSING. `opencode` successfully creates `hello.go`.
- **FixBug Test:** Timeout/Hang. Agent starts but `session.created` event does not fire, preventing prompt injection. This appears to be an `opencode` initialization stall when opening a repository with existing files.
- **Infrastructure:** Robust. Fails fast on session death. Logs visible. Plugin installed automatically.

## Next Steps
1. Investigate why `session.created` doesn't fire in `FixBug` (repo with files).
2. Consider `opencode` configuration to skip file indexing or auto-approve permissions if that's the blocker.
3. Optimize timeout/polling for faster feedback.
