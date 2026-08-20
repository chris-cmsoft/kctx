BIN_DIR := bin
VERSION ?= $(shell git describe --tags --dirty=+dirty --always 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build dist install test fmt clean

build:
	@mkdir -p $(BIN_DIR)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/kctx .

# Same targets the release workflow publishes.
dist:
	@mkdir -p dist
	@for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64; do \
		goos="$${target%/*}"; goarch="$${target#*/}"; \
		echo "building $$goos/$$goarch"; \
		CGO_ENABLED=0 GOOS="$$goos" GOARCH="$$goarch" go build \
			-trimpath -ldflags "$(LDFLAGS)" -o "dist/kctx_$${goos}_$${goarch}" . || exit 1; \
	done

install:
	go install -trimpath -ldflags "$(LDFLAGS)" .

test:
	go test ./...

fmt:
	go fmt ./...

clean:
	rm -rf $(BIN_DIR) dist
