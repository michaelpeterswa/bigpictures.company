.DEFAULT_GOAL := build

GO        ?= go
PKG       := github.com/michaelpeterswa/bigpictures.company
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE      ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS   := -s -w \
             -X $(PKG)/internal/version.Version=$(VERSION) \
             -X $(PKG)/internal/version.Commit=$(COMMIT) \
             -X $(PKG)/internal/version.Date=$(DATE)

DIST := dist
BIN  := $(DIST)/pano

.PHONY: build
build: $(BIN)

$(BIN): $(shell find . -name '*.go' -not -path './web/*' 2>/dev/null)
	@mkdir -p $(DIST)
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/pano

.PHONY: test
test:
	$(GO) test -race ./...

.PHONY: lint
lint:
	golangci-lint run

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: clean
clean:
	rm -rf $(DIST) tmp/

.PHONY: migrate-up
migrate-up: $(BIN)
	$(BIN) migrate up

.PHONY: migrate-down
migrate-down: $(BIN)
	$(BIN) migrate down

.PHONY: web-dev
web-dev:
	cd web && pnpm dev

.PHONY: web-install
web-install:
	cd web && pnpm install --frozen-lockfile

.PHONY: web-lint
web-lint:
	cd web && pnpm lint

.PHONY: web-typecheck
web-typecheck:
	cd web && pnpm tsc --noEmit

.PHONY: web-build
web-build:
	cd web && pnpm build

.PHONY: ci
ci: lint test web-lint web-typecheck web-build
