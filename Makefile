.PHONY: build test sqlc tidy run

GO ?= go
LDFLAGS := -X github.com/cyrus/glutton/internal/version.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev) \
           -X github.com/cyrus/glutton/internal/version.Commit=$(shell git rev-parse --short HEAD 2>/dev/null || echo none) \
           -X github.com/cyrus/glutton/internal/version.Date=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/glutton ./cmd/glutton

test:
	$(GO) test ./...

tidy:
	$(GO) mod tidy

sqlc:
	sqlc generate

run: build
	./bin/glutton
