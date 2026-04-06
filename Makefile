.PHONY: build test test-race cover vet fmt fmt-check lint lint-fix check parity clean

BUILD_DIR := build
BINARY    := $(BUILD_DIR)/overlay
PKG       := ./...

build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BINARY) ./cmd/overlay

test:
	go test $(PKG)

test-race:
	go test -race $(PKG)

cover:
	go test -cover $(PKG)

vet:
	go vet $(PKG)

fmt:
	gofmt -w .

fmt-check:
	@diff=$$(gofmt -l . | grep -v '^vendor/' || true); \
	if [ -n "$$diff" ]; then echo "gofmt issues:"; echo "$$diff"; exit 1; fi

lint:
	golangci-lint run $(PKG)

lint-fix:
	golangci-lint run --fix $(PKG)

check: fmt-check vet lint test

parity:
	go test -tags=parity $(PKG) -run TestParity

clean:
	rm -rf $(BUILD_DIR) coverage.out coverage.html
