#!/usr/bin/env xsh
# Run from any directory; no arguments are required.
func checked {command ... |
    eval command
    if [eq [last-exit-code] 0] {
        id 0
    } else {
        exit [last-exit-code]
    }
}
cd $XSH_SCRIPT_DIR
checked go build -o datatree .
