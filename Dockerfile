# Gas Town API Server - Production Dockerfile
# Builds the gt binary and packages with all required tools for
# Claude Code agent orchestration (tmux, git, node, claude CLI, beads).

# --- Stage 1: Build Go binary ---
FROM golang:1.24-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git gcc musl-dev

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build with version info
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/steveyegge/gastown/internal/cmd.Version=${VERSION} \
              -X github.com/steveyegge/gastown/internal/cmd.Commit=${COMMIT} \
              -X github.com/steveyegge/gastown/internal/cmd.BuiltProperly=1" \
    -o gt ./cmd/gt

# --- Stage 2: Runtime image ---
FROM node:20-alpine

# Install system dependencies
RUN apk add --no-cache \
    git \
    openssh-client \
    tmux \
    bash \
    ca-certificates \
    python3 \
    py3-pip \
    curl \
    github-cli

# Install gt binary from builder
COPY --from=builder /app/gt /usr/local/bin/gt

# Install beads CLI
RUN pip3 install --break-system-packages beads-cli

# Install Claude Code CLI (npm global)
RUN npm install -g @anthropic-ai/claude-code

# Create non-root user and data directories
RUN adduser -D gastown \
    && mkdir -p /data/gt /data/home/.claude \
    && chown -R gastown:gastown /data

WORKDIR /data/gt
USER gastown

# Expose API port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s \
    CMD wget -q --spider http://localhost:8080/health || exit 1

# Start the API server
CMD ["gt", "serve", "--port", "8080"]
