.PHONY: build test clean help

# Configuration
MODULE_ID := $(shell grep '"id"' module.json | cut -d'"' -f4)
BIN_DIR := bin/$(MODULE_ID)

help:
	@echo "Module Development Commands:"
	@echo "  make build    - Compiles and packages the module into $(BIN_DIR)"
	@echo "  make test     - Runs all unit tests"
	@echo "  make clean    - Removes build artifacts"

build:
	@echo "Building $(MODULE_ID)..."
	@mkdir -p $(BIN_DIR)/state/instances
	@CGO_ENABLED=1 go build -o $(BIN_DIR)/adapter ./cmd/adapter/main.go
	@cp module.json $(BIN_DIR)/bundle.json
	@echo "Done! Ready to copy $(BIN_DIR) to core/bundles/"

test:
	@CGO_ENABLED=1 go test -v ./...

clean:
	@rm -rf bin/
	@echo "Cleaned."
