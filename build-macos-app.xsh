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
checked env CGO_ENABLED=0 go build -o datatree .
checked mkdir -p Datatree.app/Contents/MacOS Datatree.app/Contents/Resources
checked install -m 755 datatree Datatree.app/Contents/MacOS/datatree-bin
checked clang -O2 -fobjc-arc -framework Cocoa -o Datatree.app/Contents/MacOS/Datatree macos/DatatreeLauncher.m
checked go run ./macos/makeicon -out Datatree.app/Contents/Resources/AppIcon.icns
checked cp macos/Info.plist Datatree.app/Contents/Info.plist
checked plutil -lint Datatree.app/Contents/Info.plist
checked codesign --force --deep --sign - Datatree.app
