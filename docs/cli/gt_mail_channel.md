---
title: "GT MAIL CHANNEL"
---

## gt mail channel

Manage and view beads-native channels

### Synopsis

View and manage beads-native broadcast channels.

Without arguments, lists all channels.
With a channel name, shows messages from that channel.

Channels are pub/sub streams where messages are broadcast to subscribers.
Messages are retained according to the channel's retention policy.

Examples:
  gt mail channel              # List all channels
  gt mail channel alerts       # View messages from 'alerts' channel
  gt mail channel list         # Alias for listing channels
  gt mail channel show alerts  # Same as: gt mail channel alerts
  gt mail channel create alerts --retain-count=100
  gt mail channel delete alerts

```
gt mail channel [name] [flags]
```

### Options

```
  -h, --help   help for channel
      --json   Output as JSON
```

### SEE ALSO

* [gt mail](../cli/gt_mail/)	 - Agent messaging system
* [gt mail channel create](../cli/gt_mail_channel_create/)	 - Create a new channel
* [gt mail channel delete](../cli/gt_mail_channel_delete/)	 - Delete a channel
* [gt mail channel list](../cli/gt_mail_channel_list/)	 - List all channels
* [gt mail channel show](../cli/gt_mail_channel_show/)	 - Show channel messages
* [gt mail channel subscribe](../cli/gt_mail_channel_subscribe/)	 - Subscribe to a channel
* [gt mail channel subscribers](../cli/gt_mail_channel_subscribers/)	 - List channel subscribers
* [gt mail channel unsubscribe](../cli/gt_mail_channel_unsubscribe/)	 - Unsubscribe from a channel

