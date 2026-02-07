# Multi-stage build for gt (Gas Town CLI)
FROM golang:1.24-alpine AS builder

ARG VERSION=dev
ARG BUILD_COMMIT=unknown

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build \
    -ldflags "-X github.com/steveyegge/gastown/internal/cmd.Version=${VERSION} \
              -X github.com/steveyegge/gastown/internal/cmd.Commit=${BUILD_COMMIT} \
              -X github.com/steveyegge/gastown/internal/cmd.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
              -X github.com/steveyegge/gastown/internal/cmd.BuiltProperly=1" \
    -o /gt ./cmd/gt

FROM alpine:3.21

RUN apk add --no-cache ca-certificates git

COPY --from=builder /gt /usr/local/bin/gt

ENTRYPOINT ["gt"]
