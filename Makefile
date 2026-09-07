.PHONY: help build test lint lint-fix fmt fmt-check tidy tidy-check version-check vuln check clean

BUILD_DIR   := build
BINARY      := $(BUILD_DIR)/overlay
CMD         := .
PKG         := ./...

VERSION := $(shell git describe --tags --dirty --always 2>/dev/null || printf 'unknown')
LDFLAGS := -ldflags "-X github.com/jmcampanini/overlay/cmd.Version=$(VERSION)"

.DEFAULT_GOAL := help

help: ## Show this help.
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_.-]+:.*##/ { printf "  %-16s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

build: ## Build overlay into ./build/overlay.
	@mkdir -p $(BUILD_DIR)
	go build -trimpath -buildvcs=false $(LDFLAGS) -o $(BINARY) $(CMD)

test: ## Run all tests uncached with the race detector.
	go test -count=1 -race $(PKG)

lint: ## Run golangci-lint.
	go tool golangci-lint run $(PKG)

lint-fix: ## Run golangci-lint with --fix.
	go tool golangci-lint run --fix $(PKG)

fmt: ## Format Go source files.
	go tool golangci-lint fmt

fmt-check: ## Verify formatting without changing files.
	go tool golangci-lint fmt --diff

tidy: ## Apply go mod tidy.
	go mod tidy

tidy-check: ## Fail if go mod tidy would change go.mod/go.sum.
	@out=$$(go mod tidy -diff); rc=$$?; \
	if [ $$rc -eq 0 ]; then exit 0; fi; \
	if [ -n "$$out" ]; then echo "$$out"; echo "go mod tidy would change go.mod/go.sum"; exit 1; fi; \
	echo "go mod tidy failed (rc=$$rc)"; exit $$rc

version-check: build ## Verify the built binary reports the injected version.
	@case "$(VERSION)" in unknown|n/a|"") echo "degenerate version identity: '$(VERSION)'"; exit 1;; esac
	@out="$$($(BINARY) --version)" || exit $$?; \
	if [ "$$out" != "overlay version $(VERSION)" ]; then \
		echo "version mismatch: got '$$out', want 'overlay version $(VERSION)'"; \
		exit 1; \
	fi

vuln: ## Check dependencies and reachable code for known vulnerabilities.
	go tool govulncheck ./...

check: fmt-check tidy-check lint test build version-check vuln ## Run the complete local verification contract.

clean: ## Remove build artifacts, coverage files, and test cache.
	rm -rf $(BUILD_DIR) out dist coverage.out coverage.html *.coverprofile
	go clean -testcache
