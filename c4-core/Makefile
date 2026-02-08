# C4 Core - Go Implementation
# ============================

.PHONY: build test lint vet clean proto-gen fmt help

# Build variables
BINARY_NAME := c4
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

# Default target
.DEFAULT_GOAL := help

## build: Build the c4 binary
build:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/c4/

## test: Run all tests
test:
	go test -v -race -count=1 ./...

## test-cover: Run tests with coverage report
test-cover:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## vet: Run go vet
vet:
	go vet ./...

## fmt: Format Go source files
fmt:
	gofmt -s -w .
	goimports -w .

## proto-gen: Generate Go code from proto files
proto-gen:
	protoc \
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/c4_bridge.proto

## clean: Remove build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

## tidy: Tidy and verify module dependencies
tidy:
	go mod tidy
	go mod verify

## check: Run all checks (vet, lint, test)
check: vet lint test

## help: Show this help message
help:
	@echo "C4 Core - Available targets:"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /' | column -t -s ':'
