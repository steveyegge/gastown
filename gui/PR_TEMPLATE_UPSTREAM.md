# Gas Town Web GUI - Candidate Implementation

## Overview

This PR introduces a **web-based graphical interface** for Gas Town, providing a browser UI for managing rigs, work items, and agent orchestration.

**Status:** 🚧 **Candidate for Testing** - This is presented as a starting point and reference implementation. It's not 100% tested and may not align perfectly with your vision, but it demonstrates what a Gas Town GUI could look like.

---

## What This Adds

### Complete Web Interface

A single-page web application that wraps Gas Town CLI tools:

```
Browser UI → Express Server → GT/BD CLI → ~/gt Database
```

**Views:**
- 📊 **Dashboard** - System overview, rig status, recent activity
- 🗂️ **Rigs** - List/add/view rigs
- 📝 **Work** - Create and manage work items (beads)
- 🎯 **Sling** - Assign work to rigs/agents
- 🔀 **PRs** - GitHub pull request tracking
- 📬 **Mail** - Agent message inbox

### Key Features

✅ **Real-Time Updates** - WebSocket broadcasts for live status
✅ **Non-Blocking Operations** - Modals close immediately, ops run in background
✅ **GitHub Integration** - Pick repos from your GitHub account
✅ **Toast Notifications** - User feedback for all operations
✅ **E2E Testing** - Puppeteer test suite validates workflows

---

## Architecture

### Server (Node.js + Express)

Acts as a bridge between browser and CLI tools:

```javascript
// All operations execute via CLI
async function executeGT(args) {
  return execFile('gt', args);
}

// API endpoints wrap commands
app.post('/api/rigs', async (req, res) => {
  const result = await executeGT(['rig', 'add', name, url]);
  res.json(result);
});
```

**No database** - All data comes from `gt status --json`, `bd list --json`, etc.

### Frontend (Vanilla JavaScript)

Single-page app with view-based routing:

```javascript
// Simple state management
window.appState = {
  currentView: 'dashboard',
  rigs: [],
  work: [],
  status: {}
};

// Views render to DOM
function renderRigsView(container, data) {
  container.innerHTML = generateHTML(data);
}
```

**No frameworks** - Intentionally minimal dependencies for easy maintenance.

### WebSocket Communication

Server broadcasts events to all connected clients:

```javascript
// Rig added successfully
broadcast({ type: 'rig_added', data: { name, url } });

// All clients refresh
ws.on('message', (data) => {
  if (data.type === 'rig_added') fetchRigs();
});
```

---

## Usage Example

### Adding a Rig

**UI Flow:**
1. Click "Add Rig"
2. Enter name: `my-project`
3. Enter URL: `https://github.com/user/repo`
4. Click "Add"
5. Modal closes immediately
6. Toast: "Adding rig..." (operation runs in background ~90s)
7. Toast: "Rig added successfully"
8. Rigs list auto-refreshes

**Behind the scenes:**
```bash
gt rig add my-project https://github.com/user/repo
```

### Creating Work

**UI Flow:**
1. Click "New Work Item"
2. Fill form (title, description, priority, labels)
3. Click "Create"
4. Modal closes
5. Toast: "Creating work item..."
6. Toast: "Work item created: hq-abc"
7. Work list updates

**Behind the scenes:**
```bash
bd new "Fix auth bug" \
  --description "Users can't login" \
  --priority P1 \
  --label bug
```

---

## What Works

### Validated by E2E Tests

**✅ Steps 1-10 Pass:**
- Rig creation (handles 90+ second git clone)
- Work item creation (all priorities)
- Toast notifications
- UI state updates
- Non-blocking operations

**⚠️ Steps 11-13 - Known Issue:**
- Sling modal works correctly
- GT CLI has bug: `gt sling` missing `--no-daemon` flag
- This is **not a GUI issue** - documented in test suite

**Run test:**
```bash
cd gui
node test-ui-flow.cjs
```

See `test-ui-flow.README.md` for details.

---

## Known Limitations

### Not Yet Implemented

- 🔲 Polecat management (spawn, kill, logs)
- 🔲 Convoy management
- 🔲 Formula editor/creator
- 🔲 Agent configuration
- 🔲 Crew management
- 🔲 Rig deletion
- 🔲 Work editing
- 🔲 Beads detail view
- 🔲 Session attach/detach

### Testing Status

- ✅ E2E test suite (Steps 1-10)
- ✅ Manual testing (rig/work creation)
- ⚠️ Not exhaustively tested
- ⚠️ Edge cases may exist

### External Issues

**GT CLI Bug:**
- `gt sling` fails with "mol bond requires direct database access"
- Root cause: Missing `--no-daemon` flag in GT CLI sling command
- Not a GUI issue - GUI correctly calls `gt sling`
- Documented in E2E test

---

## Installation

### Prerequisites

```bash
# Gas Town CLI must be installed
gt --version
bd --version
gh --version  # For PR tracking
```

### Setup

```bash
cd gui
npm install
PORT=5555 node server.js
```

Open: http://localhost:5555

---

## File Structure

```
gui/
├── README.md                 # Complete documentation
├── server.js                 # Express + WebSocket server
├── index.html                # Single-page app
├── css/styles.css           # All styles
├── js/
│   ├── app.js               # Main initialization
│   ├── api.js               # Server API client
│   ├── websocket.js         # WebSocket handler
│   └── components/
│       ├── dashboard.js     # Dashboard view
│       ├── rigs.js          # Rigs view
│       ├── work.js          # Work view
│       ├── modals.js        # Modals
│       └── toasts.js        # Notifications
├── test-ui-flow.cjs         # E2E test suite
└── test-ui-flow.README.md   # Test docs
```

