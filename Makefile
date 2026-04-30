.PHONY: build test sqlc tidy run web web-clean

GO ?= go
LDFLAGS := -X github.com/cyrus/glutton/internal/version.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev) \
           -X github.com/cyrus/glutton/internal/version.Commit=$(shell git rev-parse --short HEAD 2>/dev/null || echo none) \
           -X github.com/cyrus/glutton/internal/version.Date=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)

web:
	cd web && pnpm install --frozen-lockfile && pnpm build
	rm -rf internal/api/spa_dist
	cp -R web/dist internal/api/spa_dist
	touch internal/api/spa_dist/.gitkeep

web-clean:
	rm -rf web/dist internal/api/spa_dist

build: web
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/glutton ./cmd/glutton

test:
	$(GO) test ./... -race

tidy:
	$(GO) mod tidy

sqlc:
	sqlc generate

run: build
	./bin/glutton
