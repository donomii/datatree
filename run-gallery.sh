#!/bin/sh
set -eu

# Open the directory from which this launcher was invoked, or the directory supplied by the caller.
directory="$(cd "${1:-$PWD}" && pwd)"
cd "$(dirname "$0")"
# Trusted portfolio source supplying the gallery stylesheet, ZIP reader and Three.js.
DATATREE_GALLERY_ASSETS="$HOME/Dropbox/newsimplewebsite/content"
DATATREE_ADDR=127.0.0.1:0
export DATATREE_GALLERY_ASSETS DATATREE_ADDR
go build -o datatree .
exec ./datatree "$directory"
