SHELL := /bin/bash

all: help

.PHONY: help
help: Makefile
	@sed -n 's/^##//p' $< | awk 'BEGIN {FS = ":"}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: build test lint

## build: Build the project for production
build:
	go build ./cmd/jobsd

## test: Run all tests
test:
	go test ./...

## lint: Run linters
lint:
	go vet ./...
