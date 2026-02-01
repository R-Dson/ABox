CONTAINER_RUNTIME ?= docker
IMAGE_NAME ?= ghcr.io/r-dson/abox
ABX_EDITOR ?= opencode

# Extract info from YAML files without yq/PyYAML
VERSION = $(shell grep "^$(ABX_EDITOR):" VERSIONS.yaml | cut -d' ' -f2)
INSTALL_CMD = $(shell awk -v e="$(ABX_EDITOR)" '$$0 ~ "^  " e ":" {f=1; next} f && /install:/ {sub(/.*install: "/, ""); sub(/".*/, ""); print; exit} f && /^[a-zA-Z]/ {f=0}' docker/editors.yaml)
COMMAND_NAME = $(shell awk -v e="$(ABX_EDITOR)" '$$0 ~ "^  " e ":" {f=1; next} f && /cmd:/ {sub(/.*cmd: "/, ""); sub(/".*/, ""); print; exit} f && /^[a-zA-Z]/ {f=0}' docker/editors.yaml)

IMAGE_TAG = $(IMAGE_NAME):$(ABX_EDITOR)

.PHONY: build install test

test: build
	@echo "Running Integration Tests..."
	IMAGE_NAME=$(IMAGE_TAG) ./tests/integration-tests.sh
	@echo "Running UX Verification..."
	IMAGE_NAME=$(IMAGE_TAG) ./tests/ux-verification.sh

build:
	@echo "Building image for $(ABX_EDITOR) (version: $(VERSION))..."
	$(CONTAINER_RUNTIME) build -t $(IMAGE_TAG) \
		--build-arg INSTALL_CMD='$(INSTALL_CMD)' \
		--build-arg VERSION="$(VERSION)" \
		--build-arg COMMAND_NAME="$(COMMAND_NAME)" \
		-f docker/Dockerfile \
		.

install: build
	sudo cp bin/abx /usr/local/bin/abx
	sudo chmod +x /usr/local/bin/abx
	@echo "Installation complete. ABox installed to /usr/local/bin/abx"

