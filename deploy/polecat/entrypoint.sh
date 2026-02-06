#!/bin/bash
# Polecat container entrypoint: starts sshd + tmux with Claude agent.
#
# Required environment variables:
#   GT_POLECAT  - polecat name (e.g., "Toast")
#   GT_RIG      - rig name (e.g., "gastown")
#
# Optional:
#   GT_COMMAND  - command to run in tmux (default: "claude")
#   GT_SESSION  - tmux session name (default: "claude")

set -euo pipefail

SESSION_NAME="${GT_SESSION:-claude}"
COMMAND="${GT_COMMAND:-claude}"

echo "[entrypoint] Starting polecat container: ${GT_POLECAT:-unknown} in ${GT_RIG:-unknown}"

# Generate SSH host keys if not present (first boot)
if [ ! -f /etc/ssh/ssh_host_ed25519_key ]; then
    echo "[entrypoint] Generating SSH host keys..."
    ssh-keygen -A
fi

# Ensure authorized_keys has correct permissions
# Keys are mounted from K8s Secret at /home/gt/.ssh/authorized_keys
if [ -f /home/gt/.ssh/authorized_keys ]; then
    chmod 600 /home/gt/.ssh/authorized_keys
    chown gt:gt /home/gt/.ssh/authorized_keys
    echo "[entrypoint] SSH authorized_keys configured"
else
    echo "[entrypoint] WARNING: No authorized_keys found at /home/gt/.ssh/authorized_keys"
    echo "[entrypoint] SSH access will not be available until keys are mounted"
fi

# Start sshd in background
echo "[entrypoint] Starting sshd..."
/usr/sbin/sshd -D &
SSHD_PID=$!
echo "[entrypoint] sshd started (PID: $SSHD_PID)"

# Start tmux session with the agent command as gt user
echo "[entrypoint] Starting tmux session '${SESSION_NAME}' with command: ${COMMAND}"
su - gt -c "tmux new-session -d -s '${SESSION_NAME}' '${COMMAND}'"

# Set remain-on-exit so we can capture diagnostic output if the agent crashes
su - gt -c "tmux set-option -t '${SESSION_NAME}' remain-on-exit on"

echo "[entrypoint] Polecat ready. SSH on port 22, tmux session: ${SESSION_NAME}"

# Wait for tmux session to exit (agent done or crashed)
# Check every 10 seconds
while true; do
    if ! su - gt -c "tmux has-session -t '=${SESSION_NAME}' 2>/dev/null"; then
        echo "[entrypoint] tmux session '${SESSION_NAME}' ended"
        break
    fi

    # Check if the pane is dead (agent exited but session preserved by remain-on-exit)
    PANE_DEAD=$(su - gt -c "tmux list-panes -t '=${SESSION_NAME}' -F '#{pane_dead}'" 2>/dev/null || echo "1")
    if [ "$PANE_DEAD" = "1" ]; then
        echo "[entrypoint] Agent process exited (pane dead). Capturing diagnostic output..."
        su - gt -c "tmux capture-pane -p -t '=${SESSION_NAME}' -S -50" 2>/dev/null || true
        break
    fi

    sleep 10
done

# Cleanup
echo "[entrypoint] Shutting down sshd..."
kill "$SSHD_PID" 2>/dev/null || true

echo "[entrypoint] Polecat container exiting"
