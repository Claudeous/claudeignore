.PHONY: build test vet lint clean

BINARY_NAME=claudeignore
BUILD_DIR=bin

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) .

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
