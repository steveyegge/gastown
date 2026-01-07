# Gas Town GUI

A web-based graphical interface for [Gas Town](https://github.com/steveyegge/gastown) - the multi-agent orchestration system for Claude Code.

## Overview

This GUI provides a browser-based interface to Gas Town's command-line tools (`gt` and `bd`), making it easier to:

- **Manage Rigs** - Add, view, and organize project repositories
- **Track Work** - Create and manage work items (beads)
- **Assign Tasks** - Sling work to rigs and agents
- **Monitor Activity** - View live updates via WebSocket
- **Browse PRs** - See GitHub pull requests across projects
- **Inspect Mail** - Read messages from agents and polecats

**Status:** 🚧 **Candidate for Testing** - Not 100% tested, may be missing features, but provides a solid starting point for a Gas Town GUI.

---

## Architecture

### Tech Stack

- **Backend:** Node.js + Express
- **Frontend:** Vanilla JavaScript (no framework)
- **Communication:** WebSocket for real-time updates
- **Rendering:** Server-side GT/BD JSON parsing + client-side DOM updates
- **Testing:** Puppeteer E2E tests

### Design Principles

1. **Server-Authoritative** - All operations execute via `gt` and `bd` CLI commands
2. **Non-Blocking UI** - Modals close immediately, operations run in background
3. **Real-Time Updates** - WebSocket broadcasts status changes to all clients
4. **Graceful Degradation** - UI handles missing data and command failures
5. **Cache & Refresh** - Background data preloading with stale-while-revalidate

---

## File Structure

```
gui/
├── server.js                 # Express + WebSocket server
├── index.html                # Main HTML (single page)
├── css/
│   └── styles.css           # All styles
├── js/
│   ├── app.js               # Main app initialization
│   ├── api.js               # Server API client
│   ├── websocket.js         # WebSocket handler
│   └── components/
│       ├── dashboard.js     # Dashboard view (overview)
│       ├── rigs.js          # Rigs list view
│       ├── work.js          # Work items view
│       ├── modals.js        # Modal dialogs
│       ├── pr-list.js       # Pull requests view
│       └── toasts.js        # Toast notifications
├── test-ui-flow.cjs         # Puppeteer E2E test
└── test-ui-flow.README.md   # Test documentation
```

---

## How It Works

### 1. Server (server.js)

The server acts as a **bridge between the web UI and Gas Town CLI tools**.

**Key Functions:**

```javascript
// Execute GT commands
async function executeGT(args, options = {})
  → Runs: gt <args>
  → Returns: { success, data, error }

// Execute BD commands
async function executeBD(args, options = {})
  → Runs: bd <args>
  → Returns: { success, data, error }

// WebSocket broadcast
function broadcast(message)
  → Sends to all connected clients
  → Used for: rig_added, status_update, etc.
```

**API Endpoints:**

| Method | Endpoint | Action | GT/BD Command |
|--------|----------|--------|---------------|
| GET | `/api/status` | Get system status | `gt status --json --fast` |
| GET | `/api/rigs` | List rigs | `gt rig list` |
| POST | `/api/rigs` | Add rig | `gt rig add <name> <url>` |
| GET | `/api/work` | List work items | `bd list --status=open --json` |
| POST | `/api/work` | Create work item | `bd new <title> --priority=<P> --label=<L>` |
| POST | `/api/sling` | Sling work to rig | `gt sling <bead> <target> --quality=<Q>` |
| GET | `/api/prs` | List PRs | `gh pr list --repo <R>` |
| GET | `/api/mail` | Get mail inbox | `gt mail inbox --json` |

**Configuration:**

```javascript
const PORT = process.env.PORT || 3000;
const GT_ROOT = process.env.GT_ROOT || path.join(HOME, 'gt');
```

### 2. Frontend Architecture

**Single Page Application** with view-based routing:

```
Views:
├── Dashboard    - System overview (rigs, agents, recent activity)
├── Rigs         - List all rigs, add new rigs
├── Work         - List work items, create new work, sling
├── PRs          - GitHub pull requests across repos
├── Formulas     - Available formulas (gt formula list)
└── Mail         - Agent/polecat messages
```

**State Management:**

```javascript
// Global state in js/app.js
window.appState = {
  currentView: 'dashboard',
  status: {},           // GT status
  rigs: [],            // Rig list
  work: [],            // Work items
  prs: [],             // Pull requests
  loading: false
};
```

**View Rendering Pattern:**

```javascript
// Each view component exports:
export function renderRigsView(container, data) {
  container.innerHTML = generateHTML(data);
  attachEventListeners();
}
```

### 3. WebSocket Communication

**Server → Client Events:**

```javascript
// Connection established
{ type: 'connected' }

// Rig added successfully
{ type: 'rig_added', data: { name, url } }

// Status update
{ type: 'status_update', data: { ... } }

// Activity stream (live feed)
{ type: 'activity', data: { message } }
```

**Client Handling:**

```javascript
ws.on('message', (data) => {
  switch (data.type) {
    case 'rig_added':
      // Refresh rigs list
      fetchRigs();
      break;
    case 'status_update':
      // Update dashboard
      updateStatus(data.data);
      break;
  }
});
```

### 4. Non-Blocking Operations

**Problem:** Operations like `gt rig add` take 90+ seconds (git clone).

**Solution:** Modal closes immediately, operation runs in background.

**Flow:**

```
User clicks "Add Rig"
  → Modal opens
  → User fills form
  → User clicks "Add"
  → Modal closes immediately ✓
  → Toast: "Adding rig..."
  → Server executes gt rig add (90s)
  → Toast: "Rig added successfully" ✓
  → WebSocket broadcast: rig_added
  → All clients refresh
```

**Implementation:**

```javascript
// Frontend - modals.js
async function handleRigAdd(name, url) {
  closeModal();  // Close immediately
  showToast(`Adding rig "${name}"...`, 'info');

  const result = await api.addRig(name, url);

  if (result.success) {
    showToast(`Rig "${name}" added successfully`, 'success');
  } else {
    showToast(`Failed: ${result.error}`, 'error');
  }
}
```

### 5. Data Flow Diagram

```
┌─────────────┐
│   Browser   │
│   (Client)  │
└──────┬──────┘
       │ HTTP API calls
       │ WebSocket events
       ↓
┌─────────────┐
│   Express   │
│   Server    │
└──────┬──────┘
       │ executeGT()
       │ executeBD()
       ↓
┌─────────────┐
│  GT / BD    │
│  CLI Tools  │
└──────┬──────┘
       │
       ↓
┌─────────────┐
│  ~/gt/      │
│  Database   │
└─────────────┘
```

---

## Key Features

### ✅ Implemented

**Rig Management:**
- List all rigs with status (polecats, crew, agents)
- Add new rig (git clone with 120s timeout)
- View rig details
- GitHub repo picker integration

**Work Management:**
- List work items (open/closed)
- Create new work items (title, description, priority, labels)
- Filter by status
- Priority mapping: urgent/high/normal/low → P0-P4

**Sling Workflow:**
- Select work item
- Choose target (rig or agent)
- Select quality (basic/shiny/chrome)
- Execute sling command

**Real-Time Updates:**
- WebSocket connection status indicator
- Live activity feed
- Auto-refresh on rig creation
- Toast notifications for all operations

**PR Tracking:**
- List GitHub PRs across repos
- Filter by state (open/merged/closed)
- Link to GitHub

**Mail Inbox:**
- View messages from agents
- Parse structured events (.events.jsonl)

### ⚠️ Known Limitations

**Not Yet Implemented:**
- Polecat management (spawn, kill, view logs)
- Convoy management
- Formula editor/creator
- Agent configuration
- Crew management
- Rig removal/deletion
- Work item editing
- Beads detail view
- Session attachment/detachment

**Known Issues:**
- GT CLI bug: `gt sling` fails with "mol bond requires direct database access"
  - Root cause: Missing `--no-daemon` flag in GT CLI
  - Not a GUI issue - documented in E2E test
- Error handling could be more granular
- Some edge cases untested

---

## Installation & Setup

### Prerequisites

```bash
# Gas Town CLI tools must be installed
gt --version
bd --version
gh --version  # For PR tracking
```

### Install Dependencies

```bash
cd gui
npm install
```

### Configuration

**Environment Variables:**

```bash
export PORT=5555                    # Server port (default: 3000)
export GT_ROOT=~/gt                 # Gas Town root (default: ~/gt)
export CORS_ORIGINS=http://localhost:5555  # CORS allowed origins
```

**GitHub Integration:**

For PR tracking, ensure GitHub CLI is authenticated:

```bash
gh auth status
```

### Run Server

```bash
cd gui
PORT=5555 node server.js
```

Server starts on: `http://localhost:5555`

---

## Usage

### 1. Add a Rig

**Via UI:**
1. Navigate to "Rigs" view
2. Click "Add Rig"
3. Enter name (e.g., "my-project")
4. Enter Git URL (e.g., "https://github.com/user/repo")
5. Click "Add Rig"
6. Wait for toast confirmation (~90 seconds for large repos)

**Behind the scenes:**
```bash
gt rig add my-project https://github.com/user/repo
```

### 2. Create Work Item

**Via UI:**
1. Navigate to "Work" view
2. Click "New Work Item"
3. Fill in:
   - Title (required)
   - Description
   - Priority (urgent/high/normal/low)
   - Labels (comma-separated)
4. Click "Create"

**Behind the scenes:**
```bash
bd new "Fix authentication bug" \
  --description "Users can't log in with Google" \
  --priority P1 \
  --label bug \
  --label auth
```

### 3. Sling Work to Rig

**Via UI:**
1. Navigate to "Work" view
2. Click "Sling" button
3. Enter bead ID (e.g., "hq-abc")
4. Select target (e.g., "my-project/witness")
5. Select quality (basic/shiny/chrome)
6. Click "Sling Work"

**Behind the scenes:**
```bash
gt sling hq-abc my-project --quality=shiny
```

**Note:** This may fail with GT CLI bug (see Known Issues).

---

## Testing

### E2E Test Suite

Comprehensive Puppeteer test validates:

**Steps 1-10 (PASSING):**
- ✅ Rig creation (90+ second timeout)
- ✅ Work item creation
- ✅ Toast notifications
- ✅ UI state updates
- ✅ Non-blocking operations

**Steps 11-13 (KNOWN ISSUE):**
- ✅ Sling modal opens and fills
- ⚠️ GT CLI sling fails (external bug)

**Run test:**

```bash
# Clean state
cd ~/gt && gt rig remove zoo-game 2>/dev/null; rm -rf zoo-game

# Run test
cd gui
node test-ui-flow.cjs
```

**Expected output:**
```
✅ GUI tests passed!

Summary:
  ✅ Created zoo-game rig
  ✅ Created work item: hq-xyz
  ✅ Opened sling modal and filled form
  ⚠️  Sling attempted but failed due to GT CLI issue (not GUI bug)
```

See `test-ui-flow.README.md` for complete documentation.

---

## Development

### Code Style

- **No frameworks** - Vanilla JS for simplicity
- **Server-authoritative** - All logic via GT/BD CLI
- **Modular views** - Each view is a separate component
- **Async/await** - For all API calls
- **Error handling** - Try/catch with user-friendly messages

### Adding a New View

1. Create component: `js/components/my-view.js`
2. Export render function:
   ```javascript
   export function renderMyView(container, data) {
     container.innerHTML = `<div>My View</div>`;
   }
   ```
3. Add route in `js/app.js`:
   ```javascript
   case 'my-view':
     renderMyView(container, appState.myData);
     break;
   ```
4. Add navigation button in `index.html`

### Adding an API Endpoint

1. Add route in `server.js`:
   ```javascript
   app.get('/api/my-endpoint', async (req, res) => {
     const result = await executeGT(['my', 'command']);
     res.json(result);
   });
   ```
2. Add client method in `js/api.js`:
   ```javascript
   async myEndpoint() {
     return this.request('/api/my-endpoint');
   }
   ```

### WebSocket Events

**Server broadcasts:**
```javascript
broadcast({
  type: 'my_event',
  data: { ... }
});
```

**Client handles:**
```javascript
// In js/websocket.js
switch (data.type) {
  case 'my_event':
    handleMyEvent(data.data);
    break;
}
```

---

## Deployment

### Production Considerations

**Security:**
- [ ] Add authentication (currently none)
- [ ] Validate all user inputs
- [ ] Rate limit API endpoints
- [ ] Use HTTPS
- [ ] Restrict CORS origins

**Performance:**
- [x] WebSocket for real-time updates
- [x] Background data preloading
- [x] Stale-while-revalidate cache pattern
- [ ] Add Redis for caching
- [ ] Compress responses

**Reliability:**
- [x] Graceful error handling
- [x] Toast notifications for all operations
- [ ] Add logging/monitoring
- [ ] Health check endpoint
- [ ] Automatic reconnection

### Docker Deployment

```dockerfile
FROM node:18-alpine

WORKDIR /app
COPY package*.json ./
RUN npm ci --production

COPY . .

ENV PORT=5555
ENV GT_ROOT=/gt

EXPOSE 5555
CMD ["node", "server.js"]
```

---

## Troubleshooting

### Server won't start

**Error:** `EADDRINUSE: address already in use`

**Solution:**
```bash
# Use different port
PORT=5556 node server.js

# Or kill existing process
lsof -i :5555 | grep LISTEN | awk '{print $2}' | xargs kill
```

### Rig creation timeout

**Error:** `Command timeout after 30000ms`

**Cause:** Large repos take 90+ seconds to clone.

**Solution:** Already fixed - timeout increased to 120s in server.js.

### WebSocket disconnects

**Symptom:** "Disconnected" indicator in top-right.

**Causes:**
- Server restarted
- Network issue
- Browser tab suspended

**Solution:** Refresh page to reconnect.

### GT commands fail

**Error:** `Command not found: gt`

**Solution:** Ensure GT CLI is installed and in PATH:
```bash
which gt
gt --version
```

### Sling fails with "mol bond" error

**Error:** `Error: mol bond requires direct database access`

**Cause:** GT CLI bug (not GUI issue).

**Workaround:**
```bash
# Manual sling with correct flags
bd --no-daemon mol bond <bead-id> <formula-id>
```

See "Known Issues" section above.

---

## Future Improvements

### High Priority

- [ ] **Authentication** - User login/sessions
- [ ] **Polecat Management** - Spawn, kill, view logs, attach to tmux
- [ ] **Error Recovery** - Retry failed operations
- [ ] **Work Editing** - Update existing beads
- [ ] **Rig Deletion** - Remove rigs safely

### Medium Priority

- [ ] **Convoy Management** - Create, view, manage convoys
- [ ] **Formula Builder** - Create custom formulas via UI
- [ ] **Agent Configuration** - Configure witness/refinery/mayor
- [ ] **Crew Management** - Add/remove crew members
- [ ] **Search & Filter** - Better work/rig filtering

### Low Priority

- [ ] **Dark Mode** - Theme switcher
- [ ] **Keyboard Shortcuts** - Power user features
- [ ] **Mobile Support** - Responsive design
- [ ] **Notifications** - Browser notifications for events
- [ ] **Analytics** - Usage tracking

---

## Contributing

This is a **candidate implementation** for Gas Town GUI. Feedback and improvements welcome!

**Before submitting PRs:**
1. Run E2E tests: `node test-ui-flow.cjs`
2. Test manually with real GT installation
3. Document any new features/limitations
4. Follow existing code style

---

## License

Same as Gas Town (MIT assumed - check upstream repo).

---

## Credits

**Original Gas Town:** [steveyegge/gastown](https://github.com/steveyegge/gastown)

**GUI Implementation:** Built with Claude Code

**Testing:** Puppeteer E2E test suite

---

## Disclaimer

⚠️ **This GUI is a candidate for testing, not a production-ready system.**

- Not 100% tested
- May be missing features
- Not certain it works exactly as Steve intended
- Presented as a starting point for what a Gas Town GUI could look like
- Use at your own risk

If this isn't the right direction, it can serve as reference for future GUI implementations.
