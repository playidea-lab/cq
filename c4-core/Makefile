# C4 Core - Go Implementation
# ============================

.PHONY: build test test-grpc lint vet clean proto-gen fmt help \
        build-solo build-connected build-full \
        install-solo install-connected install-full

# Build variables
BINARY_NAME := c4
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

# Install destination (Claude Code reads this binary via .mcp.json)
INSTALL_DIR := $(HOME)/.local/bin
INSTALL_BIN := $(INSTALL_DIR)/cq

# Default target
.DEFAULT_GOAL := help

## build: Build the c4 binary (full, all tags)
build:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/c4/

## build-solo: Build offline-only binary (~40 tools, no external deps)
build-solo:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/cq-solo ./cmd/c4/

## build-connected: Build with cloud/LLM/EventBus integrations
build-connected:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -tags 'c0_drive c3_eventbus llm_gateway' -o $(BUILD_DIR)/cq-connected ./cmd/c4/

## build-full: Build with all integrations (Hub, CDP, GPU, C1, Research)
build-full:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -tags 'c0_drive c3_eventbus c5_hub llm_gateway cdp gpu c1_messenger research' -o $(BUILD_DIR)/cq-full ./cmd/c4/

## install-solo: Build solo tier and install to ~/.local/bin/cq
install-solo: build-solo
	@mkdir -p $(INSTALL_DIR)
	cp $(BUILD_DIR)/cq-solo $(INSTALL_BIN)
	@echo "Installed solo tier → $(INSTALL_BIN)"

## install-connected: Build connected tier and install to ~/.local/bin/cq
install-connected: build-connected
	@mkdir -p $(INSTALL_DIR)
	cp $(BUILD_DIR)/cq-connected $(INSTALL_BIN)
	@echo "Installed connected tier → $(INSTALL_BIN)"

## install-full: Build full tier and install to ~/.local/bin/cq
install-full: build-full
	@mkdir -p $(INSTALL_DIR)
	cp $(BUILD_DIR)/cq-full $(INSTALL_BIN)
	@echo "Installed full tier → $(INSTALL_BIN)"

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

## test-grpc: Run tests including gRPC client tests (requires proto-gen first)
test-grpc:
	go test -v -race -count=1 -tags=grpc ./...

## proto-gen: Generate Go code from proto files
proto-gen:
	@mkdir -p internal/bridge/pb
	protoc \
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/c4_bridge.proto
	@echo "Generated pb files in internal/bridge/pb/"

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
