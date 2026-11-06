#!/bin/sh

# Installer smoke test: install into a throwaway prefix, then run the installed
# launcher outside the project directory.

PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$PROJECT_DIR" || exit 1

WORK="$(mktemp -d "${TMPDIR:-/tmp}/datatree-install-test.XXXXXX")" || exit 1

fail() {
    echo "FAIL: $1"
    exit 1
}

echo "Installing into throwaway prefix $WORK/prefix ..."
DATATREE_PREFIX="$WORK/prefix" ./install.sh > "$WORK/install.log" 2>&1 || fail "install.sh exited non-zero; see $WORK/install.log"

for installed in bin/datatree libexec/datatree; do
    if [ -f "$WORK/prefix/$installed" ]; then
        echo "OK: installed $installed"
    else
        fail "installer did not create $WORK/prefix/$installed"
    fi
done

if [ -L "$WORK/prefix/bin/datatree" ] && [ "$(readlink "$WORK/prefix/bin/datatree")" = "../libexec/datatree" ]; then
    echo "OK: bin/datatree links to the single release binary"
else
    fail "expected bin/datatree to link to ../libexec/datatree, got $(readlink "$WORK/prefix/bin/datatree")"
fi

REGULAR_FILE_COUNT=$(find "$WORK/prefix" -type f | wc -l | tr -d ' ')
if [ "$REGULAR_FILE_COUNT" = "1" ]; then
    echo "OK: installed prefix contains exactly one regular file"
else
    fail "expected exactly one regular file in $WORK/prefix, got $REGULAR_FILE_COUNT"
fi

mkdir "$WORK/empty" || fail "cannot create the empty working directory"
(cd "$WORK/empty" && "$WORK/prefix/bin/datatree" -test) || fail "the installed launcher self-test failed for prefix $WORK/prefix"

echo "Installer smoke test passed."
echo "Installer smoke-test files remain at $WORK for inspection."
