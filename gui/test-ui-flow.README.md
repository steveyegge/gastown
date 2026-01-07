# Gas Town UI E2E Test

## Overview

Automated Puppeteer test that validates the complete Gas Town GUI workflow:

1. ✅ **Rig Creation** - Add a new rig (git repo clone, 90+ seconds)
2. ✅ **Work Item Creation** - Create a task with title, description, priority, labels
3. ⚠️ **Sling Workflow** - Assign work to rig (GUI works, GT CLI has known issue)
4. ✅ **UI Validation** - Verify toasts, state updates, and UI feedback

## Running the Test

```bash
# Prerequisites
# 1. Server must be running on port 5555
cd gui
PORT=5555 node server.js

# 2. Clean state (in another terminal)
cd ~/gt && gt rig remove zoo-game 2>/dev/null; rm -rf zoo-game

# Run test
node test-ui-flow.cjs
```

## Expected Results

**Steps 1-10: PASS** ✅
- Rig creation succeeds (takes 90+ seconds)
- Work item creation succeeds
- UI updates correctly with toasts

**Steps 11-13: KNOWN ISSUE** ⚠️
- Sling modal opens and form fills correctly
- Backend `gt sling` command fails with:
  ```
  Error: mol bond requires direct database access
  Hint: use --no-daemon flag: bd --no-daemon mol bond
  ```

## Known Issues

### GT CLI Bug (Not GUI Issue)

The test may fail at the sling step due to a **GT CLI bug**, not a GUI issue:

**Problem:**
```bash
gt sling hq-xyz zoo-game --quality=shiny
# Internally calls: bd mol bond hq-xyz ...
# Should call: bd --no-daemon mol bond hq-xyz ...
```

**Root Cause:**
- `gt sling` internally calls `bd mol bond` without the `--no-daemon` flag
- The beads daemon doesn't allow direct database access
- This is a GT CLI implementation issue, not a GUI bug

**Impact:**
- GUI correctly sends the sling request to the server
- Server correctly executes `gt sling` command
- GT CLI fails during formula bonding step

**Workaround:**
Manual sling with correct flags:
```bash
bd --no-daemon mol bond <bead-id> <formula-id>
```

## What This Test Validates

✅ **GUI Functionality:**
- Form inputs work correctly
- Modal workflows function properly
- Toast notifications display
- Server API endpoints respond
- Non-blocking operations work
- UI state updates correctly

⚠️ **External Dependencies:**
- GT CLI commands execute (with known issue)
- Beads database operations (blocked by GT CLI bug)

## Screenshots

- **Success:** `/tmp/gastown-test-success.png`
- **Failure:** `/tmp/gastown-test-failure.png`

## Timeout Configuration

- **Page load:** 5 seconds
- **Rig creation:** 150 seconds (cloning large repos)
- **Work item creation:** 15 seconds
- **Sling operation:** 15 seconds
- **Element selectors:** 10 seconds

## Debugging

Enable verbose logging:
```javascript
// In test-ui-flow.cjs
const TEST_CONFIG = {
  headless: false,  // See browser UI
  slowMo: 100,      // Slow down actions
};
```

View browser console:
```javascript
page.on('console', msg => {
  console.log(`[Browser ${msg.type()}]`, msg.text());
});
```
