BINARY_NAME=ralph
VERSION?=dev
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

.PHONY: build test lint clean install

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/ralph

test:
	go test ./...

lint:
	golangci-lint run

clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/

install:
	go install $(LDFLAGS) ./cmd/ralph

# Cross-compilation for npm wrapper
dist: dist/ralph-darwin-arm64 dist/ralph-darwin-amd64 dist/ralph-linux-amd64 dist/ralph-linux-arm64

dist/ralph-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $@ ./cmd/ralph

dist/ralph-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $@ ./cmd/ralph

dist/ralph-linux-amd64:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $@ ./cmd/ralph

dist/ralph-linux-arm64:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $@ ./cmd/ralph
