.PHONY: build clean test lint run

BINARY := bin/wildcat
VERSION ?= dev
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildTime=$(BUILD_TIME)"

build:
	@mkdir -p bin
	go build $(LDFLAGS) -o $(BINARY) .

clean:
	rm -f $(BINARY)

test:
	go test -v ./...

lint:
	golangci-lint run

run: build
	./$(BINARY)
