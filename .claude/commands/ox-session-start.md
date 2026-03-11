<!-- ox-hash: c738f862487e ver: 0.4.0 -->
<!-- Keep this file thin. Behavioral guidance (use-when, common-issues, errors)
     belongs in the ox CLI JSON output (guidance field), not here.
     Skills are agent-specific wrappers; ox serves all agents (Codex, etc.).
     Exception: Post-Command sections that require agent-side actions (e.g.,
     displaying a notice, generating a summary) are legitimate here. -->
Start recording this agent session to the project ledger.

## Post-Command (REQUIRED)

After the command completes, check the JSON output:
- **`notice`**: If present, display the notice text to the user verbatim. This is a one-time transparency notice about session recording.
- **`guidance`**: Follow this guidance throughout the session. It contains instructions about plan capture, session boundaries, and troubleshooting.

$ox agent session start
