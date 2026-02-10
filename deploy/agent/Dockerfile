# Generic Gas Town agent image for all roles (mayor, deacon, crew, polecat, etc.).
#
# Multi-stage build:
#   1. Build gt from Go source
#   2. Final image with gt, coop, bd, claude CLI, and entrypoint
#
# External binaries (coop, bd) are downloaded from GitHub releases.
# Set build args to pin versions:
#   COOP_VERSION  - coop release tag (default: latest)
#   BD_VERSION    - beads release tag (default: latest)
#
# Build: docker build --platform linux/amd64 -t gastown-agent -f deploy/agent/Dockerfile .
# Run:   docker run -e GT_ROLE=mayor -e GT_AGENT=hq gastown-agent

# ── Stage 1: Build gt ───────────────────────────────────────────────────
FROM golang:1.25 AS gt-builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /gt ./cmd/gt

# ── Stage 2: Download external binaries ─────────────────────────────────
FROM ubuntu:24.04 AS downloader

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl ca-certificates jq \
    && rm -rf /var/lib/apt/lists/*

ARG TARGETARCH=amd64
ARG COOP_VERSION=latest
ARG BD_VERSION=latest

# Download coop from groblegark/coop releases
RUN set -e; \
    if [ "$COOP_VERSION" = "latest" ]; then \
      COOP_VERSION=$(curl -fsSL https://api.github.com/repos/groblegark/coop/releases/latest | jq -r .tag_name); \
    fi; \
    ARCH=$([ "$TARGETARCH" = "arm64" ] && echo "arm64" || echo "amd64"); \
    echo "Downloading coop ${COOP_VERSION} for linux-${ARCH}"; \
    curl -fsSL "https://github.com/groblegark/coop/releases/download/${COOP_VERSION}/coop-linux-${ARCH}.tar.gz" \
      -o /tmp/coop.tar.gz \
    && tar -xzf /tmp/coop.tar.gz -C /usr/local/bin \
    && chmod +x /usr/local/bin/coop

# Download bd from groblegark/beads releases
RUN set -e; \
    if [ "$BD_VERSION" = "latest" ]; then \
      BD_VERSION=$(curl -fsSL https://api.github.com/repos/groblegark/beads/releases/latest | jq -r .tag_name); \
    fi; \
    VERSION_NUM=${BD_VERSION#v}; \
    ARCH=$([ "$TARGETARCH" = "arm64" ] && echo "arm64" || echo "amd64"); \
    echo "Downloading bd ${BD_VERSION} for linux_${ARCH}"; \
    curl -fsSL "https://github.com/groblegark/beads/releases/download/${BD_VERSION}/beads_${VERSION_NUM}_linux_${ARCH}.tar.gz" \
      -o /tmp/bd.tar.gz \
    && tar -xzf /tmp/bd.tar.gz -C /usr/local/bin \
    && chmod +x /usr/local/bin/bd

# ── Stage 3: Final image ────────────────────────────────────────────────
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    ca-certificates \
    jq \
    nodejs \
    npm \
    && rm -rf /var/lib/apt/lists/*

# Install Claude CLI
RUN npm install -g @anthropic-ai/claude-code

# Create agent user (UID/GID 1000 to match pod security context)
# Use -f to allow existing GID 1000 (ubuntu group in Ubuntu 24.04)
RUN groupadd -f -g 1000 agent \
    && useradd -m -u 1000 -g 1000 -s /bin/bash agent \
    && mkdir -p /home/agent/gt \
    && chown -R agent:agent /home/agent

# Copy binaries
COPY --from=gt-builder /gt /usr/local/bin/gt
COPY --from=downloader /usr/local/bin/coop /usr/local/bin/coop
COPY --from=downloader /usr/local/bin/bd /usr/local/bin/bd

# Copy entrypoint script
COPY deploy/agent/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Workspace volume mount point
VOLUME /home/agent/gt

# Run as agent user
USER agent
WORKDIR /home/agent/gt

# Environment defaults (overridden by pod spec)
ENV HOME=/home/agent
ENV GT_ROLE=unknown
ENV GT_AGENT=unknown

ENTRYPOINT ["/entrypoint.sh"]
