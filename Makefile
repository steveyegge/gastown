.PHONY: build install clean test generate

BINARY := gt
BUILD_DIR := .
GO := go
GOROOT_VALID := $(shell [ -n "$(GOROOT)" ] && [ -d "$(GOROOT)" ] && echo yes)
ifeq ($(GOROOT_VALID),)
GO := env -u GOROOT go
endif

GOBIN ?= $(shell $(GO) env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell $(GO) env GOPATH)/bin
endif

# Get version info for ldflags
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -X github.com/steveyegge/gastown/internal/cmd.Version=$(VERSION) \
           -X github.com/steveyegge/gastown/internal/cmd.Commit=$(COMMIT) \
           -X github.com/steveyegge/gastown/internal/cmd.BuildTime=$(BUILD_TIME)

generate:
	$(GO) generate ./...

build: generate
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/gt
ifeq ($(shell uname),Darwin)
	@codesign -s - -f $(BUILD_DIR)/$(BINARY) 2>/dev/null || true
	@echo "Signed $(BINARY) for macOS"
endif

install: generate
	GOBIN=$(GOBIN) $(GO) install -ldflags "$(LDFLAGS)" ./cmd/gt
ifeq ($(shell uname),Darwin)
	@codesign -s - -f $(GOBIN)/$(BINARY) 2>/dev/null || true
endif

clean:
	rm -f $(BUILD_DIR)/$(BINARY)

test:
	$(GO) test ./...
