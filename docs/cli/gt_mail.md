---
title: "GT MAIL"
---

## gt mail

Agent messaging system

### Synopsis

Send and receive messages between agents.

The mail system allows Mayor, polecats, and the Refinery to communicate.
Messages are stored in beads as issues with type=message.

MAIL ROUTING:
  ┌─────────────────────────────────────────────────────┐
  │                    Town (.beads/)                   │
  │  ┌─────────────────────────────────────────────┐   │
  │  │                 Mayor Inbox                 │   │
  │  │  └── mayor/                                 │   │
  │  └─────────────────────────────────────────────┘   │
  │                                                     │
  │  ┌─────────────────────────────────────────────┐   │
  │  │           gastown/ (rig mailboxes)          │   │
  │  │  ├── witness      ← greenplace/witness         │   │
  │  │  ├── refinery     ← greenplace/refinery        │   │
  │  │  ├── Toast        ← greenplace/Toast           │   │
  │  │  └── crew/max     ← greenplace/crew/max        │   │
  │  └─────────────────────────────────────────────┘   │
  └─────────────────────────────────────────────────────┘

ADDRESS FORMATS:
  mayor/              → Mayor inbox
  <rig>/witness       → Rig's Witness
  <rig>/refinery      → Rig's Refinery
  <rig>/<polecat>     → Polecat (e.g., greenplace/Toast)
  <rig>/crew/<name>   → Crew worker (e.g., greenplace/crew/max)
  --human             → Special: human overseer

COMMANDS:
  inbox     View your inbox
  send      Send a message
  read      Read a specific message
  mark      Mark messages read/unread

```
gt mail [flags]
```

### Options

```
  -h, --help   help for mail
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt mail announces](../cli/gt_mail_announces/)	 - List or read announce channels
* [gt mail archive](../cli/gt_mail_archive/)	 - Archive messages
* [gt mail channel](../cli/gt_mail_channel/)	 - Manage and view beads-native channels
* [gt mail check](../cli/gt_mail_check/)	 - Check for new mail (for hooks)
* [gt mail claim](../cli/gt_mail_claim/)	 - Claim a message from a queue
* [gt mail clear](../cli/gt_mail_clear/)	 - Clear all messages from an inbox
* [gt mail delete](../cli/gt_mail_delete/)	 - Delete messages
* [gt mail directory](../cli/gt_mail_directory/)	 - List all valid mail recipient addresses
* [gt mail drain](../cli/gt_mail_drain/)	 - Bulk-archive stale protocol messages
* [gt mail group](../cli/gt_mail_group/)	 - Manage mail groups
* [gt mail hook](../cli/gt_mail_hook/)	 - Attach mail to your hook (alias for 'gt hook attach')
* [gt mail inbox](../cli/gt_mail_inbox/)	 - Check inbox
* [gt mail mark-read](../cli/gt_mail_mark-read/)	 - Mark messages as read without archiving
* [gt mail mark-unread](../cli/gt_mail_mark-unread/)	 - Mark messages as unread
* [gt mail peek](../cli/gt_mail_peek/)	 - Show preview of first unread message
* [gt mail queue](../cli/gt_mail_queue/)	 - Manage mail queues
* [gt mail read](../cli/gt_mail_read/)	 - Read a message
* [gt mail release](../cli/gt_mail_release/)	 - Release a claimed queue message
* [gt mail reply](../cli/gt_mail_reply/)	 - Reply to a message
* [gt mail search](../cli/gt_mail_search/)	 - Search messages by content
* [gt mail send](../cli/gt_mail_send/)	 - Send a message
* [gt mail thread](../cli/gt_mail_thread/)	 - View a message thread

