.PHONY: build test lint run clean demo-gifs

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

# Renders docs/assets/demo/*.gif from their VHS tapes (settings.tape is
# shared, not rendered). Needs vhs: run inside `nix develop`, or see
# https://github.com/charmbracelet/vhs. Not run in CI.
demo-gifs: build
	@command -v vhs >/dev/null || { echo "vhs not found: run 'nix develop' or install charmbracelet/vhs"; exit 1; }
	@for tape in docs/assets/demo/*.tape; do \
		case $$tape in */settings.tape) continue;; esac; \
		echo "vhs $$tape"; vhs $$tape || exit 1; \
	done
