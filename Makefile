SHELL := /bin/bash

all: help

.PHONY: help
help: Makefile
	@sed -n 's/^##//p' $< | awk 'BEGIN {FS = ":"}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: build test lint release-check release-snapshot

## build: Build the project for production
build:
	go build -o ./bin/jobsd ./cmd/jobsd

## test: Run all tests
test:
	go test ./...

## lint: Run linters
lint:
	go vet ./...

## release-check: Validate the GoReleaser configuration
release-check:
	go run github.com/goreleaser/goreleaser/v2@latest check

## release-snapshot: Build snapshot release artifacts locally
release-snapshot:
	go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean
