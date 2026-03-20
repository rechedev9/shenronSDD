.PHONY: build install test lint fmt check clean

MODULE := github.com/rechedev9/shenronSDD/sdd-cli
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(MODULE)/internal/cli.version=$(VERSION)

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/sdd ./cmd/sdd

install: build
	cp bin/sdd ~/.local/bin/sdd

test:
	go test ./...

lint:
	golangci-lint run ./...

fmt:
	gofumpt -w .

check: fmt lint test

clean:
	rm -rf bin/
