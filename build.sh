#!/bin/sh

# Use Homebrew Go if available, otherwise system go
GO_BIN="/opt/homebrew/bin/go"
if [ ! -f "$GO_BIN" ]; then
    GO_BIN="go"
fi

echo "Building Datatree..."
$GO_BIN build -o datatree .

echo "Build complete."
