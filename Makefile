.DEFAULT_GOAL := help
.PHONY: help build install test fmt fmt-check tidy tidy-check lint version-check vuln check release-snapshot clean

BUILD_DIR ?= build
BIN ?= $(BUILD_DIR)/overlay
VERSION := $(shell git describe --tags --dirty --always 2>/dev/null || printf 'unknown')
LDFLAGS := -ldflags "-X github.com/jmcampanini/overlay/cmd.Version=$(VERSION)"

help: ## Show available targets.
	@printf 'Usage: make <target>\n\nTargets:\n'
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST) | LC_ALL=C sort

build: ## Build overlay to build/overlay.
	@mkdir -p $(dir $(BIN))
	go build -trimpath -buildvcs=false $(LDFLAGS) -o $(BIN) .

install: ## Install the CLI.
	go install .

test: ## Run all tests uncached with the race detector.
	go test -count=1 -race ./...

fmt: ## Format Go source files.
	go tool golangci-lint fmt

fmt-check: ## Verify formatting without changing files.
	go tool golangci-lint fmt --diff

tidy: ## Apply go mod tidy.
	go mod tidy

tidy-check: ## Verify go.mod and go.sum are tidy without changing them.
	go mod tidy -diff

lint: ## Run static analysis.
	go tool golangci-lint run

version-check: build ## Verify the built binary reports the injected version.
	@case "$(VERSION)" in unknown|n/a|"") echo "degenerate version identity: '$(VERSION)'"; exit 1;; esac
	@out="$$($(BIN) --version)" || exit $$?; \
	if [ "$$out" != "overlay version $(VERSION)" ]; then \
		echo "version mismatch: got '$$out', want 'overlay version $(VERSION)'"; \
		exit 1; \
	fi

vuln: ## Check dependencies and reachable code for known vulnerabilities.
	go tool govulncheck ./...

check: fmt-check tidy-check lint test build version-check vuln ## Run the complete local verification contract.

release-snapshot: ## Build release archives into dist/ with GoReleaser without publishing.
	@command -v goreleaser >/dev/null 2>&1 || { echo "goreleaser not found on PATH; install it with 'mise use -g goreleaser' or 'brew install goreleaser'"; exit 1; }
	goreleaser check
	goreleaser release --snapshot --clean

clean: ## Remove local build artifacts and the Go test cache.
	rm -rf $(BUILD_DIR) dist
	go clean -testcache
