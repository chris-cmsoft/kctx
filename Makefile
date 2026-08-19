BIN_DIR := bin

.PHONY: build install test fmt clean

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/kctx .

install:
	go install .

test:
	go test ./...

fmt:
	go fmt ./...

clean:
	rm -rf $(BIN_DIR)
