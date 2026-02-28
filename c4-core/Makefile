# C4 Core - Go Implementation
# ============================

.PHONY: build test test-grpc lint vet clean proto-gen fmt help \
        build-solo build-connected build-full build-nightly \
        install-solo install-connected install-full install-nightly \
        embed-skills embed-c5 build-cross

# Build variables
BINARY_NAME := c4
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Supabase built-in credentials — injected only when CI env vars are set.
# Local builds leave these empty; users can override via C4_CLOUD_URL / SUPABASE_URL env vars.
_SUPABASE_FLAGS :=
ifneq ($(SUPABASE_URL),)
  _SUPABASE_FLAGS += -X main.builtinSupabaseURL=$(SUPABASE_URL)
endif
ifneq ($(SUPABASE_ANON_KEY),)
  _SUPABASE_FLAGS += -X main.builtinSupabaseKey=$(SUPABASE_ANON_KEY)
endif

# C5 Hub version to embed when building with c5_embed tag.
# Set C5_VERSION in CI (e.g. from git tag or release version).
C5_VERSION ?= $(shell cd $(REPO_ROOT)/c5 2>/dev/null && git describe --tags --always --dirty 2>/dev/null || echo "dev")

LDFLAGS := -ldflags "-X main.version=$(VERSION) -X 'github.com/changmin/c4-core/cmd/c4.embeddedC5Version=$(C5_VERSION)' $(_SUPABASE_FLAGS)"

# Install destination (Claude Code reads this binary via .mcp.json)
INSTALL_DIR := $(HOME)/.local/bin
INSTALL_BIN := $(INSTALL_DIR)/cq

# Repository root (absolute path, used for embed-skills)
REPO_ROOT := $(shell git -C $(dir $(abspath $(lastword $(MAKEFILE_LIST)))) rev-parse --show-toplevel)

# C5 embed variables (used by CI embed-c5 target)
# C5_BIN: path to the compiled c5 binary (set by CI after build-c5 stage)
# C5_VERSION: version string injected into embeddedC5Version ldflags
C5_BIN     ?=
C5_VERSION ?= dev
EXT        ?=

# Default target
.DEFAULT_GOAL := help

## embed-skills: Copy .claude/skills/ into cmd/c4/skills_src/ (dereferences symlinks)
embed-skills:
	rm -rf cmd/c4/skills_src
	rsync -rL $(REPO_ROOT)/.claude/skills/ cmd/c4/skills_src/
	echo "$(shell git -C $(REPO_ROOT) rev-parse --short HEAD)" > cmd/c4/skills_src/.version
	@echo "Skills embedded from $(REPO_ROOT)/.claude/skills/ ($(shell git -C $(REPO_ROOT) rev-parse --short HEAD))"

## embed-c5: Copy c5 binary into cmd/c4/embed/c5/ (CI use only; C5_BIN must be set)
embed-c5:
	@test -n "$(C5_BIN)" || (echo "Error: C5_BIN not set. Usage: make embed-c5 C5_BIN=../c5/bin/c5-linux-amd64" && exit 1)
	mkdir -p cmd/c4/embed/c5
	cp $(C5_BIN) cmd/c4/embed/c5/c5$(EXT)
	@echo "Embedded c5 binary: $(C5_BIN) → cmd/c4/embed/c5/c5$(EXT)"

# Cross-compilation target — output goes to dist/ for CI artifact collection
# Usage: make build-cross GOOS=linux GOARCH=amd64 TIER=solo
TIER_TAGS_solo      = skills_embed
TIER_TAGS_connected = c0_drive c3_eventbus llm_gateway skills_embed
TIER_TAGS_full      = c0_drive c3_eventbus c5_hub llm_gateway cdp gpu c1_messenger research skills_embed c5_embed

# Extra ldflags for c5_embed tier: inject embeddedC5Version
_C5_EMBED_FLAGS :=
ifeq ($(TIER),full)
  _C5_EMBED_FLAGS := -X 'github.com/changmin/c4-core/cmd/c4.embeddedC5Version=$(C5_VERSION)'
endif

## build-cross: Cross-compile for target GOOS/GOARCH/TIER (used by CI)
build-cross: embed-skills
	@mkdir -p dist
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build \
		-ldflags "-X main.version=$(VERSION) $(_SUPABASE_FLAGS) $(_C5_EMBED_FLAGS)" \
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

## lint-skills: Skill 계약 검증 (deprecated stub 크기 + knowledge gate); archtest는 test 타겟에서도 실행됨
lint-skills:
	bash ../scripts/lint-skills.sh -race -count=1

## check: Run all checks (vet, lint, test)
## NOTE: lint-skills archtest는 test 타겟(go test ./...)에 포함되어 있어 check에서 중복 제외
check: vet lint test

## help: Show this help message
help:
	@echo "C4 Core - Available targets:"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /' | column -t -s ':'
