# Run with 
# docker build -t gastown:latest -f Dockerfile . \
# --build-args GIT_USER=TestUser --build-args GIT_EMAIL=test@example.com 
FROM docker/sandbox-templates:claude-code

ARG GIT_USER
ARG GIT_EMAIL

RUN if [ -z "$GIT_USER" ] || [ -z "$GIT_EMAIL" ]; then \
    echo "ERROR: Required build arguments missing"; \
    echo "Build with: docker build --build-arg GIT_USER=<name> --build-arg GIT_EMAIL=<email> ."; \
    exit 1; \
    fi && \
    git config --global user.name "$GIT_USER" && \
    git config --global user.email "$GIT_EMAIL"

USER root

# Install your tools
RUN apt-get update && apt-get install -y \
    build-essential \
    golang-go \
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

RUN curl -fsSL https://raw.githubusercontent.com/steveyegge/beads/main/scripts/install.sh | bash

RUN curl -L https://github.com/dolthub/dolt/releases/latest/download/install.sh | bash

RUN mkdir -p /app
RUN chown agent:agent /app

RUN mkdir /gt
RUN chown agent:agent /gt

RUN echo 'export PATH="/app/gastown:$PATH"' >> /etc/profile.d/gastown.sh && \
    echo 'export PATH="/app/gastown:$PATH"' >> /etc/zsh/zshenv

RUN echo 'export COLORTERM="truecolor"' >> /etc/profile.d/colorterm.sh && \
    echo 'export COLORTERM="truecolor"' >> /etc/zsh/zshenv

RUN echo 'export TERM="xterm-256color"' >> /etc/profile.d/term.sh && \
    echo 'export TERM="xterm-256color"' >> /etc/zsh/zshenv


# Always switch back to agent user at the end
USER agent

COPY . /app/gastown

RUN cd /app/gastown && make build

RUN git config --global credential.helper store
RUN git config --global user.name "${GIT_USER}"
RUN git config --global user.email "${GIT_EMAIL}"

RUN dolt config --global --add user.name "${GIT_USER}"
RUN dolt config --global --add user.email "${GIT_EMAIL}"

RUN /app/gastown/gt install /gt --git

WORKDIR /gt