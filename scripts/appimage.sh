#!/usr/bin/env bash
# Builds Mayoi for Linux on this machine and packages it as a single-file
# AppImage in the repository root, mayoi-<version>-<arch>.AppImage.
# Usage: scripts/appimage.sh
#
# It runs the same go tool mygo build the release workflow runs, then puts
# the app, the library it loads from its own folder (libghostty-vt), its
# icon and its entry in an AppDir, and runs appimagetool over it. The tool
# is $APPIMAGETOOL, else appimagetool.AppImage in the repository root, else
# appimagetool on PATH; without FUSE, run it with APPIMAGE_EXTRACT_AND_RUN=1.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="$(sed -n 's/^  "version": "\(.*\)",$/\1/p' "$ROOT/mygo.json")"
if [ -z "$VERSION" ]; then
    echo "appimage.sh: no version in mygo.json" >&2
    exit 1
fi

case "$(uname -m)" in
    x86_64 | amd64) GO_ARCH=amd64; APPIMAGE_ARCH=x86_64 ;;
    aarch64 | arm64) GO_ARCH=arm64; APPIMAGE_ARCH=aarch64 ;;
    *)
        echo "appimage.sh: no AppImage for $(uname -m)" >&2
        exit 1
        ;;
esac

TOOL="${APPIMAGETOOL:-}"
if [ -z "$TOOL" ]; then
    for CANDIDATE in "$ROOT/appimagetool.AppImage" "$ROOT/appimagetool"; do
        if [ -x "$CANDIDATE" ]; then
            TOOL="$CANDIDATE"
            break
        fi
    done
fi
if [ -z "$TOOL" ] && command -v appimagetool >/dev/null 2>&1; then
    TOOL="appimagetool"
fi
if [ -z "$TOOL" ]; then
    echo "appimage.sh: no appimagetool: set APPIMAGETOOL, put it in $ROOT," >&2
    echo "appimage.sh: or install it from https://github.com/AppImage/appimagetool/releases" >&2
    exit 1
fi

cd "$ROOT"
echo "==> go tool mygo build -platform linux/$GO_ARCH"
go tool mygo build -platform "linux/$GO_ARCH"

BUILD="$ROOT/build/linux-$GO_ARCH"
for FILE in mayoi libghostty-vt.so mayoi.png mayoi.desktop; do
    if [ ! -f "$BUILD/$FILE" ]; then
        echo "appimage.sh: the build left no $BUILD/$FILE" >&2
        exit 1
    fi
done

APPDIR="$(mktemp -d)"
trap 'rm -rf "$APPDIR"' EXIT
trap 'exit 1' HUP INT TERM
cp "$BUILD/mayoi" "$BUILD/libghostty-vt.so" "$BUILD/mayoi.png" "$BUILD/mayoi.desktop" "$APPDIR/"
# AppRun is what the AppImage runs; Mayoi loads libghostty-vt.so from the
# folder of its own executable, which the symlink keeps next to it.
ln -s mayoi "$APPDIR/AppRun"

OUT="$ROOT/mayoi-$VERSION-$APPIMAGE_ARCH.AppImage"
echo "==> appimagetool $OUT"
rm -f "$OUT"
ARCH="$APPIMAGE_ARCH" "$TOOL" --no-appstream "$APPDIR" "$OUT"
echo "==> $OUT"
