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
    ripgrep \
    zsh \
    gh \
    netcat-openbsd \
    tini \
    vim \
    libicu-dev \
    && rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/*

# Upgrade OpenCode to pinned version (replaces base image copy in-place)
RUN npm install -g opencode-ai@1.3.13

# Install Go from official tarball (apt golang-go is too old)
RUN ARCH=$(dpkg --print-architecture) && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" | tar -C /usr/local -xz
ENV PATH="/app/gastown:/usr/local/go/bin:/home/agent/go/bin:/home/agent/bin:${PATH}"

# Install beads (bd) from source — prebuilt binaries link against ICU 74
# but the base image ships ICU 76.
RUN GOBIN=/usr/local/bin CGO_ENABLED=1 go install github.com/steveyegge/beads/cmd/bd@latest
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

USER agent

COPY --chown=agent:agent . /app/gastown

RUN cd /app/gastown && make build

COPY --chown=agent:agent docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

WORKDIR /gt

EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=5s --start-period=90s --retries=3 \
  CMD curl -fsS http://localhost:8080/up || exit 1

ENTRYPOINT ["tini", "--", "/app/docker-entrypoint.sh"]
CMD ["gt", "dashboard", "--bind", "0.0.0.0", "--port", "8080"]
