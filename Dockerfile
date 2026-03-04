# Run with
# docker build -t gastown:latest -f Dockerfile . \
#   --build-arg GIT_USER=TestUser --build-arg GIT_EMAIL=test@example.com
FROM docker/sandbox-templates:claude-code

ARG GIT_USER
ARG GIT_EMAIL
ARG GO_VERSION=1.25.6

RUN if [ -z "$GIT_USER" ] || [ -z "$GIT_EMAIL" ]; then \
    echo "ERROR: Required build arguments missing"; \
    echo "Build with: docker build --build-arg GIT_USER=<name> --build-arg GIT_EMAIL=<email> ."; \
    exit 1; \
    fi

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
    vim \
    && rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/*

# Install Go from official tarball (apt golang-go is too old)
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:/home/agent/go/bin:${PATH}"

# Install beads (bd) and dolt
RUN curl -fsSL https://raw.githubusercontent.com/steveyegge/beads/main/scripts/install.sh | bash
RUN curl -fsSL https://github.com/dolthub/dolt/releases/latest/download/install.sh | bash

# Set up directories
RUN mkdir -p /app /gt && chown agent:agent /app /gt

# Environment setup for bash and zsh
RUN echo 'export PATH="/app/gastown:$PATH"' >> /etc/profile.d/gastown.sh && \
    echo 'export PATH="/app/gastown:$PATH"' >> /etc/zsh/zshenv
RUN echo 'export COLORTERM="truecolor"' >> /etc/profile.d/colorterm.sh && \
    echo 'export COLORTERM="truecolor"' >> /etc/zsh/zshenv
RUN echo 'export TERM="xterm-256color"' >> /etc/profile.d/term.sh && \
    echo 'export TERM="xterm-256color"' >> /etc/zsh/zshenv

USER agent

COPY . /app/gastown

RUN cd /app/gastown && make build

# Configure git and dolt for the user
RUN git config --global credential.helper store && \
    git config --global user.name "${GIT_USER}" && \
    git config --global user.email "${GIT_EMAIL}" && \
    dolt config --global --add user.name "${GIT_USER}" && \
    dolt config --global --add user.email "${GIT_EMAIL}"

RUN /app/gastown/gt install /gt --git

WORKDIR /gt
