#!/bin/sh

set -eu

# Build everything
./build.sh

# Install the complete runtime tree under lib and a launcher under bin.
# DATATREE_PREFIX overrides the $HOME/.local default.
PREFIX="${DATATREE_PREFIX:-${HOME:?Cannot install Datatree: HOME is empty and DATATREE_PREFIX is not set}/.local}"
INSTALL_BIN="$PREFIX/bin"
INSTALL_LIBEXEC="$PREFIX/libexec"

echo ""
echo "Installing Datatree to $INSTALL_LIBEXEC/datatree ..."

mkdir -p "$INSTALL_BIN"
mkdir -p "$INSTALL_LIBEXEC"

install_executable() {
    source_file="$1"
    destination_file="$2"
    cp "$source_file" "$destination_file.new"
    chmod 755 "$destination_file.new"
    mv -f "$destination_file.new" "$destination_file"
}

install_executable datatree "$INSTALL_LIBEXEC/datatree"

ln -sfn ../libexec/datatree "$INSTALL_BIN/datatree.new"
mv -f "$INSTALL_BIN/datatree.new" "$INSTALL_BIN/datatree"

echo "Installed single binary to $INSTALL_LIBEXEC/datatree"
echo "Installed launcher symlink to $INSTALL_BIN/datatree"
echo ""
echo "Run with: $INSTALL_BIN/datatree"
