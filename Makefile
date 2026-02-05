CONTAINER_RUNTIME ?= docker
IMAGE_NAME ?= ghcr.io/r-dson/abox
ABX_EDITOR ?= opencode

# Extract build metadata from config/editors.json
VERSION = $(shell jq -r '.editors["$(ABX_EDITOR)"].version' config/editors.json)
INSTALL_CMD_RAW = $(shell jq -r '.editors["$(ABX_EDITOR)"].install_cmd' config/editors.json)
INSTALL_CMD = $(subst {version},$(VERSION),$(INSTALL_CMD_RAW))
COMMAND_NAME = $(shell jq -r '.editors["$(ABX_EDITOR)"].cmd_name' config/editors.json)

IMAGE_TAG = $(IMAGE_NAME):$(ABX_EDITOR)

.PHONY: build install test bundle compile

# Default target - build Docker image
build:
	@echo "Building image for $(ABX_EDITOR) (version: $(VERSION))..."
	$(CONTAINER_RUNTIME) build -t $(IMAGE_TAG) \
		--build-arg INSTALL_CMD='$(INSTALL_CMD)' \
		--build-arg VERSION="$(VERSION)" \
		--build-arg COMMAND_NAME="$(COMMAND_NAME)" \
		-f docker/Dockerfile \
		.

# Bundle source files into single script
bundle:
	@echo "Bundling source files..."
	@mkdir -p build
	@echo '#!/bin/bash' > build/abx
	@echo '# ABox - Agnostic Sandbox Runtime' >> build/abx
	@echo '# This is a generated file - do not edit directly' >> build/abx
	@echo '# Source files are in src/ directory' >> build/abx
	@echo '' >> build/abx
	@echo 'set -o pipefail' >> build/abx
	@echo '' >> build/abx
	@echo '# Read main script' >> build/abx
	@cat src/helpers.sh >> build/abx
	@echo '' >> build/abx
	@cat src/sync.sh >> build/abx
	@echo '' >> build/abx
	@sed '/^source.*SCRIPT_DIR.*\.sh/d' src/main.sh | tail -n +14 >> build/abx
	@chmod +x build/abx
	@echo "Bundle created: build/abx"

# Compile to binary with shc
compile: bundle
	@echo "Compiling to binary..."
	@which shc >/dev/null 2>&1 || (echo "shc not found. Run: brew install shc" && exit 1)
	@shc -f build/abx -o bin/abx
	@rm -f build/abx.x.c
	@echo "Binary compiled: bin/abx"

# Install compiled binary
install: compile
	sudo cp bin/abx /usr/local/bin/abx
	sudo chmod +x /usr/local/bin/abx
	@echo "Installation complete. ABox installed to /usr/local/bin/abx"

test: build
	@echo "Running Integration Tests..."
	IMAGE_NAME=$(IMAGE_TAG) ./tests/integration-tests.sh
	@echo "Running UX Verification..."
	IMAGE_NAME=$(IMAGE_TAG) ./tests/ux-verification.sh
	@echo "Running Content Exclusion Unit Tests..."
	./tests/exclusion-unit-test.sh
