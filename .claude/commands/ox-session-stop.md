<!-- ox-hash: b4dbdd886a38 ver: 0.4.0 -->
<!-- Keep this file thin. Behavioral guidance (use-when, common-issues, errors)
     belongs in the ox CLI JSON output (guidance field), not here.
     Skills are agent-specific wrappers; ox serves all agents (Codex, etc.).
     Exception: Post-Command sections that require agent-side actions (e.g.,
     displaying a notice, generating a summary) are legitimate here. -->
Stop recording and save this agent session to the project ledger.

## Post-Command: Generate and Push Summary (REQUIRED)

After the command completes, check the JSON output for `guidance` and `summary_prompt` fields.
Follow the `guidance` field for next steps. The summary step is critical for session completeness.

**If `summary_prompt` is present:**
1. Read the prompt carefully — it references the raw session file on disk
2. Read the raw session file at the path specified in the prompt
3. Generate the summary JSON following the Output Format in the prompt
4. Save it to a temporary file (e.g., `.ox-summary.json` in the workspace root, or `/tmp/ox-summary.json`) — do NOT write to the session cache dir as it may be outside the workspace sandbox
5. If the prompt includes a `push-summary` step, run that command with `--file` pointing to your temp file
6. Verify the push succeeded by checking the JSON output for `"success": true`
7. Clean up the temporary summary file

**If summary generation fails:**
- Run `ox agent <id> doctor` — it can detect and help recover missing summaries
- The session data is safe regardless; only the rich summary is missing

$ox agent session stop
