#!/bin/bash
set -e

# Extract Module ID from module.json
MODULE_ID=$(grep '"id"' module.json | cut -d'"' -f4)
OUT_DIR="bin/$MODULE_ID"

echo "========================================"
echo "Building Module: $MODULE_ID"
echo "========================================"

# 1. Prepare directory
rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR/state/instances"

# 2. Build Binary (requires CGO for SQLite)
echo "Compiling Go binary..."
CGO_ENABLED=1 go build -o "$OUT_DIR/adapter" ./cmd/adapter/main.go

# 3. Build UI Bundle
if [ -d "ui" ]; then
    echo "Compiling UI bundle..."
    cd ui && npm install && npm run build && cd ..
    mkdir -p "$OUT_DIR/ui"
    cp -r ui/dist/* "$OUT_DIR/ui/"
fi

# 4. Copy Metadata
cp module.json "$OUT_DIR/bundle.json"

echo "----------------------------------------"
echo "Build Successful!"
echo "Package: $OUT_DIR"
echo "Contents:"
ls -F "$OUT_DIR"
echo "----------------------------------------"
echo "To Install: cp -r $OUT_DIR <core_path>/bundles/"
echo "========================================"
