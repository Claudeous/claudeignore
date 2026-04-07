.PHONY: build test vet lint clean dev prod which

BINARY_NAME=claudeignore
BUILD_DIR=bin
LINK_PATH=$(HOME)/bin/$(BINARY_NAME)
DEV_BIN=$(CURDIR)/$(BUILD_DIR)/$(BINARY_NAME)
BREW_BIN=$(shell brew --prefix 2>/dev/null)/bin/$(BINARY_NAME)
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

## dev/prod switch — requires ~/bin in PATH

dev: build
	@ln -sf $(DEV_BIN) $(LINK_PATH)
	@echo "→ dev (local build)"

prod:
	@if [ ! -x $(BREW_BIN) ]; then \
		echo "error: brew binary not found at $(BREW_BIN)" >&2; \
		echo "  run: brew install claudeignore" >&2; exit 1; \
	fi
	@ln -sf $(BREW_BIN) $(LINK_PATH)
	@echo "→ prod ($(BREW_BIN))"

which:
	@TARGET=$$(readlink $(LINK_PATH) 2>/dev/null); \
	if [ "$$TARGET" = "$(DEV_BIN)" ]; then \
		echo "dev (local build)"; \
	elif [ "$$TARGET" = "$(BREW_BIN)" ]; then \
		echo "prod (brew)"; \
	elif [ -n "$$TARGET" ]; then \
		echo "custom: $$TARGET"; \
	else \
		echo "no symlink at $(LINK_PATH)"; \
	fi

test:
	go test -race -v ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf $(BUILD_DIR)

fmt:
	go fmt ./...
	goimports -w .

cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"
