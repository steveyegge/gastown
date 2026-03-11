<!-- ox-hash: 409a9267d73d ver: 0.4.0 -->
<!-- Keep this file thin. Behavioral guidance (use-when, common-issues, errors)
     belongs in the ox CLI JSON output (guidance field), not here.
     Skills are agent-specific wrappers; ox serves all agents (Codex, etc.). -->
Check the status of all active session recordings in this project.

## Post-Command

After the command completes, check the JSON output:
- **`recording: true`** — A session is active. Continue working normally.
- **`recording: false`** — No active session. Consider running `/ox-session-start` if you want to record.
- **`guidance`** — Follow any guidance returned by the CLI.
- **`count > 1`** — Multiple concurrent recordings. The `sessions` array shows each one.
- **`agent_id`** — The agent ID of the recording. Compare with your own to identify your session.

$ox session status --json --current
