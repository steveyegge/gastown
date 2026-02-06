# Polecat container image with SSH + tmux for remote terminal access (Phase 5).
#
# Each polecat pod runs:
#   1. sshd - for remote peek/nudge via SSH
#   2. tmux session "claude" - running the Claude agent
#
# Build: docker build -t gastown-polecat -f deploy/polecat/Dockerfile .
# Run:   docker run -d -p 2222:22 -v /path/to/ssh-keys:/home/gt/.ssh:ro gastown-polecat

FROM ubuntu:24.04

# Avoid interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install dependencies: SSH server, tmux, git, and common tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    openssh-server \
    tmux \
    git \
    curl \
    ca-certificates \
    jq \
    && rm -rf /var/lib/apt/lists/*

# Create gt user for SSH access
RUN useradd -m -s /bin/bash gt \
    && mkdir -p /home/gt/.ssh \
    && chmod 700 /home/gt/.ssh \
    && chown -R gt:gt /home/gt

# Configure sshd
RUN mkdir -p /run/sshd \
    && sed -i 's/#PasswordAuthentication yes/PasswordAuthentication no/' /etc/ssh/sshd_config \
    && sed -i 's/#PubkeyAuthentication yes/PubkeyAuthentication yes/' /etc/ssh/sshd_config \
    && echo "AllowUsers gt" >> /etc/ssh/sshd_config \
    && echo "ClientAliveInterval 30" >> /etc/ssh/sshd_config \
    && echo "ClientAliveCountMax 3" >> /etc/ssh/sshd_config

# Install gt and bd binaries (built externally, copied in)
COPY deploy/polecat/bin/gt /usr/local/bin/gt
COPY deploy/polecat/bin/bd /usr/local/bin/bd

# Copy entrypoint script
COPY deploy/polecat/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# SSH port
EXPOSE 22

# Environment variables set by K8s pod spec:
#   GT_POLECAT     - polecat name
#   GT_RIG         - rig name
#   GT_TOWN_ROOT   - town root path
#   GT_ROLE        - "polecat"
#   ANTHROPIC_AUTH_TOKEN - API key

ENTRYPOINT ["/entrypoint.sh"]
