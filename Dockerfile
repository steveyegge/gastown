# Run with
# docker build -t gastown:latest -f Dockerfile .
FROM docker/sandbox-templates:claude-code

ARG GO_VERSION=1.25.6

USER root

# Install system dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    git \
    sqlite3 \
    tmux \
    curl \
    wget \
    openssh-client \
    ripgrep \
    zsh \
    gh \
    netcat-openbsd \
    tini \
    vim \
    && rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/*

# Install Go from official tarball (apt golang-go is too old)
RUN ARCH=$(dpkg --print-architecture) && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" | tar -C /usr/local -xz
ENV PATH="/app/gastown:/usr/local/go/bin:/home/agent/go/bin:${PATH}"

# Install beads (bd) and dolt
RUN curl -fsSL https://raw.githubusercontent.com/steveyegge/beads/main/scripts/install.sh | bash
RUN curl -fsSL https://github.com/dolthub/dolt/releases/latest/download/install.sh | bash

# Set up directories
RUN mkdir -p /app /gt /gt/.dolt-data && chown -R agent:agent /app /gt

# Environment setup for bash and zsh
RUN echo 'export PATH="/app/gastown:$PATH"' >> /etc/profile.d/gastown.sh && \
    echo 'export PATH="/app/gastown:$PATH"' >> /etc/zsh/zshenv
RUN echo 'export COLORTERM="truecolor"' >> /etc/profile.d/colorterm.sh && \
    echo 'export COLORTERM="truecolor"' >> /etc/zsh/zshenv
RUN echo 'export TERM="xterm-256color"' >> /etc/profile.d/term.sh && \
    echo 'export TERM="xterm-256color"' >> /etc/zsh/zshenv

################################ Carepatron customisations ###############################

# Versions — keep in sync with .devcontainer/Dockerfile
ARG NODE_VERSION=24
ARG YARN_VERSION=3.5.0
ARG SHFMT_VERSION=3.11.0
ARG YAMLFMT_VERSION=0.21.0

# 1Password CLI (credential retrieval at container startup)
RUN curl -sS https://downloads.1password.com/linux/keys/1password.asc | \
    gpg --dearmor --output /usr/share/keyrings/1password-archive-keyring.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/1password-archive-keyring.gpg] \
    https://downloads.1password.com/linux/debian/$(dpkg --print-architecture) stable main" | \
    tee /etc/apt/sources.list.d/1password.list && \
    apt-get update && apt-get install -y 1password-cli && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Node.js + corepack + yarn
# Required for: biome (JSON/TS/CSS formatting), yarn install in worktrees (UI work)
RUN curl -fsSL https://deb.nodesource.com/setup_${NODE_VERSION}.x | bash - && \
    apt-get install -y nodejs && \
    apt-get clean && rm -rf /var/lib/apt/lists/*
RUN corepack enable && corepack prepare yarn@${YARN_VERSION} --activate

# shfmt + yamlfmt
RUN ARCH=$(dpkg --print-architecture) && \
    wget "https://github.com/mvdan/sh/releases/download/v${SHFMT_VERSION}/shfmt_v${SHFMT_VERSION}_linux_${ARCH}" \
    -O /usr/local/bin/shfmt && \
    chmod +x /usr/local/bin/shfmt && \
    YAMLFMT_ARCH=$(case "${ARCH}" in amd64) echo x86_64;; arm64) echo arm64;; *) echo "${ARCH}";; esac) && \
    wget "https://github.com/google/yamlfmt/releases/download/v${YAMLFMT_VERSION}/yamlfmt_${YAMLFMT_VERSION}_Linux_${YAMLFMT_ARCH}.tar.gz" \
    -O /tmp/yamlfmt.tar.gz && \
    tar -xzf /tmp/yamlfmt.tar.gz -C /usr/local/bin yamlfmt && \
    chmod +x /usr/local/bin/yamlfmt && \
    rm /tmp/yamlfmt.tar.gz

# .NET SDK 10
# Matches public-api base image (mcr.microsoft.com/dotnet/sdk:10.0)
# Required for: dotnet build, dotnet test, dotnet format (Refinery quality gates)
RUN curl -fsSL https://dot.net/v1/dotnet-install.sh | \
    bash -s -- --channel 10.0 --install-dir /usr/share/dotnet
ENV PATH="/usr/share/dotnet:${PATH}"

################################ Carepatron customisations ###############################

USER agent

COPY --chown=agent:agent . /app/gastown

RUN cd /app/gastown && make build

COPY --chown=agent:agent docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

WORKDIR /gt

ENTRYPOINT ["tini", "--", "/app/docker-entrypoint.sh"]
CMD ["sleep", "infinity"]
