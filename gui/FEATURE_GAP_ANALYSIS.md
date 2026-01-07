# Gas Town GUI - Feature Gap Analysis

## Current Status

The GUI currently covers **basic operations** but is missing significant functionality from the CLI.

---

## 1. VISUAL CONSISTENCY ISSUES

### Sidebar Agent Colors (Needs Update)
The sidebar uses CSS classes (`role-mayor`, `role-witness`, etc.) but they're not consistent with the new mail colors. Should use the same `AGENT_TYPES` config:

| Role | Current | Should Be |
|------|---------|-----------|
| Mayor | `role-mayor` class | `#a855f7` (purple) + `account_balance` icon |
| Witness | `role-witness` class | `#3b82f6` (blue) + `visibility` icon |
| Deacon | `role-deacon` class | `#f59e0b` (amber) + `gavel` icon |
| Refinery | `role-refinery` class | `#ef4444` (red) + `precision_manufacturing` icon |
| Polecat | `role-polecat` class | `#22c55e` (green) + `smart_toy` icon |

**Fix:** Extract `AGENT_TYPES` to a shared module, use consistently in sidebar, mail, agent grid, activity feed.

---

## 2. MISSING VIEWS/TABS

### A. GitHub Integration (HIGH PRIORITY)
Each rig has a `git_url` in config (e.g., `https://github.com/web3dev1337/hytopia-map-compression`). The UI should show:

- **Repos Tab**: List all GitHub repos linked to rigs
- **Pull Requests**: PRs from each repo (open, merged, draft)
- **Issues**: GitHub issues (linked to beads)
- **Commits**: Recent commits per rig

### B. Rigs View (HIGH PRIORITY)
Currently no way to see/manage rigs. Should show:
- All rigs with their GitHub URL
- Polecat count, Witness status, Refinery status
- Create new rig (`gt rig add`)
- View rig health/stats

### C. Formulas View
Workflow templates are invisible in UI:
- List available formulas
- Create new formulas
- View formula details
- Cook formula into molecule

### D. Logs/Output View (HIGH PRIORITY)
- **Polecat output**: `gt peek <polecat>` - see what a worker is doing
- **Activity log**: `gt log` - historical view
- **Session transcripts**: What agents are actually outputting

### E. Health/Doctor View
- `gt doctor` results - show system health
- Daemon status
- Agent session status

---

## 3. MISSING FEATURES IN EXISTING VIEWS

### Convoys View
Missing:
- [ ] Convoy synthesis steps (`gt synthesis`)
- [ ] Mark convoy done (`gt done`)
- [ ] Convoy progress percentage
- [ ] Link to GitHub PRs for convoy issues

### Work/Beads View
Missing:
- [ ] Bead detail modal with full content
- [ ] Edit bead
- [ ] Close/reopen bead
- [ ] Link to GitHub issue
- [ ] See which polecat is working on it
- [ ] See bead history/audit trail

### Agents View
Missing:
- [ ] **Peek at output** - see what agent is currently doing (CRITICAL)
- [ ] Start/stop agent
- [ ] View agent session transcript
- [ ] Agent hook status (what work is hooked)
- [ ] Nudge history

### Mail View
Missing:
- [ ] Thread view (related messages)
- [ ] Mark as read/unread
- [ ] Archive
- [ ] Reply inline

---

## 4. MISSING ACTIONS

### Work Management
| Action | CLI Command | UI Status |
|--------|-------------|-----------|
| Signal work done | `gt done` | Missing |
| Park work | `gt park` | Missing |
| Resume parked work | `gt resume` | Missing |
| Release stuck work | `gt release` | Missing |
| Find orphaned work | `gt orphans` | Missing |
| Hand off session | `gt handoff` | Missing |
| View/attach hook | `gt hook` | Partial (shows in status bar only) |

