# Remote gt11 Access from MacBook

SSH reverse tunnel setup for accessing gt11 from MacBook Pro.

## Architecture

```
MacBook Pro                         gt11 Server
-----------                         -----------
    |                                    |
    | SSH to gt11 (outbound)             |
    |----------------------------------->|
    |                                    |
    | Reverse tunnel: gt11:2222 → Mac:22 |
    |<-----------------------------------|
    |                                    |
    | gt11 can now SSH to MacBook        |
    | via: ssh -p 2222 localhost         |
    |                                    |
```

## Quick Start

### 1. MacBook Setup

```bash
# One-time: Add SSH config
cat config/ssh-config-macbook.conf >> ~/.ssh/config
mkdir -p ~/.ssh/sockets

# Start the tunnel (simple)
./scripts/macbook-tunnel.sh -h your-gt11-host

# Or start with auto-reconnect in tmux (recommended)
./scripts/macbook-tmux-session.sh start -h your-gt11-host
```

### 2. gt11 Setup

```bash
# One-time: Add SSH config
cat config/ssh-config-gt11.conf >> ~/.ssh/config

# Connect to MacBook (once tunnel is established)
ssh macbook

# Or use the script
./scripts/gt11-to-macbook.sh
```

## Scripts

### MacBook Side

| Script | Purpose |
|--------|---------|
| `macbook-tunnel.sh` | Basic reverse tunnel connection |
| `macbook-tunnel-persistent.sh` | Auto-reconnecting tunnel wrapper |
| `macbook-tmux-session.sh` | tmux session manager for persistent tunnel |

### gt11 Side

| Script | Purpose |
|--------|---------|
| `gt11-to-macbook.sh` | Connect to MacBook via tunnel |

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GT11_HOST` | gt11.example.com | gt11 server hostname |
| `GT11_USER` | ubuntu | Username on gt11 |
| `REVERSE_PORT` | 2222 | Port on gt11 for reverse tunnel |
| `MACBOOK_USER` | current user | Username on MacBook |
| `TUNNEL_PORT` | 2222 | Same as REVERSE_PORT (gt11 side) |

### SSH Config Files

- `config/ssh-config-macbook.conf` - Add to `~/.ssh/config` on MacBook
- `config/ssh-config-gt11.conf` - Add to `~/.ssh/config` on gt11

## Usage Examples

### Basic Tunnel

```bash
# MacBook: Start tunnel
./macbook-tunnel.sh -h my-gt11.aws.com -u ubuntu

# gt11: Connect to MacBook
ssh -p 2222 steve@localhost
# or
ssh macbook
```

### Persistent Tunnel with tmux

```bash
# MacBook: Start persistent session
./macbook-tmux-session.sh start -h my-gt11.aws.com

# Check status
./macbook-tmux-session.sh status

# Attach to see tunnel output
./macbook-tmux-session.sh attach

# Stop tunnel
./macbook-tmux-session.sh stop
```

### Using SSH Config (recommended)

```bash
# MacBook: Simple tunnel via config
ssh gt11-tunnel
# This automatically sets up the reverse tunnel

# gt11: Connect back
ssh macbook
```

## Prerequisites

### MacBook
- SSH key added to gt11's `~/.ssh/authorized_keys`
- SSH daemon enabled: System Preferences → Sharing → Remote Login
- SSH key for gt11 to use when connecting back

### gt11
- SSH key added to MacBook's `~/.ssh/authorized_keys`
- netcat (`nc`) for connection testing (usually pre-installed)

## Troubleshooting

### Tunnel won't connect
1. Verify SSH access: `ssh gt11` from MacBook works
2. Check firewall on gt11 for port 2222
3. Ensure MacBook SSH daemon is running

### Connection drops frequently
1. Check network stability
2. Increase `ServerAliveInterval` in SSH config
3. Use persistent tunnel script with auto-reconnect

### Port already in use
1. Change `REVERSE_PORT` to a different port (e.g., 2223)
2. Check for existing tunnels: `ss -tlnp | grep 2222` on gt11

## Security Notes

- Use SSH keys only (no passwords)
- The reverse tunnel only binds to localhost on gt11
- Consider using a non-standard port for the tunnel
- Restrict MacBook SSH access if needed via `/etc/ssh/sshd_config`
