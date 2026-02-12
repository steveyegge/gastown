# Toolchain sidecar image for Gas Town K8s agent pods.
#
# Provides development tools shared with the agent container via workspace volume.
# Used as a K8s native sidecar (initContainer with restartPolicy: Always).
#
# Build targets:
#   full    — Go, Node, Python, AWS CLI, Docker CLI, kaniko (~1.5GB)
#   minimal — git, jq, make, curl (~200MB)
#
# Build:
#   docker build --platform linux/amd64 --target full -t gastown-toolchain:full .
#   docker build --platform linux/amd64 --target minimal -t gastown-toolchain:minimal .
#
# Fork: override ARGs to customize tool versions or disable tools.

# ── Base stage: common tools ──────────────────────────────────────────
FROM public.ecr.aws/docker/library/ubuntu:24.04 AS base
ARG TARGETARCH
ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    git \
    jq \
    make \
    unzip \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# ── Go ────────────────────────────────────────────────────────────────
FROM base AS go-builder
ARG GO_VERSION=1.24.1
ARG TARGETARCH
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${TARGETARCH}.tar.gz" | \
    tar -C /usr/local -xz

# ── Node.js ───────────────────────────────────────────────────────────
FROM base AS node-builder
ARG NODE_MAJOR=22
RUN curl -fsSL https://deb.nodesource.com/setup_${NODE_MAJOR}.x | bash - && \
    apt-get install -y --no-install-recommends nodejs && \
    rm -rf /var/lib/apt/lists/*

# ── Python ────────────────────────────────────────────────────────────
FROM base AS python-builder
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip python3-venv \
    && rm -rf /var/lib/apt/lists/*

# ── AWS CLI ───────────────────────────────────────────────────────────
FROM base AS aws-builder
ARG TARGETARCH
RUN ARCH=$([ "$TARGETARCH" = "arm64" ] && echo "aarch64" || echo "x86_64") && \
    curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-${ARCH}.zip" -o /tmp/awscli.zip && \
    unzip -q /tmp/awscli.zip -d /tmp && \
    /tmp/aws/install --install-dir /opt/aws-cli --bin-dir /opt/aws-cli/bin && \
    rm -rf /tmp/awscli.zip /tmp/aws

# ── Docker CLI (client only, no daemon) ───────────────────────────────
FROM base AS docker-builder
ARG DOCKER_VERSION=27.5.1
ARG TARGETARCH
RUN ARCH=$([ "$TARGETARCH" = "arm64" ] && echo "aarch64" || echo "x86_64") && \
    curl -fsSL "https://download.docker.com/linux/static/stable/${ARCH}/docker-${DOCKER_VERSION}.tgz" | \
    tar -xz --strip-components=1 -C /usr/local/bin docker/docker

# ── Kaniko (daemonless image builder) ─────────────────────────────────
FROM gcr.io/kaniko-project/executor:v1.23.2 AS kaniko

# ── Full target: all tools ────────────────────────────────────────────
FROM base AS full

# Go
COPY --from=go-builder /usr/local/go /usr/local/go

# Node.js (copy installed packages)
COPY --from=node-builder /usr/bin/node /usr/bin/node
COPY --from=node-builder /usr/bin/npm /usr/bin/npm
COPY --from=node-builder /usr/bin/npx /usr/bin/npx
COPY --from=node-builder /usr/lib/node_modules /usr/lib/node_modules

# Python
COPY --from=python-builder /usr/bin/python3 /usr/bin/python3
COPY --from=python-builder /usr/bin/pip3 /usr/bin/pip3
COPY --from=python-builder /usr/lib/python3* /usr/lib/
RUN ln -sf /usr/bin/python3 /usr/bin/python

# AWS CLI
COPY --from=aws-builder /opt/aws-cli /opt/aws-cli
RUN ln -sf /opt/aws-cli/bin/aws /usr/local/bin/aws

# Docker CLI (client only)
COPY --from=docker-builder /usr/local/bin/docker /usr/local/bin/docker

# Kaniko
COPY --from=kaniko /kaniko/executor /usr/local/bin/kaniko

ENV PATH="/usr/local/go/bin:${PATH}"

# Run as agent user (matches agent container UID/GID 1000)
RUN groupadd -g 1000 agent 2>/dev/null || true && \
    useradd -u 1000 -g 1000 -m -s /bin/bash agent 2>/dev/null || true

USER 1000:1000
WORKDIR /home/agent/gt

# Native sidecar: sleep forever, tools accessed via kubectl exec or shared volume
CMD ["sleep", "infinity"]

# ── Minimal target: just base tools ───────────────────────────────────
FROM base AS minimal

# Run as agent user
RUN groupadd -g 1000 agent 2>/dev/null || true && \
    useradd -u 1000 -g 1000 -m -s /bin/bash agent 2>/dev/null || true

USER 1000:1000
WORKDIR /home/agent/gt

CMD ["sleep", "infinity"]
