#!/bin/sh

# Build and run Datatree. Datatree prints and opens its browser URL.
./build.sh

exec ./datatree "$@"
