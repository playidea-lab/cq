# C4 Core - Go Implementation
# ============================

.PHONY: build test test-grpc lint vet clean proto-gen fmt help \
        build-solo build-connected build-full build-nightly \
        install-solo install-connected install-full install-nightly \
        embed-skills build-cross

# Build variables
BINARY_NAME := c4
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

# Install destination (Claude Code reads this binary via .mcp.json)
INSTALL_DIR := $(HOME)/.local/bin
INSTALL_BIN := $(INSTALL_DIR)/cq

# Repository root (absolute path, used for embed-skills)
REPO_ROOT := $(shell git -C $(dir $(abspath $(lastword $(MAKEFILE_LIST)))) rev-parse --show-toplevel)

# Default target
.DEFAULT_GOAL := help

## embed-skills: Copy .claude/skills/ into cmd/c4/skills_src/ (dereferences symlinks)
embed-skills:
	rm -rf cmd/c4/skills_src
	rsync -rL $(REPO_ROOT)/.claude/skills/ cmd/c4/skills_src/
	echo "$(shell git -C $(REPO_ROOT) rev-parse --short HEAD)" > cmd/c4/skills_src/.version
	@echo "Skills embedded from $(REPO_ROOT)/.claude/skills/ ($(shell git -C $(REPO_ROOT) rev-parse --short HEAD))"

# Cross-compilation target — output goes to dist/ for CI artifact collection
# Usage: make build-cross GOOS=linux GOARCH=amd64 TIER=solo
TIER_TAGS_solo      = skills_embed
TIER_TAGS_connected = c0_drive c3_eventbus llm_gateway skills_embed
TIER_TAGS_full      = c0_drive c3_eventbus c5_hub llm_gateway cdp gpu c1_messenger research skills_embed

## build-cross: Cross-compile for target GOOS/GOARCH/TIER (used by CI)
build-cross: embed-skills
	@mkdir -p dist
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) \
		-tags '$(TIER_TAGS_$(TIER))' \
		-o dist/cq-$(TIER)-$(GOOS)-$(GOARCH) \
		./cmd/c4/

## build: Build the c4 binary (full, all tags)
build:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/c4/

## build-solo: Build offline-only binary (skills_embed, no external deps)
build-solo: embed-skills
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -tags 'skills_embed' -o $(BUILD_DIR)/cq-solo ./cmd/c4/

## build-connected: Build with cloud/LLM/EventBus integrations + skills_embed
build-connected: embed-skills
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -tags 'c0_drive c3_eventbus llm_gateway skills_embed' -o $(BUILD_DIR)/cq-connected ./cmd/c4/

## build-full: Build with all integrations (Hub, CDP, GPU, C1, Research) + skills_embed
build-full: embed-skills
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -tags 'c0_drive c3_eventbus c5_hub llm_gateway cdp gpu c1_messenger research skills_embed' -o $(BUILD_DIR)/cq-full ./cmd/c4/

## build-nightly: Build with all integrations including Observe/Guard/Gate (C7/C6/C8) + skills_embed
build-nightly: embed-skills
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -tags 'c0_drive c3_eventbus c5_hub llm_gateway cdp gpu c1_messenger research c7_observe c6_guard c8_gate skills_embed' -o $(BUILD_DIR)/cq-nightly ./cmd/c4/

## install-nightly: Build nightly tier and install to ~/.local/bin/cq
install-nightly: build-nightly
	@mkdir -p $(dir $(INSTALL_BIN))
	@cp $(BUILD_DIR)/cq-nightly $(INSTALL_BIN).tmp && mv $(INSTALL_BIN).tmp $(INSTALL_BIN) || { rm -f $(INSTALL_BIN).tmp; exit 1; }
	@echo "Installed nightly tier → $(INSTALL_BIN)"

## install-solo: Build solo tier and install to ~/.local/bin/cq
install-solo: build-solo
	@mkdir -p $(dir $(INSTALL_BIN))
	@cp $(BUILD_DIR)/cq-solo $(INSTALL_BIN).tmp && mv $(INSTALL_BIN).tmp $(INSTALL_BIN) || { rm -f $(INSTALL_BIN).tmp; exit 1; }
	@echo "Installed solo tier → $(INSTALL_BIN)"

## install-connected: Build connected tier and install to ~/.local/bin/cq
install-connected: build-connected
	@mkdir -p $(dir $(INSTALL_BIN))
	@cp $(BUILD_DIR)/cq-connected $(INSTALL_BIN).tmp && mv $(INSTALL_BIN).tmp $(INSTALL_BIN) || { rm -f $(INSTALL_BIN).tmp; exit 1; }
	@echo "Installed connected tier → $(INSTALL_BIN)"

## install-full: Build full tier and install to ~/.local/bin/cq
install-full: build-full
	@mkdir -p $(dir $(INSTALL_BIN))
	@cp $(BUILD_DIR)/cq-full $(INSTALL_BIN).tmp && mv $(INSTALL_BIN).tmp $(INSTALL_BIN) || { rm -f $(INSTALL_BIN).tmp; exit 1; }
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
	rm -rf $(BUILD_DIR) dist
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
