VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "1.0.0")
CONTAINER_RUNTIME ?= docker

.PHONY: build install test

test: build
	@echo "Running Integration Tests..."
	./tests/integration-tests.sh
	@echo "Running UX Verification..."
	./tests/ux-verification.sh

build:
	$(CONTAINER_RUNTIME) build -t abox:latest -f docker/Dockerfile docker/

install: build
	@sed -i 's/VERSION=".*"/VERSION="$(VERSION)"/' bin/abx
	sudo cp bin/abx /usr/local/bin/abx
	sudo chmod +x /usr/local/bin/abx
	@echo "Installation complete. ABox $(VERSION) installed to /usr/local/bin/abx"
