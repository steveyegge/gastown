# Remote MacBook Crew

This crew workspace enables remote command execution on a MacBook Pro via SSH reverse tunnel.

## Purpose

Execute commands on MacBook from gt11 through a reverse SSH tunnel. This allows gt11 agents to:
- Run macOS-specific tools and commands
- Access local development environments on MacBook
- Sync files between gt11 and MacBook
- Test software on macOS

## Quick Start

### 1. MacBook: Start the tunnel
```bash
# Copy scripts to MacBook first, then:
./scripts/macbook-tmux-session.sh start -h <gt11-host>
```

### 2. gt11: Connect via this crew
```bash
# Check tunnel is available
./scripts/gt11-to-macbook.sh

# Or use SSH directly
ssh -p 2222 localhost
```

## Scripts

| Script | Purpose |
|--------|---------|
| `scripts/macbook-tunnel.sh` | Basic reverse tunnel (run on MacBook) |
| `scripts/macbook-tunnel-persistent.sh` | Auto-reconnect wrapper (MacBook) |
| `scripts/macbook-tmux-session.sh` | tmux session manager (MacBook) |
| `scripts/gt11-to-macbook.sh` | Connect from gt11 to MacBook |

## Config Files

| File | Purpose |
|------|---------|
| `config/ssh-config-macbook.conf` | Add to ~/.ssh/config on MacBook |
| `config/ssh-config-gt11.conf` | Add to ~/.ssh/config on gt11 |

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
```
