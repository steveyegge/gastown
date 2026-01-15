.PHONY: build install clean test test-e2e test-e2e-container generate container-test

BINARY := gt
BUILD_DIR := .
INSTALL_DIR := $(HOME)/.local/bin
CONTAINER_IMAGE ?= golang:1.24.2
BEADS_VERSION ?= v0.47.2

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

clean:
	rm -f $(BUILD_DIR)/$(BINARY)

test:
	go test ./...

container-test:
	@docker run --rm \
		-e HOST_UID=$(shell id -u) \
		-e HOST_GID=$(shell id -g) \
		-e BEADS_VERSION=$(BEADS_VERSION) \
		-v $(CURDIR):/work \
		-w /work \
		$(CONTAINER_IMAGE) \
		sh -c 'set -eu; \
			apt-get update -qq; \
			apt-get install -y -qq git tmux passwd; \
			groupadd -g "$$HOST_GID" hostgroup || true; \
			useradd -m -u "$$HOST_UID" -g "$$HOST_GID" hostuser || true; \
			su hostuser -c "git config --global --add safe.directory /work"; \
			CGO_ENABLED=1 su hostuser -c "go install github.com/steveyegge/beads/cmd/bd@$$BEADS_VERSION"; \
			su hostuser -c "go build -o /tmp/gt ./cmd/gt"; \
			su hostuser -c "PATH=/home/hostuser/go/bin:$$PATH go test ./..."; \
		'

# Run e2e tests locally (may have false failures from host environment leakage)
test-e2e:
	go test -tags=integration -run 'TestInstallDoctorClean' ./internal/cmd -v -timeout=5m

# Run e2e tests in isolated container (recommended for CI)
test-e2e-container:
	docker build -f Dockerfile.e2e -t gastown-test .
	docker run --rm gastown-test
