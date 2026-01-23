.PHONY: build install clean test generate deploy

BINARY := gt
BUILD_DIR := .
DEPLOY_DIR := $(HOME)/local/bin
TMP_BUILD := /tmp/$(BINARY)-build

# Get version info for ldflags
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -X github.com/steveyegge/gastown/internal/cmd.Version=$(VERSION) \
           -X github.com/steveyegge/gastown/internal/cmd.Commit=$(COMMIT) \
           -X github.com/steveyegge/gastown/internal/cmd.BuildTime=$(BUILD_TIME)

generate:
	go generate ./...

build: generate
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/gt
ifeq ($(shell uname),Darwin)
	@codesign -s - -f $(BUILD_DIR)/$(BINARY) 2>/dev/null || true
	@echo "Signed $(BINARY) for macOS"
endif

install: generate
	go install -ldflags "$(LDFLAGS)" ./cmd/gt

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
	rm -f $(TMP_BUILD)

test:
	go test ./...