### Agent Management
| Action | CLI Command | UI Status |
|--------|-------------|-----------|
| Peek at output | `gt peek <agent>` | Missing (CRITICAL) |
| Start agent | `gt polecat spawn` | Missing |
| Stop agent | `gt stop` | Missing |
| Broadcast to all | `gt broadcast` | Missing |
| Set DND mode | `gt dnd` | Missing |
| Set notification level | `gt notify` | Missing |
| Escalate to human | `gt escalate` | Missing |

### Workspace Management
| Action | CLI Command | UI Status |
|--------|-------------|-----------|
| Add rig | `gt rig add` | Missing |
| Remove rig | `gt rig remove` | Missing |
| Create crew | `gt crew create` | Missing |
| Run health check | `gt doctor` | Missing |
| View activity | `gt activity` | Partial (feed only) |
| Audit by actor | `gt audit` | Missing |

### Service Management
| Action | CLI Command | UI Status |
|--------|-------------|-----------|
| Start services | `gt up` | Missing |
| Stop services | `gt down` | Missing |
| Shutdown | `gt shutdown` | Missing |
| Manage daemon | `gt daemon` | Missing |

---

## 5. PRIORITY RECOMMENDATIONS

### Phase 1: Critical Visibility (Week 1)
1. **Peek/Output view** - See what agents are doing in real-time
2. **Rigs view** - See all projects and their GitHub repos
3. **Consistent agent colors** - Shared config across all components

### Phase 2: GitHub Integration (Week 2)
1. **GitHub repos list** - Show all linked repos
2. **Pull requests view** - See PRs from rig repos
3. **Issues integration** - Link beads to GitHub issues

### Phase 3: Full Control (Week 3)
1. **Start/stop agents** - Control agent lifecycle
2. **Rig management** - Add/remove rigs
3. **Formula management** - Create and run workflows
4. **Service controls** - Start/stop/restart services

### Phase 4: Advanced Features (Week 4+)
1. **Audit/history view** - Query work by actor
2. **Session transcripts** - Full agent conversation logs
3. **Crew workspaces** - Manage persistent workspaces
4. **Checkpoint/recovery** - Session crash recovery UI

---

## 6. API ENDPOINTS NEEDED

The server (`server.js`) needs these new endpoints:

```javascript
// Rigs
GET  /api/rigs                    // ✓ Exists
GET  /api/rigs/:name              // Details
POST /api/rigs                    // ✓ Exists
DELETE /api/rigs/:name            // Remove

// Agent Output (CRITICAL)
GET  /api/polecat/:rig/:name/output   // ✓ Exists but not used in UI!
GET  /api/agent/:id/transcript    // Full transcript

// GitHub Integration
GET  /api/github/repos            // All linked repos
GET  /api/github/:owner/:repo/prs // Pull requests
GET  /api/github/:owner/:repo/issues // Issues

// Formulas
GET  /api/formulas                // List all
GET  /api/formulas/:name          // Details
POST /api/formulas                // Create
POST /api/formulas/:name/cook     // Cook to molecule

// Service Control
GET  /api/services                // Status of all services
POST /api/services/up             // Start all
POST /api/services/down           // Stop all
POST /api/agent/:id/start         // Start specific agent
POST /api/agent/:id/stop          // Stop specific agent

// Work Management
POST /api/beads/:id/done          // Mark done
POST /api/beads/:id/park          // Park work
POST /api/beads/:id/release       // Release stuck
GET  /api/orphans                 // Find orphaned work
```

---

## 7. QUICK WINS

These can be done immediately with existing APIs:

1. **Use the polecat output endpoint** - `/api/polecat/:rig/:name/output` exists but isn't used!
2. **Show rig list** - `/api/rigs` endpoint exists
3. **Consistent colors** - Just refactor `AGENT_TYPES` to shared module
4. **Add bead detail modal** - Data exists, just need UI

---

## Summary

**Current coverage: ~30%** of CLI functionality

The biggest gaps are:
1. **Visibility into agent work** - Can't see what they're doing
2. **GitHub integration** - No repo/PR/issue visibility
3. **Rig management** - Can't see or manage projects
4. **Agent control** - Can't start/stop/manage agents
