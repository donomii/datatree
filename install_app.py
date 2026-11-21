#!/usr/bin/env python3
"""Build Datatree.app and install it into /Applications, or the directory given as the one argument."""
import plistlib
import shutil
import subprocess
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent
APP_NAME = "Datatree.app"
DEFAULT_DIRECTORY = Path("/Applications")
LSREGISTER = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"


def run(argv):
    print("+", " ".join(argv), flush=True)
    subprocess.run(argv, check=True, cwd=REPO)


def output(argv):
    return subprocess.run(argv, check=True, cwd=REPO, capture_output=True, text=True).stdout.strip()


def go_binary():
    homebrew_go = Path("/opt/homebrew/bin/go")
    if homebrew_go.is_file():
        return str(homebrew_go)
    else:
        return "go"


def info_plist():
    return {
        "CFBundleDevelopmentRegion": "en",
        "CFBundleDisplayName": "Datatree",
        "CFBundleExecutable": "Datatree",
        "CFBundleIdentifier": "com.donomii.datatree",
        "CFBundleInfoDictionaryVersion": "6.0",
        "CFBundleName": "Datatree",
        "CFBundlePackageType": "APPL",
        "CFBundleShortVersionString": "1.0",
        "CFBundleVersion": output(["git", "rev-parse", "--short", "HEAD"]),
        "LSApplicationCategoryType": "public.app-category.developer-tools",
        "LSMinimumSystemVersion": "13.0",
        "NSHighResolutionCapable": True,
        "NSPrincipalClass": "NSApplication",
    }


def build_bundle(bundle):
    contents = bundle / "Contents"
    macos = contents / "MacOS"
    resources = contents / "Resources"
    macos.mkdir(parents=True)
    resources.mkdir()
    run([go_binary(), "build", "-o", str(resources / "datatree"), "."])
    run(["swiftc", "-O", "-swift-version", "5", "-o", str(macos / "Datatree"), str(REPO / "app" / "Datatree.swift")])
    with open(contents / "Info.plist", "wb") as plist_file:
        plistlib.dump(info_plist(), plist_file)
    run(["codesign", "--force", "--sign", "-", str(resources / "datatree")])
    run(["codesign", "--force", "--sign", "-", str(bundle)])


def remove_tree(path):
    if path.exists():
        shutil.rmtree(path)
    else:
        pass


def install(staged, destination):
    previous = destination.with_name(destination.name + ".old")
    remove_tree(previous)
    if destination.exists():
        destination.rename(previous)
    else:
        pass
    staged.rename(destination)
    remove_tree(previous)
    run([LSREGISTER, "-f", str(destination)])


def main():
    if len(sys.argv) > 2:
        sys.exit(f"cannot install Datatree.app: expected at most one argument (the install directory, default {DEFAULT_DIRECTORY}), got {len(sys.argv) - 1}: {sys.argv[1:]}")
    elif len(sys.argv) == 2:
        directory = Path(sys.argv[1]).resolve()
    else:
        directory = DEFAULT_DIRECTORY
    destination = directory / APP_NAME
    staged = directory / (APP_NAME + ".new")
    remove_tree(staged)
    build_bundle(staged)
    install(staged, destination)
    print(f"Installed {destination}")


if __name__ == "__main__":
    main()
