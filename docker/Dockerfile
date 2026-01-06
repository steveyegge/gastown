FROM golang:1.24-bookworm AS base
RUN apt-get update && apt-get install -y git && rm -rf /var/lib/apt/lists/*

FROM base AS gastown-builder
WORKDIR /build/gastown
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -gcflags="all=-N -l" -o /gt ./cmd/gt

FROM base AS beads-builder
WORKDIR /build
RUN git clone --depth 1 https://github.com/steveyegge/beads.git
WORKDIR /build/beads
RUN CGO_ENABLED=0 go build -gcflags="all=-N -l" -o /bd ./cmd/bd

FROM golang:1.24-bookworm AS debug
RUN apt-get update && apt-get install -y \
    git \
    tmux \
    curl \
    procps \
    jq \
    strace \
    lsof \
    net-tools \
    vim \
    less \
    tree \
    && rm -rf /var/lib/apt/lists/*

RUN go install github.com/go-delve/delve/cmd/dlv@latest

COPY --from=gastown-builder /gt /usr/local/bin/gt
COPY --from=beads-builder /bd /usr/local/bin/bd

RUN git config --global user.email "test@gastown.local" && \
    git config --global user.name "Gas Town Test" && \
    git config --global init.defaultBranch main

RUN useradd -m -s /bin/bash tester
USER tester
WORKDIR /home/tester

ENV HOME=/home/tester
ENV PATH="/root/go/bin:/usr/local/bin:${PATH}"
ENV TERM=xterm-256color

COPY --chown=tester:tester docker/investigate.sh /home/tester/investigate.sh
RUN chmod +x /home/tester/investigate.sh

CMD ["/bin/bash"]

FROM debian:bookworm-slim AS production
RUN apt-get update && apt-get install -y \
    git \
    tmux \
    && rm -rf /var/lib/apt/lists/*

COPY --from=gastown-builder /gt /usr/local/bin/gt
COPY --from=beads-builder /bd /usr/local/bin/bd

RUN git config --global user.email "test@gastown.local" && \
    git config --global user.name "Gas Town Test" && \
    git config --global init.defaultBranch main

RUN useradd -m -s /bin/bash tester
USER tester
WORKDIR /home/tester

ENV HOME=/home/tester
ENV PATH="/usr/local/bin:${PATH}"

CMD ["/bin/bash"]