---

## Design Decisions

### Why Vanilla JS?

- **Simplicity** - No build step, no framework lock-in
- **Maintainability** - Easy to understand and modify
- **Minimal Dependencies** - Fewer security vulnerabilities
- **Fast Load** - No large framework bundles

If you prefer React/Vue/Svelte, the server API is framework-agnostic.

### Why Server-Authoritative?

All operations execute via `gt` and `bd` CLI:

**Pros:**
- ✅ No duplication of Gas Town logic
- ✅ Stays in sync with CLI changes
- ✅ Validates operations server-side
- ✅ Easy to audit (just read server.js)

**Cons:**
- ❌ Slower than direct DB access
- ❌ Depends on CLI stability
- ❌ Can't do operations CLI doesn't support

### Why WebSocket?

Real-time updates without polling:

**Before (Polling):**
```javascript
setInterval(() => fetchStatus(), 5000);  // Every 5s
```

**After (WebSocket):**
```javascript
ws.on('rig_added', () => fetchStatus());  // Only when needed
```

**Benefits:**
- Instant updates
- Reduced server load
- Better UX

---

## Security Considerations

⚠️ **This GUI has NO authentication** - it's designed for local development use.

**For production:**
- [ ] Add user authentication
- [ ] Validate all inputs
- [ ] Rate limit API endpoints
- [ ] Use HTTPS
- [ ] Restrict CORS
- [ ] Add session management

**Currently safe for:**
- ✅ Local development (localhost)
- ✅ Trusted internal networks

**NOT safe for:**
- ❌ Public internet
- ❌ Untrusted users
- ❌ Production without hardening

---

## Future Improvements

If this direction is worth pursuing:

### High Priority

- [ ] Polecat management (spawn, kill, logs, tmux attach)
- [ ] Authentication & sessions
- [ ] Rig deletion
- [ ] Work editing
- [ ] Error recovery

### Medium Priority

- [ ] Convoy management
- [ ] Formula builder
- [ ] Agent configuration
- [ ] Search & filtering

### Low Priority

- [ ] Dark mode
- [ ] Keyboard shortcuts
- [ ] Mobile responsive
- [ ] Browser notifications

---

## Testing This PR

### Quick Test

```bash
# 1. Install
cd gui
npm install

# 2. Run server
PORT=5555 node server.js

# 3. Open browser
open http://localhost:5555

# 4. Try workflows
# - Add a rig
# - Create work item
# - View dashboard
```

### E2E Test

```bash
# Clean state
cd ~/gt && gt rig remove zoo-game 2>/dev/null; rm -rf zoo-game

# Run automated test
cd gui
node test-ui-flow.cjs

# Expected: Steps 1-10 pass
```

---

## Screenshots

*(If you want screenshots before merging, I can add them)*

**Dashboard View:**
- System status overview
- Rig list with agent counts
- Recent activity feed

**Rigs View:**
- All rigs with details
- Add rig modal
- GitHub repo picker

**Work View:**
- Work items list
- Create work modal
- Sling modal

---

## Questions for Review

I'm presenting this as a **candidate implementation** and would appreciate feedback on:

1. **Architecture** - Is wrapping CLI commands the right approach?
2. **Tech Stack** - Vanilla JS vs React/Vue?
3. **Features** - What's most important to add next?
4. **UI/UX** - Does the flow make sense?
5. **Direction** - Is this worth building on, or start fresh?

---

## Development Context

**Time Constraints:**

This was built with Claude Code over the course of a week. I ran out of Claude Max credits with **2 days left to go** in the billing cycle, so this represents what was achievable within those constraints.

**Demo Video:**

Watch the GUI in action: [View demo video on YouTube/Vimeo](https://example.com/demo-video) *(link to be updated)*

The video demonstrates:
- Rig creation workflow
- Work item management
- Sling operations
- Real-time UI updates
- Toast notifications

**What This Means:**

- Built iteratively with AI assistance
- Not every edge case tested
- Some features prioritized over others
- Focused on core workflows first
- Documentation added at the end

Despite time/credit constraints, the core functionality works and the architecture is sound. More time would allow for:
- More comprehensive testing
- Additional features
- Better error handling
- Performance optimization
- Security hardening

---

## Disclaimer

⚠️ **Important:**

- This is **not 100% tested** - edge cases likely exist
- May be **missing features** you consider essential
- **Not certain** it aligns with your vision for Gas Town
- Presented as a **candidate for testing** and a **starting point**
- Built under **time/resource constraints** (ran out of credits)

If this isn't the right direction, it can serve as:
- Reference implementation
- Ideas for UI patterns
- Starting point for discussion
- Example of what's possible

I'm open to feedback and happy to iterate if you think this is worth pursuing.

---

## Files Changed

**New Files:**
- `gui/README.md` - Complete documentation (this file)
- `gui/server.js` - Express + WebSocket server
- `gui/index.html` - Single-page app
- `gui/css/styles.css` - All styles
- `gui/js/**/*.js` - Frontend components
- `gui/test-ui-flow.cjs` - E2E test suite
- `gui/test-ui-flow.README.md` - Test documentation

**Modified Files:**
- None (GUI is isolated in `gui/` directory)

**No changes to existing Gas Town code** - This is purely additive.

---

## Credits

Built with Claude Code as an exploration of what a Gas Town GUI could be.

Thanks for creating Gas Town! 🚂

---

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
