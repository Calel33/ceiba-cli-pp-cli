.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/ceiba-cli-pp-cli$(BIN_EXT) ./cmd/ceiba-cli-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/ceiba-cli-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/ceiba-cli-pp-mcp$(BIN_EXT) ./cmd/ceiba-cli-pp-mcp

install-mcp:
	go install ./cmd/ceiba-cli-pp-mcp

build-all: build build-mcp
