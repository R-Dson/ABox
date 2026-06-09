CONTAINER_RUNTIME ?= docker
IMAGE_NAME ?= ghcr.io/r-dson/abox
ABX_EDITOR ?= opencode

# Extract editor image metadata from config/editors.json
EDITOR_VERSION ?= $(shell jq -r '.editors["$(ABX_EDITOR)"].version' config/editors.json)
INSTALL_CMD_RAW = $(shell jq -r '.editors["$(ABX_EDITOR)"].install_cmd' config/editors.json)
INSTALL_CMD = $(subst {version},$(EDITOR_VERSION),$(INSTALL_CMD_RAW))
COMMAND_NAME = $(shell jq -r '.editors["$(ABX_EDITOR)"].cmd_name' config/editors.json)

IMAGE_TAG = $(IMAGE_NAME):$(ABX_EDITOR)

.PHONY: build install test bundle

# Default target - build Docker image
build:
	@echo "Building image for $(ABX_EDITOR) (version: $(EDITOR_VERSION))..."
	$(CONTAINER_RUNTIME) build -t $(IMAGE_TAG) \
		--build-arg INSTALL_CMD='$(INSTALL_CMD)' \
		--build-arg VERSION="$(EDITOR_VERSION)" \
		--build-arg COMMAND_NAME="$(COMMAND_NAME)" \
		-f docker/Dockerfile \
		.

# Bundle source files into single script
bundle:
	@echo "Bundling source files..."
	@mkdir -p bin
	@echo '#!/bin/bash' > bin/abx
	@echo '# ABox - Agnostic Sandbox Runtime' >> bin/abx
	@echo '# This is a generated file - do not edit directly' >> bin/abx
	@echo '# Source files are in src/ directory' >> bin/abx
	@echo '' >> bin/abx
	@echo 'ABX_VERSION="$(ABX_VERSION)"' >> bin/abx
	@echo '' >> bin/abx
	@# Add HOST_UID and HOST_GID definitions (needed for chown)
	@echo 'HOST_UID=$${HOST_UID:-$$(id -u)}' >> bin/abx
	@echo 'HOST_GID=$${HOST_GID:-$$(id -g)}' >> bin/abx
	@echo '' >> bin/abx
	@cat src/state.sh >> bin/abx
	@echo '' >> bin/abx
	@cat src/helpers.sh >> bin/abx
	@echo '' >> bin/abx
	@cat src/exclusion.sh >> bin/abx
	@echo '' >> bin/abx
	@cat src/audit.sh >> bin/abx
	@echo '' >> bin/abx
	@cat src/container.sh >> bin/abx
	@echo '' >> bin/abx
	@cat src/sync.sh >> bin/abx
	@echo '' >> bin/abx
	@# Strip out the source statements, SCRIPT_DIR, and HOST_UID/GID from main.sh
	@tail -n +9 src/main.sh | grep -v '^source ' | grep -v 'SCRIPT_DIR=' | grep -v 'HOST_UID=' | grep -v 'HOST_GID=' >> bin/abx
	@chmod +x bin/abx
	@cp config/editors.json bin/editors.json
	@echo "Bundle created: bin/abx"

# Install bundled script
install: bundle
	sudo cp bin/abx /usr/local/bin/abx
	sudo chmod +x /usr/local/bin/abx
	sudo mkdir -p /usr/local/share/abx
	sudo cp config/editors.json /usr/local/share/abx/editors.json
	sudo mkdir -p /etc/abox
	sudo cp config/seccomp.json /etc/abox/seccomp.json
	@echo "Installation complete. ABox installed to /usr/local/bin/abx"

test: build
	@echo "Running Exclusion Unit Tests..."
	./tests/exclusion-unit-test.sh
	@echo "Running Exclusion Fuzz Tests..."
	ABX_FUZZ_COUNT=200 ./tests/exclusion-fuzz.sh
	@echo "Running Editor Registry Tests..."
	./tests/editor-registry-test.sh
	@echo "Running Sync Unit Tests..."
	./tests/sync-unit-test.sh
	@echo "Running Integration Tests..."
	IMAGE_NAME=$(IMAGE_TAG) ./tests/integration-tests.sh
	@echo "Running UX Verification..."
	IMAGE_NAME=$(IMAGE_TAG) ./tests/ux-verification.sh

# ── Go targets ──────────────────────────────────────────────────────────

.PHONY: go-build go-test go-lint go-install go-cover go-race-cover

CLI_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -s -w -X main.version=$(CLI_VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
GO_COVER_PACKAGES := $(shell go list ./... | grep -v -E '(cmd/abx$$|internal/runtime$$)')

go-build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o abx ./cmd/abx

go-test:
	go test -count=1 ./...

go-lint:
	go tool golangci-lint run ./...

go-install: go-build
	install -d $(DESTDIR)/usr/local/bin
	install -m 755 abx $(DESTDIR)/usr/local/bin/abx

go-cover:
	go test -coverprofile=coverage.out $(GO_COVER_PACKAGES)
	go tool cover -func=coverage.out | tail -1
	rm -f coverage.out

go-race-cover:
	go test -race -coverprofile=coverage.out $(GO_COVER_PACKAGES)
	go tool cover -func=coverage.out | tail -1
	rm -f coverage.out

# ── Dev image ──────────────────────────────────────────────────────────

.PHONY: build-dev
build-dev:
	@echo "Generating editor install script..."
	@jq -r '[.editors | to_entries[] | "# " + .key + "\n" + .value.install_cmd] | join("\n")' config/editors.json > docker/install-editors.sh.tmp
	@# Substitute {version} placeholders with actual versions from the registry.
	@jq -r '.editors | to_entries[] | "\(.key)=\(.value.version)"' config/editors.json | while IFS='=' read -r key ver; do \
	    sed -i "s/{version}/$$ver/g" docker/install-editors.sh.tmp ; \
	done
	@mv docker/install-editors.sh.tmp docker/install-editors.sh
	@echo "Building dev image with all editors..."
	$(CONTAINER_RUNTIME) build \
		-t $(IMAGE_NAME):dev \
		-f docker/Dockerfile.dev \
		.
	@echo "Dev image built: $(IMAGE_NAME):dev"
	@rm -f docker/install-editors.sh

# Ensure the generated install script is gitignored
docker/install-editors.sh
