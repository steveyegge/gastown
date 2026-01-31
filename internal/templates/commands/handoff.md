---
description: Hand off to fresh session, work continues from hook
allowed-tools: Bash(gt handoff:*)
argument-hint: [message]
---

Hand off to a fresh session.

User's handoff message (if any): $ARGUMENTS

Execute these steps in order:

1. If user provided a message, run the handoff command with a subject and message.
   Example: `gt handoff -s "HANDOFF: Session cycling" -m "USER_MESSAGE_HERE"`

2. If no message was provided, run the handoff command:
   `gt handoff`

Note: The new session will auto-prime via the SessionStart hook and find your handoff mail.
End watch. A new session takes over, picking up any molecule on the hook.
