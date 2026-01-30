.PHONY: build install clean test test-safe generate deploy \
        build-slackbot install-slackbot deploy-slackbot deploy-all proto

BINARY := gt
SLACKBOT := gtslack
BUILD_DIR := .
INSTALL_DIR := $(HOME)/.local/bin
DEPLOY_DIR := $(HOME)/local/bin
TMP_BUILD := /tmp/$(BINARY)-build

# Get version info for ldflags
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -X github.com/steveyegge/gastown/internal/cmd.Version=$(VERSION) \
           -X github.com/steveyegge/gastown/internal/cmd.Commit=$(COMMIT) \
           -X github.com/steveyegge/gastown/internal/cmd.BuildTime=$(BUILD_TIME) \
           -X github.com/steveyegge/gastown/internal/cmd.BuiltProperly=1

generate:
	go generate ./...

# Regenerate protobuf files (requires buf CLI)
proto:
	buf generate proto

build: generate
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/gt
ifeq ($(shell uname),Darwin)
	@codesign -s - -f $(BUILD_DIR)/$(BINARY) 2>/dev/null || true
	@echo "Signed $(BINARY) for macOS"
endif

install: build
	@mkdir -p $(INSTALL_DIR)
	@rm -f $(INSTALL_DIR)/$(BINARY)
	@cp $(BUILD_DIR)/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"

# Deploy to ~/local/bin with proper file replacement
# Uses mv (not cp) to avoid "binary file in use" errors
# See: gt-347882
deploy: generate
	@echo "==> Pulling latest from origin/main..."
	git pull origin main
	@echo "==> Building $(BINARY) to temp location..."
	go build -ldflags "$(LDFLAGS)" -o $(TMP_BUILD) ./cmd/gt
ifeq ($(shell uname),Darwin)
	@codesign -s - -f $(TMP_BUILD) 2>/dev/null || true
endif
	@echo "==> Creating deploy directory if needed..."
	@mkdir -p $(DEPLOY_DIR)
	@echo "==> Deploying $(BINARY) to $(DEPLOY_DIR)/$(BINARY)..."
	@mv $(TMP_BUILD) $(DEPLOY_DIR)/$(BINARY)
	@echo "==> Verifying deployment..."
	@echo "Hash: $$(sha256sum $(DEPLOY_DIR)/$(BINARY) | cut -d' ' -f1)"
	@echo "Version: $$( $(DEPLOY_DIR)/$(BINARY) version 2>/dev/null || echo 'unknown' )"
	@echo "✓ Deployed $(BINARY) to $(DEPLOY_DIR)/$(BINARY)"

clean:
	rm -f $(BUILD_DIR)/$(BINARY)
	rm -f $(BUILD_DIR)/$(SLACKBOT)
	rm -f $(TMP_BUILD)
	rm -f /tmp/$(SLACKBOT)-build

test:
	go test ./...

# Resource-constrained test mode: runs one package at a time with limited CPUs
test-safe:
	GOMAXPROCS=2 go test -p 1 -v ./...

# === DEPRECATED: Slackbot (gtslack) standalone targets ===
# These targets build the legacy standalone gtslack binary.
# The preferred approach is now 'gt slack start'.
# These will be removed in a future version.

build-slackbot: generate
	@echo "WARNING: gtslack is deprecated. Use 'gt slack start' instead."
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(SLACKBOT) ./cmd/gtslack
	@echo "Built $(SLACKBOT) (deprecated)"

install-slackbot: build-slackbot
	@mkdir -p $(INSTALL_DIR)
	@rm -f $(INSTALL_DIR)/$(SLACKBOT)
	@cp $(BUILD_DIR)/$(SLACKBOT) $(INSTALL_DIR)/$(SLACKBOT)
	@echo "Installed $(SLACKBOT) to $(INSTALL_DIR)/$(SLACKBOT) (deprecated)"

deploy-slackbot: generate
	@echo "WARNING: gtslack is deprecated. Use 'gt slack start' instead."
	@echo "==> Building $(SLACKBOT) to temp location..."
	go build -ldflags "$(LDFLAGS)" -o /tmp/$(SLACKBOT)-build ./cmd/gtslack
	@echo "==> Creating deploy directory if needed..."
	@mkdir -p $(DEPLOY_DIR)
	@echo "==> Deploying $(SLACKBOT) to $(DEPLOY_DIR)/$(SLACKBOT)..."
	@mv /tmp/$(SLACKBOT)-build $(DEPLOY_DIR)/$(SLACKBOT)
	@echo "✓ Deployed $(SLACKBOT) to $(DEPLOY_DIR)/$(SLACKBOT) (deprecated)"

# Deploy both gt and gtslack (gtslack deprecated)
deploy-all: deploy deploy-slackbot
	@echo "✓ Deployed all binaries"
	@echo "NOTE: gtslack is deprecated. Use 'gt slack start' instead."
