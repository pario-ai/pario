BINARY    := pario
PKG       := github.com/pario-ai/pario
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -ldflags "-X main.version=$(VERSION)"
GOFLAGS   ?=

.PHONY: build test lint run clean

build:
	go build $(GOFLAGS) $(LDFLAGS) -o bin/$(BINARY) ./cmd/pario

test:
	go test ./... -race -count=1

lint:
	golangci-lint run ./...

run: build
	./bin/$(BINARY)

clean:
	rm -rf bin/ dist/
