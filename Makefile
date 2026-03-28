# Makefile for secret-rotator
# Usage: make [target]
#   make build          - Build binary to ./bin/rotator
#   make test           - Run all tests
#   make test-verbose   - Run tests with verbose output
#   make lint           - Run go vet
#   make clean          - Remove build artifacts
#   make docker         - Build Docker image
#   make release-snapshot - Dry-run goreleaser release
#   make release        - Full goreleaser release

VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X github.com/giulio/secret-rotator/internal/cli.version=$(VERSION) -X github.com/giulio/secret-rotator/internal/cli.commit=$(COMMIT) -X github.com/giulio/secret-rotator/internal/cli.date=$(DATE)

.DEFAULT_GOAL := build

.PHONY: build test test-verbose lint clean docker release-snapshot release

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/rotator ./cmd/rotator

test:
	go test ./... -count=1

test-verbose:
	go test ./... -v -count=1

lint:
	go vet ./...

clean:
	rm -rf bin/

docker:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg DATE=$(DATE) -t secret-rotator:$(VERSION) .

release-snapshot:
	goreleaser release --snapshot --clean

release:
	goreleaser release --clean
