CONTAINER_RUNTIME ?= docker
IMAGE_NAME ?= ghcr.io/r-dson/abox
ABX_EDITOR ?= opencode

# Extract build metadata from config/editors.json
VERSION = $(shell jq -r '.editors["$(ABX_EDITOR)"].version' config/editors.json)
INSTALL_CMD_RAW = $(shell jq -r '.editors["$(ABX_EDITOR)"].install_cmd' config/editors.json)
INSTALL_CMD = $(subst {version},$(VERSION),$(INSTALL_CMD_RAW))
COMMAND_NAME = $(shell jq -r '.editors["$(ABX_EDITOR)"].cmd_name' config/editors.json)

IMAGE_TAG = $(IMAGE_NAME):$(ABX_EDITOR)

.PHONY: build install test bundle

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
	@mkdir -p bin
	@echo '#!/bin/bash' > bin/abx
	@echo '# ABox - Agnostic Sandbox Runtime' >> bin/abx
	@echo '# This is a generated file - do not edit directly' >> bin/abx
	@echo '# Source files are in src/ directory' >> bin/abx
	@echo '' >> bin/abx
	@# Add HOST_UID and HOST_GID definitions (needed for chown)
	@echo 'HOST_UID=$${HOST_UID:-$$(id -u)}' >> bin/abx
	@echo 'HOST_GID=$${HOST_GID:-$$(id -g)}' >> bin/abx
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
	@echo "Bundle created: bin/abx"

# Install bundled script
install: bundle
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
