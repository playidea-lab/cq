# C4 Core - Go Implementation
# ============================

.PHONY: build test test-grpc lint vet clean proto-gen fmt help \
        build-nightly install install-nightly \
        embed-skills build-cross cross-compile

# Build variables
BINARY_NAME := c4
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Built-in credentials — injected only when CI env vars are set.
# Local builds leave these empty; users can override via env vars or .c4/config.yaml.
_SUPABASE_FLAGS :=
ifneq ($(SUPABASE_URL),)
  _SUPABASE_FLAGS += -X main.builtinSupabaseURL=$(SUPABASE_URL)
endif
ifneq ($(SUPABASE_ANON_KEY),)
  _SUPABASE_FLAGS += -X main.builtinSupabaseKey=$(SUPABASE_ANON_KEY)
endif
ifneq ($(HUB_URL),)
  _SUPABASE_FLAGS += -X main.builtinHubURL=$(HUB_URL)
endif

LDFLAGS := -ldflags "-X main.version=$(VERSION) $(_SUPABASE_FLAGS)"

# Install destination (Claude Code reads this binary via .mcp.json)
INSTALL_DIR := $(HOME)/.local/bin
INSTALL_BIN := $(INSTALL_DIR)/cq

# Repository root (absolute path, used for embed-skills)
REPO_ROOT := $(shell git -C $(dir $(abspath $(lastword $(MAKEFILE_LIST)))) rev-parse --show-toplevel)

EXT        ?=

# Default target
.DEFAULT_GOAL := help

## embed-skills: Copy .claude/skills/ into cmd/c4/skills_src/ (dereferences symlinks)
embed-skills:
	rm -rf cmd/c4/skills_src
	rsync -rL $(REPO_ROOT)/.claude/skills/ cmd/c4/skills_src/
	echo "$(shell git -C $(REPO_ROOT) rev-parse --short HEAD)" > cmd/c4/skills_src/.version
	@echo "Skills embedded from $(REPO_ROOT)/.claude/skills/ ($(shell git -C $(REPO_ROOT) rev-parse --short HEAD))"

## embed-standards: Copy standards/ into cmd/c4/standards_src/ for go:embed
embed-standards:
	rsync -rL --exclude='*.go' $(REPO_ROOT)/standards/ cmd/c4/standards_src/
	@echo "Standards embedded from $(REPO_ROOT)/standards/"

# Build tags — single tier (all features)
BUILD_TAGS = c0_drive c3_eventbus c5_hub llm_gateway cdp gpu c1_messenger research skills_embed c7_observe

## build-cross: Cross-compile for target GOOS/GOARCH (used by CI)
build-cross: embed-skills embed-standards
	@mkdir -p dist
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build \
		-ldflags "-X main.version=$(VERSION) $(_SUPABASE_FLAGS)" \
		-tags '$(BUILD_TAGS)' \
		-o dist/cq-$(GOOS)-$(GOARCH)$(EXT) \
		./cmd/c4/

## cross-compile: Build for all platforms (linux/amd64, linux/arm64, darwin/arm64)
cross-compile: embed-skills
	@mkdir -p dist
	@for target in linux/amd64 linux/arm64 darwin/arm64; do \
		GOOS=$$(echo $$target | cut -d/ -f1); \
		GOARCH=$$(echo $$target | cut -d/ -f2); \
		echo "Building $$GOOS/$$GOARCH..."; \
		GOOS=$$GOOS GOARCH=$$GOARCH go build \
			-ldflags "-X main.version=$(VERSION) $(_SUPABASE_FLAGS)" \
			-tags '$(BUILD_TAGS)' \
			-o dist/cq-$$GOOS-$$GOARCH \
			./cmd/c4/ || exit 1; \
	done
	@echo "All cross-compile targets done. Binaries in dist/"


## build: Build the cq binary (all features)
build: embed-skills embed-standards
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -tags '$(BUILD_TAGS)' -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/c4/

## build-nightly: Build with experimental features (C7/C6/C8)
build-nightly: embed-skills embed-standards
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -tags '$(BUILD_TAGS) c7_observe c6_guard c8_gate' -o $(BUILD_DIR)/cq-nightly ./cmd/c4/

## install: Build and install to ~/.local/bin/cq
install: build
	@mkdir -p $(dir $(INSTALL_BIN))
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_BIN).tmp && mv $(INSTALL_BIN).tmp $(INSTALL_BIN) || { rm -f $(INSTALL_BIN).tmp; exit 1; }
	@echo "Installed → $(INSTALL_BIN)"

## install-nightly: Build nightly and install to ~/.local/bin/cq
install-nightly: build-nightly
	@mkdir -p $(dir $(INSTALL_BIN))
	@cp $(BUILD_DIR)/cq-nightly $(INSTALL_BIN).tmp && mv $(INSTALL_BIN).tmp $(INSTALL_BIN) || { rm -f $(INSTALL_BIN).tmp; exit 1; }
	@echo "Installed nightly → $(INSTALL_BIN)"

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
