CONTAINER_RUNTIME ?= docker
IMAGE_NAME ?= ghcr.io/r-dson/abox:main

.PHONY: build install test

test: build
	@echo "Running Integration Tests..."
	IMAGE_NAME=$(IMAGE_NAME) ./tests/integration-tests.sh
	@echo "Running UX Verification..."
	IMAGE_NAME=$(IMAGE_NAME) ./tests/ux-verification.sh

build:
	$(CONTAINER_RUNTIME) build -t $(IMAGE_NAME) -f docker/Dockerfile docker/

install: build
	sudo cp bin/abx /usr/local/bin/abx
	sudo chmod +x /usr/local/bin/abx
	@echo "Installation complete. ABox installed to /usr/local/bin/abx"
