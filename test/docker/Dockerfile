# Gastown E2E Testing Environment with Copilot CLI
FROM golang:1.23-bookworm

# Install system dependencies
RUN apt-get update && apt-get install -y \
    git \
    tmux \
    sqlite3 \
    curl \
    jq \
    nodejs \
    npm \
    && rm -rf /var/lib/apt/lists/*

# Install GitHub CLI (needed for copilot)
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update \
    && apt-get install -y gh \
    && rm -rf /var/lib/apt/lists/*

# Install GitHub Copilot CLI
RUN npm install -g @githubnext/github-copilot-cli || echo "Copilot CLI install attempted"

# Create test user and directories
RUN useradd -m -s /bin/bash testuser
USER testuser
WORKDIR /home/testuser

# Set up Go environment
ENV GOPATH=/home/testuser/go
ENV PATH=$PATH:/home/testuser/go/bin

# Create gt directory structure
RUN mkdir -p ~/gt/settings ~/gt/mayor ~/gt/deacon

# Clone gastown from fork
ARG GASTOWN_REPO=https://github.com/mzkoch/gastown.git
ARG GASTOWN_BRANCH=feature/copilot-native-support
RUN git clone --branch ${GASTOWN_BRANCH} ${GASTOWN_REPO} ~/gastown-src

# Build and install gt
WORKDIR /home/testuser/gastown-src
RUN go mod download && go install ./...

# Install bd (beads CLI)
RUN go install github.com/steveyegge/beads/cmd/bd@latest || echo "bd install attempted"

# Set up default copilot config
RUN mkdir -p ~/.config/gt
COPY --chown=testuser:testuser settings/ /home/testuser/gt/settings/

# Copy test scripts
COPY --chown=testuser:testuser scripts/ /home/testuser/scripts/
RUN chmod +x /home/testuser/scripts/*.sh 2>/dev/null || true

WORKDIR /home/testuser/gt

# Default command runs the test suite
CMD ["/bin/bash", "-c", "~/scripts/run-tests.sh"]
