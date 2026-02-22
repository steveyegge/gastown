.PHONY: build install clean test test-e2e-container check-up-to-date

BINARY := gt
BUILD_DIR := .
INSTALL_DIR := $(HOME)/.local/bin

# Get version info for ldflags
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -X github.com/steveyegge/gastown/internal/cmd.Version=$(VERSION) \
           -X github.com/steveyegge/gastown/internal/cmd.Commit=$(COMMIT) \
           -X github.com/steveyegge/gastown/internal/cmd.BuildTime=$(BUILD_TIME) \
           -X github.com/steveyegge/gastown/internal/cmd.BuiltProperly=1

# ICU4C detection for macOS (required by go-icu-regex transitive dependency).
# Homebrew installs icu4c as a keg-only package, so headers/libs aren't on the
# default search path. Auto-detect the prefix and export CGo flags.
ifeq ($(shell uname),Darwin)
  ICU_PREFIX := $(shell brew --prefix icu4c 2>/dev/null)
  ifneq ($(ICU_PREFIX),)
    export CGO_CPPFLAGS += -I$(ICU_PREFIX)/include
    export CGO_LDFLAGS  += -L$(ICU_PREFIX)/lib
  endif
endif

build:
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/gt
ifeq ($(shell uname),Darwin)
	@codesign -s - -f $(BUILD_DIR)/$(BINARY) 2>/dev/null || true
	@echo "Signed $(BINARY) for macOS"
endif

check-up-to-date:
ifndef SKIP_UPDATE_CHECK
	@git fetch origin main --quiet 2>/dev/null || true
	@LOCAL=$$(git rev-parse HEAD 2>/dev/null); \
	REMOTE=$$(git rev-parse origin/main 2>/dev/null); \
	if [ -n "$$REMOTE" ] && [ "$$LOCAL" != "$$REMOTE" ]; then \
		echo "ERROR: Local branch is not up to date with origin/main"; \
		echo "  Local:  $$(git rev-parse --short HEAD)"; \
		echo "  Remote: $$(git rev-parse --short origin/main)"; \
		echo "Run 'git pull' first, or use SKIP_UPDATE_CHECK=1 to override"; \
		exit 1; \
	fi
endif

install: check-up-to-date build
	@mkdir -p $(INSTALL_DIR)
	@rm -f $(INSTALL_DIR)/$(BINARY)
	@cp $(BUILD_DIR)/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@# Nuke any stale go-install binaries that shadow the canonical location
	@for bad in $(HOME)/go/bin/$(BINARY) $(HOME)/bin/$(BINARY); do \
		if [ -f "$$bad" ]; then \
			echo "Removing stale $$bad (use make install, not go install)"; \
			rm -f "$$bad"; \
		fi; \
	done
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"
	@# Restart daemon so it picks up the new binary.
	@# A stale daemon is a recurring source of bugs (wrong session prefixes, etc.)
	@if $(INSTALL_DIR)/$(BINARY) daemon status >/dev/null 2>&1; then \
		echo "Restarting daemon to pick up new binary..."; \
		$(INSTALL_DIR)/$(BINARY) daemon stop >/dev/null 2>&1 || true; \
		sleep 1; \
		$(INSTALL_DIR)/$(BINARY) daemon start >/dev/null 2>&1 && \
			echo "Daemon restarted." || \
			echo "Warning: daemon restart failed (start manually with: gt daemon start)"; \
	fi

clean:
	rm -f $(BUILD_DIR)/$(BINARY)

test:
	go test ./...

# Run e2e tests in isolated container (the only supported way to run them)
test-e2e-container:
	docker build -f Dockerfile.e2e -t gastown-test .
	docker run --rm gastown-test
