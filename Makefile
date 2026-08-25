.PHONY: build test lint run clean

BINARY=slk
BUILD_DIR=bin
# tools/go.sh execs native go on unmanaged machines and runs go in docker on
# Santa hosts, where bare `go test`/`go run` binaries get SIGKILLed.
GO=tools/go.sh

build:
	$(GO) build -ldflags="-s -w" -trimpath -o $(BUILD_DIR)/$(BINARY) ./cmd/slk

test:
	$(GO) test ./... -v -race

# golangci-lint version matches .github/workflows/ci.yml
lint:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...

run: build
	./$(BUILD_DIR)/$(BINARY)

clean:
	rm -rf $(BUILD_DIR)
