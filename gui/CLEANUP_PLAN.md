# PR Cleanup Plan - Address Steve's Feedback

## Feedback Received from steveyegge

Date: 2026-01-06
PR: #212 (draft)

**Overall:** Concept and architecture look promising, but PR needs cleanup before merge.

---

## Critical Issues to Fix

### 1. Remove .beads/ Files (76 files)

**Problem:** PR includes local workspace state that should never be committed:
- `issues.jsonl`, `interactions.jsonl` (contributor's local data)
- `daemon-*.log.gz` (local logs)
- `mq/*.json` (merge queue state)
- `formulas/*.toml` (may overwrite existing formulas)

**Action:**
- [ ] Remove all `.beads/` files from PR
- [ ] Add `.beads/` to `.gitignore`
- [ ] Commit: "fix: remove local .beads/ workspace files"

### 2. Remove Session Artifacts at Repo Root

**Problem:** Claude session/continuity artifacts shouldn't be in main repo:
- `CONTINUITY_CLAUDE-gastown-gui.md`
- `PR_READINESS_CHECKLIST.md`
- `docs/gui-review-2026-01-05.md`
- `docs/merge-analysis-2026-01-06.md`
- `docs/upstream-sync-2026-01-06.md`

**Action:**
- [ ] Remove root-level continuity files
- [ ] Remove docs/ session artifacts
- [ ] Optionally move useful ones to `gui/docs/` if needed
- [ ] Commit: "fix: remove session artifacts from repo root"

### 3. Handle Large Binary Files

**Problem:** Large files shouldn't be in git history:
- `gui/demo/combined_twitter_video.mp4` (11MB)
- `gui/assets/loading-background.jpeg` (verify size/license)

**Action:**
- [ ] Remove demo video from git
- [ ] Link to external host (YouTube/Vimeo) instead
- [ ] Check loading-background.jpeg size and license
- [ ] Commit: "fix: remove large binary demo video"

---

## Suggested Improvements

### 4. Keep PR Focused on gui/ Directory Only

**Goal:** Clean file structure
```
gui/
├── README.md
├── server.js
├── index.html
├── css/
├── js/
└── test-*.cjs
```

**Action:**
- [ ] Verify no files outside `gui/` directory
- [ ] Move any stragglers into `gui/` or remove
- [ ] Commit: "refactor: ensure all GUI files in gui/ directory"

### 5. Emphasize Security Warning in README

**Goal:** Make it crystal clear this is for localhost dev only

**Action:**
- [ ] Add prominent security warning in README
- [ ] Emphasize never expose publicly
- [ ] Add production hardening checklist
- [ ] Commit: "docs: emphasize security warning for localhost-only use"

---

## Execution Plan

### New Branch Strategy

1. **Create new branch from latest main:**
   ```bash
   git fetch upstream main
   git checkout -b feature/gui-cleanup upstream/main
   ```

2. **Cherry-pick good commits:**
   - Extract only GUI files
   - Exclude .beads/ files
   - Exclude session artifacts
   - Exclude large binaries

3. **Apply fixes one commit at a time:**
   - Commit 1: Remove .beads/ files + add to .gitignore
   - Commit 2: Remove session artifacts
   - Commit 3: Remove demo video (link externally instead)
   - Commit 4: Ensure files only in gui/ directory
   - Commit 5: Enhance security warning in README

4. **Create new clean PR:**
   - Close old PR #212 (explain cleanup in progress)
   - Open new PR with clean history
   - Reference Steve's feedback
   - Tag as ready for review

---

## Commits to Make (In Order)

### Commit 1: Add .gitignore for .beads/
```bash
# Create .gitignore with .beads/ exclusion
git commit -m "chore: add .gitignore to exclude .beads/ workspace files"
```

### Commit 2: Remove .beads/ files
```bash
# Remove all .beads/ files if they exist
git rm -rf .beads/
git commit -m "fix: remove local .beads/ workspace files (76 files)

These files are local workspace state that should never be committed:
- issues.jsonl, interactions.jsonl (local data)
- daemon-*.log.gz (local logs)
- mq/*.json (merge queue state)
- formulas/*.toml (may overwrite existing formulas)

Fixes feedback from steveyegge in PR #212."
```

### Commit 3: Remove session artifacts
```bash
# Remove continuity and session files
git rm CONTINUITY_CLAUDE-gastown-gui.md PR_READINESS_CHECKLIST.md
git rm docs/gui-review-2026-01-05.md docs/merge-analysis-2026-01-06.md docs/upstream-sync-2026-01-06.md
git commit -m "fix: remove Claude session artifacts from repo root

Removed:
- CONTINUITY_CLAUDE-gastown-gui.md
- PR_READINESS_CHECKLIST.md
- docs/gui-review-2026-01-05.md
- docs/merge-analysis-2026-01-06.md
- docs/upstream-sync-2026-01-06.md

These are session/continuity artifacts that shouldn't be in main repo.

Fixes feedback from steveyegge in PR #212."
```

### Commit 4: Remove demo video
```bash
# Remove video, update docs with external link
git rm gui/demo/combined_twitter_video.mp4
# Update PR_TEMPLATE_UPSTREAM.md with YouTube link
git commit -m "fix: remove large binary demo video from git

Removed 11MB video file. Will link to external host (YouTube/Vimeo)
instead of committing large binary to git history.

Fixes feedback from steveyegge in PR #212."
```

### Commit 5: Enhance security warning
```bash
# Update README.md with prominent security section
git commit -m "docs: emphasize security warning for localhost-only use

Added prominent security warning at top of README:
- This GUI has NO authentication
- Safe for localhost development ONLY
- NEVER expose to public internet
- Production deployment requires hardening

Addresses feedback from steveyegge in PR #212."
```

---

## Verification Checklist

Before creating new PR:

- [ ] No .beads/ files in git
- [ ] .beads/ in .gitignore
- [ ] No session artifacts in root
- [ ] No large binary files
- [ ] All files in gui/ directory
- [ ] Security warning prominent in README
- [ ] Clean git history (one issue per commit)
- [ ] Rebased on latest upstream/main
- [ ] E2E tests still pass
- [ ] No secrets or credentials

---

## New PR Details

**Title:** `feat: Gas Town Web GUI (cleaned up per feedback)`

**Description:**
```
This is a cleaned-up version of PR #212, addressing all feedback from @steveyegge.

## Changes from Previous PR

**Fixed:**
- ✅ Removed all .beads/ files (76 files)
- ✅ Added .beads/ to .gitignore
- ✅ Removed session artifacts from repo root
- ✅ Removed large demo video (will link externally)
- ✅ Enhanced security warning in README
- ✅ Focused on gui/ directory only

**What Stayed:**
- ✅ Server-authoritative architecture (wrapping CLI)
- ✅ Vanilla JS (simple and maintainable)
- ✅ WebSocket for real-time updates
- ✅ E2E test suite

## Previous Feedback

Original PR #212: [link]
Feedback addressed: [quote Steve's comments]

Ready for review!
```

---

## Timeline

1. Create cleanup branch: 5 minutes
2. Make commits (5 commits): 15 minutes
3. Verify and test: 10 minutes
4. Create new PR: 5 minutes
5. Close old PR with explanation: 2 minutes

**Total:** ~40 minutes

---

## Status

- [ ] Cleanup branch created
- [ ] Commit 1: .gitignore
- [ ] Commit 2: Remove .beads/
- [ ] Commit 3: Remove session artifacts
- [ ] Commit 4: Remove demo video
- [ ] Commit 5: Security warning
- [ ] Verification complete
- [ ] New PR created
- [ ] Old PR closed

---

## Notes

Steve's feedback was constructive and positive. The architecture is approved, just needs cleanup. This should be straightforward to address.

**What he liked:**
- Server-authoritative approach ✅
- Vanilla JS simplicity ✅
- WebSocket updates ✅
- E2E testing ✅

**Just needs:**
- File cleanup
- Better organization
- No local artifacts
