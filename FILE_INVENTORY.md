# Town Root File Inventory (~/gt11)

**Audit Date**: 2026-01-29
**Auditor**: gastown/crew/town_audit
**Bead**: gt-zrw0o.1

## Summary

The town root already has a git repository initialized. This inventory categorizes all files for version control decisions.

---

## TO VERSION (Config/Templates)

### Town-Level Configuration
| Path | Purpose | Notes |
|------|---------|-------|
| `.beads/config.yaml` | Beads storage backend config | Already tracked |
| `.beads/metadata.json` | Beads metadata | Already tracked |
| `.beads/routes.jsonl` | Cross-rig routing rules | Already tracked |
| `.beads/.gitignore` | Beads-specific ignores | Already tracked |
| `.beads/formulas/*.toml` | Workflow formula templates | Already tracked (38 files) |
| `.beads/formulas/.installed.json` | Installed formula registry | Already tracked |
| `.beads/hooks/on_decision_respond` | Decision hook script | Already tracked |

### Claude Configuration
| Path | Purpose | Notes |
|------|---------|-------|
| `.claude/commands/handoff.md` | Handoff command template | Already tracked |
| `.claude/skills/*` | Skill symlinks | Already tracked (4 symlinks) |
| `.claude/settings.json` | Symlink to mayor settings | Should track |
| `.claude/settings.local.json` | Local overrides | Should track |

### Mayor Configuration
| Path | Purpose | Notes |
|------|---------|-------|
| `mayor/town.json` | Town identity/metadata | Should track |
| `mayor/rigs.json` | Registered rigs | Should track |
| `mayor/overseer.json` | Overseer config | Should track |
| `mayor/daemon.json` | Daemon config | Should track |
| `mayor/accounts.json` | User accounts | Contains credentials? Review first |
| `mayor/.claude/settings.json` | Claude settings for Mayor | Should track |

### Documentation
| Path | Purpose | Notes |
|------|---------|-------|
| `AGENTS.md` | Agent documentation | Already tracked |
| `.gitattributes` | Git attributes | Already tracked |

### Plugins
| Path | Purpose | Notes |
|------|---------|-------|
| `plugins/README.md` | Plugin documentation | Should track |

### Git Hooks (Beads)
| Path | Purpose | Notes |
|------|---------|-------|
| `.beads/hooks/post-checkout` | Git hook | Should track |
| `.beads/hooks/post-merge` | Git hook | Should track |
| `.beads/hooks/pre-commit` | Git hook | Should track |
| `.beads/hooks/pre-push` | Git hook | Should track |
| `.beads/hooks/prepare-commit-msg` | Git hook | Should track |

---

## TO GITIGNORE (Runtime/Ephemeral)

### Large Data (Has Own Versioning)
| Path | Size | Reason |
|------|------|--------|
| `.beads/beads/` | 96MB | Dolt database (self-versioning) |
| `.beads/beads-dolt-backup/` | 6.2MB | Dolt backup |
| `.beads/dolt` | symlink | Points to ~/.beads-dolt |

### Runtime Event Logs
| Path | Size | Reason |
|------|------|--------|
| `.events.jsonl` | 1.5MB | Runtime event stream |
| `.feed.jsonl` | 1.4MB | Activity feed |
| `logs/town.log` | 220KB | Town-wide logs |

### Daemon Files
| Path | Reason |
|------|--------|
| `daemon/daemon.log` | Runtime logs |
| `daemon/daemon.lock` | Lock file |
| `daemon/daemon.pid` | PID file |
| `daemon/shutdown.lock` | Lock file |
| `daemon/state.json` | Runtime state |
| `daemon/activity.json` | Runtime activity |
| `.beads/daemon.log` | Beads daemon log |
| `.beads/daemon.lock` | Lock file |
| `.beads/last-touched` | Timestamp file |

### Runtime Queues
| Path | Reason |
|------|--------|
| `.runtime/` | Entire runtime directory |
| `.runtime/inject-queue/` | Message queue (~200+ files) |
| `.runtime/nudge-queue/` | Nudge queue |
| `.runtime/session_id` | Current session |

### Backup/Corrupted Files
| Path | Reason |
|------|--------|
| `.beads/issues.jsonl.backup` | Backup file |
| `.beads/routes.jsonl.backup.*` | Backup file |
| `.beads/routes.jsonl.corrupted.*` | Corrupted file |
| `.beads/config.yaml.bak` | Backup file |

### Generated JSONL (Runtime Data)
| Path | Size | Reason |
|------|------|--------|
| `.beads/issues.jsonl` | 3.6MB | Runtime bead data |
| `.beads/interactions.jsonl` | 0 | Runtime interactions |

### Binaries
| Path | Size | Reason |
|------|------|--------|
| `gt` | 29MB | gt binary (build artifact) |
| `beads.db` | 733KB | SQLite database |

### Secrets
| Path | Reason |
|------|--------|
| `.slack-env` | Slack credentials |

### Marker Files
| Path | Reason |
|------|--------|
| `.beads/.gt-types-configured` | Local state marker |
| `.beads/.local_version` | Local version tracker |
| `.beads/README.md` | Generated readme (not tracked) |

---

## RIG DIRECTORIES (Special Handling)

Each rig directory (beads/, gastown/, fics_helm_chart/, test_rig_e2e/, deacon/) contains:

### Version
- `.beads/redirect` - Worktree redirect (tracked at rig level)
- `.repo.git/` - Git repo data
- `mayor/` - Rig-specific mayor config
- `settings/` - Rig settings
- `plugins/` - Rig plugins

### Ignore (Runtime)
- `.beads/beads.db*` - Local SQLite
- `.beads/daemon.*` - Daemon files
- `.runtime/` - Runtime data
- `crew/*/` - Worker workspaces (contain their own git)
- `polecats/*/` - Polecat workspaces
- `witness/state.json` - Runtime state
- `refinery/` - Merge queue runtime
- `metadata.json` - Runtime metadata

### Deacon-Specific
- `deacon/.beads/` - Local beads (SQLite mode)
- `deacon/.runtime/` - Runtime
- `deacon/dogs/*/` - Dog workspaces
- `deacon/heartbeat.json` - Runtime
- `deacon/state.json` - Runtime
- `deacon/dolt-server.pid` - Runtime

---

## RECOMMENDATIONS

1. **Already a git repo**: ~/gt11 has existing .git with commits
2. **Create root .gitignore**: Currently missing
3. **Review accounts.json**: May contain sensitive data
4. **Rig clones are complex**: Each has its own .repo.git, consider if rig contents should be in town repo or remain separate
5. **Large files**: .events.jsonl, .feed.jsonl growing - definitely ignore

---

## CURRENT GIT STATE

```
Tracked files: ~100+ (mainly .beads/ config, gastown rig internals)
Untracked: .events.jsonl, .feed.jsonl, various runtime files
Modified: Several formula files, gastown rig files
```

The repo appears to be tracking a mix of town config AND the gastown rig clone contents. This is an architectural decision point.
