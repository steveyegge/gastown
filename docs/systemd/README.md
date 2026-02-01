# Systemd Service Management for Gas Town

Gas Town services (`gt rpc serve`, `gt slack start`) should be managed via systemd
to prevent stale binary issues and ensure automatic restarts.

## Quick Start

```bash
# Copy service files
cp docs/systemd/gt-rpc@.service ~/.config/systemd/user/
cp docs/systemd/gt-slack.service ~/.config/systemd/user/

# Create Slack config
mkdir -p ~/.config/gt
cat > ~/.config/gt/slack.env << EOF
SLACK_BOT_TOKEN=xoxb-your-token
SLACK_APP_TOKEN=xapp-your-token
SLACK_CHANNEL=C0123456789
GT_TOWN_ROOT=/path/to/your/town
EOF
chmod 600 ~/.config/gt/slack.env

# Reload systemd
systemctl --user daemon-reload

# Enable and start RPC server (encode your town path)
UNIT_NAME=$(systemd-escape --path /path/to/your/town)
systemctl --user enable gt-rpc@${UNIT_NAME}.service
systemctl --user start gt-rpc@${UNIT_NAME}.service

# Enable and start Slack bot
systemctl --user enable gt-slack.service
systemctl --user start gt-slack.service
```

## Why Systemd?

When you run `make install` to update gt, any running `gt rpc serve` or `gt slack start`
processes continue using the old binary in memory. This causes subtle bugs when new
features are added to the RPC or Slack integration.

Systemd provides:
- **Automatic restarts**: `systemctl --user restart gt-slack` after rebuilds
- **Logging**: `journalctl --user -u gt-slack -f` for live logs
- **Persistence**: Services start automatically on login
- **Clean shutdown**: Proper signal handling

## Managing Services

### Check Status
```bash
systemctl --user status gt-rpc@home-ubuntu-gt11.service
systemctl --user status gt-slack.service
```

### Restart After Rebuild
```bash
# After make install, restart to use new binary
systemctl --user restart gt-rpc@home-ubuntu-gt11.service
systemctl --user restart gt-slack.service
```

### View Logs
```bash
journalctl --user -u gt-rpc@home-ubuntu-gt11.service -f
journalctl --user -u gt-slack.service -f
```

### Stop Services
```bash
systemctl --user stop gt-slack.service
systemctl --user stop gt-rpc@home-ubuntu-gt11.service
```

### Disable Systemd Management
If you want to run services manually:
```bash
systemctl --user disable gt-slack.service
systemctl --user disable gt-rpc@home-ubuntu-gt11.service
```

## Service Files

- `gt-rpc@.service`: Template unit for RPC server (per-town)
- `gt-slack.service`: Slack bot service (single instance)

The `@` in `gt-rpc@.service` indicates a template unit. The part after `@` in
the enabled unit name (e.g., `home-ubuntu-gt11`) is the systemd-escaped town path.
