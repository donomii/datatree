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
# Install the executable into the user local bin directory.
checked xsh build.xsh
checked mkdir -p "$HOME/.local/bin"
checked install -m 755 datatree "$HOME/.local/bin/datatree"
