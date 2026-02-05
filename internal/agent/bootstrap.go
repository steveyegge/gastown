package agent

// GenerateBootstrap creates the bootstrap pointer content for CLAUDE.md/AGENTS.md.
// This is a minimal on-disk file that tells agents to run `gt prime` for full context.
// Used by both rig manager (initial creation) and doctor (fix missing files).
//
// Bootstrap files are placed at:
// - <root>/mayor/ - town-level mayor
// - <rig>/refinery/ - rig-level refinery
// - <rig>/witness/ - rig-level witness
// - <rig>/crew/ - parent directory for crew worktrees
// - <rig>/polecats/ - parent directory for polecat worktrees
//
// Note: Per-rig mayor (<rig>/mayor/) is just a source clone and does NOT get bootstrap files.
func GenerateBootstrap(role, rigName string) string {
	switch role {
	case "mayor":
		return `# Mayor Context (` + rigName + `)

> **Recovery**: Run ` + "`gt prime`" + ` after compaction, clear, or new session

Full context is injected by ` + "`gt prime`" + ` at session start.
`
	case "refinery":
		return `# Refinery Context (` + rigName + `)

> **Recovery**: Run ` + "`gt prime`" + ` after compaction, clear, or new session

Full context is injected by ` + "`gt prime`" + ` at session start.

## Quick Reference

- Check MQ: ` + "`gt mq list`" + `
- Process next: ` + "`gt mq process`" + `
`
	case "witness":
		return `# Witness Context (` + rigName + `)

> **Recovery**: Run ` + "`gt prime`" + ` after compaction, clear, or new session

Full context is injected by ` + "`gt prime`" + ` at session start.

## Quick Reference

- Check patrol: ` + "`gt patrol status`" + `
- Spawn polecat: ` + "`gt polecat spawn`" + `
`
	case "crew":
		return `# Crew Context (` + rigName + `)

> **Recovery**: Run ` + "`gt prime`" + ` after compaction, clear, or new session

Full context is injected by ` + "`gt prime`" + ` at session start.
`
	case "polecats":
		return `# Polecat Context (` + rigName + `)

> **Recovery**: Run ` + "`gt prime`" + ` after compaction, clear, or new session

Full context is injected by ` + "`gt prime`" + ` at session start.

## Quick Reference

- Check status: ` + "`gt polecat status`" + `
- View logs: ` + "`gt polecat logs`" + `
`
	default:
		return `# Agent Context (` + rigName + `)

> **Recovery**: Run ` + "`gt prime`" + ` after compaction, clear, or new session

Full context is injected by ` + "`gt prime`" + ` at session start.
`
	}
}
