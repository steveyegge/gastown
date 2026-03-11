<!-- ox-hash: 96adbd0d854e ver: 0.4.0 -->
<!-- Keep this file thin. Behavioral guidance (use-when, post-command, errors)
     belongs in the ox CLI JSON output (guidance field), not here.
     Skills are agent-specific wrappers; ox serves all agents (Codex, etc.). -->
Abort the current session, discarding all local data without uploading to the ledger.
This is destructive and cannot be undone. Use `/ox-session-stop` to save instead.

$ox agent session abort --force
