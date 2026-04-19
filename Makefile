.PHONY: help build test lint lint-fix fmt fmt-check tidy tidy-check check clean

OUT_DIR := out
BINARY  := $(OUT_DIR)/overlay
PKG     := ./...

.DEFAULT_GOAL := help

help: ## list tasks
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ \
	     {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## compile binary to ./out/overlay
	@mkdir -p $(OUT_DIR)
	go build -o $(BINARY) ./cmd/overlay

test: ## run tests with -race
	go test -race $(PKG)

lint: ## run golangci-lint
	golangci-lint run $(PKG)

lint-fix: ## run golangci-lint with --fix
	golangci-lint run --fix $(PKG)

fmt: ## apply gofmt -w
	gofmt -w .

fmt-check: ## fail if gofmt would change files
	@diff=$$(gofmt -l .); rc=$$?; \
	if [ $$rc -ne 0 ]; then echo "gofmt failed (rc=$$rc)"; exit $$rc; fi; \
	if [ -n "$$diff" ]; then echo "gofmt issues:"; echo "$$diff"; exit 1; fi

tidy: ## apply go mod tidy
	go mod tidy

tidy-check: ## fail if go mod tidy would change go.mod/go.sum
	@out=$$(go mod tidy -diff); rc=$$?; \
	if [ -n "$$out" ]; then echo "$$out"; echo "go mod tidy would change go.mod/go.sum"; exit 1; fi; \
	exit $$rc

check: fmt-check tidy-check lint test ## CI gate: fmt-check + tidy-check + lint + test

clean: ## remove build artifacts + test cache
	rm -rf $(OUT_DIR) coverage.out coverage.html
	go clean -testcache
