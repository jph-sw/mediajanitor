BINARY   := mediajanitor
PKG      := github.com/jph-sw/mediajanitor/cmd/mediajanitor
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -ldflags="-s -w -X main.version=$(VERSION)"
GOFLAGS  := CGO_ENABLED=0

.PHONY: build install test lint migrate release clean

build:
	$(GOFLAGS) go build $(LDFLAGS) -o $(BINARY) $(PKG)

install:
	$(GOFLAGS) go install $(LDFLAGS) $(PKG)

test:
	go test ./...

lint:
	golangci-lint run ./...

migrate:
	go run ./internal/store/migrations/run/main.go

release:
	goreleaser release --clean

snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -f $(BINARY)

# Generate sqlc code from queries
sqlc:
	sqlc generate

# Generate man pages
man:
	go run ./internal/cli/gen/man/main.go

# Generate shell completions
completions:
	./$(BINARY) completion bash  > completions/$(BINARY).bash
	./$(BINARY) completion zsh   > completions/_$(BINARY)
	./$(BINARY) completion fish  > completions/$(BINARY).fish
