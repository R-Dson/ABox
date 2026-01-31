CONTAINER_RUNTIME ?= docker
IMAGE_NAME ?= ghcr.io/r-dson/abox:main
IMAGE_TAG = $(shell echo $(IMAGE_NAME) | tr '[:upper:]' '[:lower:]')

.PHONY: build install test

test: build
	@echo "Running Integration Tests..."
	IMAGE_NAME=$(IMAGE_TAG) ./tests/integration-tests.sh
	@echo "Running UX Verification..."
	IMAGE_NAME=$(IMAGE_TAG) ./tests/ux-verification.sh

OPENCODE_VERSION ?= $(shell cat OPENCODE_VERSION 2>/dev/null || echo "")

build:
	$(CONTAINER_RUNTIME) build -t $(IMAGE_TAG) -f docker/Dockerfile \
		$(if $(OPENCODE_VERSION),--build-arg OPENCODE_VERSION=$(OPENCODE_VERSION),) \
		docker/

install: build
	sudo cp bin/abx /usr/local/bin/abx
	sudo chmod +x /usr/local/bin/abx
	@echo "Installation complete. ABox installed to /usr/local/bin/abx"
