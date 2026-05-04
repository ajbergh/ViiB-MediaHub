#!/usr/bin/env bash
# Build script for ViiB MediaHub (Wails Linux build)
#
# Builds the native Linux desktop application using Wails.
# Output: build/ViiB-MediaHub
#
# Supported architectures:
#   amd64  - x86-64 [default]
#   arm64  - ARM64 / AArch64
#
# Prerequisites:
#   - Go 1.22+ with CGO_ENABLED=1
#   - Node.js 20+ and npm
#   - Wails CLI: go install github.com/wailsapp/wails/v2/cmd/wails@latest
#   - CGO toolchain:
#       amd64: gcc (e.g., sudo apt install gcc)
#       arm64 cross-compile: gcc-aarch64-linux-gnu
#                            (sudo apt install gcc-aarch64-linux-gnu)
#   - WebKitGTK (for Wails WebView):
#       sudo apt install libgtk-3-dev libwebkit2gtk-4.0-dev
#   - System tray support:
#       sudo apt install libayatana-appindicator3-dev
#       (or: libappindicator3-dev on older distros)
#
# Usage:
#   ./scripts/build-wails-linux.sh
#   ./scripts/build-wails-linux.sh --arch arm64
#   ./scripts/build-wails-linux.sh --arch amd64 --debug
#   ./scripts/build-wails-linux.sh --arch amd64 --clean

set -euo pipefail

# ============================================================================
# Defaults
# ============================================================================

ARCH="amd64"
DEBUG=false
CLEAN=false
SKIP_FRONTEND=false

# ============================================================================
# Parse Arguments
# ============================================================================

while [[ $# -gt 0 ]]; do
    case "$1" in
        --arch)
            ARCH="$2"
            shift 2
            ;;
        --debug)
            DEBUG=true
            shift
            ;;
        --clean)
            CLEAN=true
            shift
            ;;
        --skip-frontend)
            SKIP_FRONTEND=true
            shift
            ;;
        *)
            echo "Unknown argument: $1"
            echo "Usage: $0 [--arch amd64|arm64] [--debug] [--clean] [--skip-frontend]"
            exit 1
            ;;
    esac
done

case "$ARCH" in
    amd64|arm64) ;;
    *)
        echo "❌ Invalid arch: $ARCH. Use amd64 or arm64."
        exit 1
        ;;
esac

# ============================================================================
# Preflight Checks
# ============================================================================

echo ""
echo "🐧 Building ViiB MediaHub for Linux ($ARCH)..."
echo ""
echo "🔍 Checking prerequisites..."

check_cmd() {
    if ! command -v "$1" &>/dev/null; then
        echo "❌ $1 not found. $2"
        exit 1
    fi
    local ver
    ver="$(${1} --version 2>&1 | head -1)"
    echo "  ✓ $1 — $ver"
}

check_cmd node "Install from https://nodejs.org/"
check_cmd npm  "Should be installed with Node.js."
check_cmd go   "Install from https://go.dev/dl/"

if ! command -v wails &>/dev/null; then
    echo "❌ Wails CLI not found."
    echo "  Install with: go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    exit 1
fi
echo "  ✓ wails $(wails version 2>&1 | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+')"

# Check CGO compiler
if [[ "$ARCH" == "arm64" && "$(uname -m)" != "aarch64" ]]; then
    # Cross-compile: need aarch64 gcc
    CC_CROSS="aarch64-linux-gnu-gcc"
    if ! command -v "$CC_CROSS" &>/dev/null; then
        echo "❌ Cross-compiler not found: $CC_CROSS"
        echo "  Install with: sudo apt install gcc-aarch64-linux-gnu"
        exit 1
    fi
    export CC="$CC_CROSS"
    echo "  ✓ Cross-compiler: $CC_CROSS"
else
    if ! command -v gcc &>/dev/null; then
        echo "❌ gcc not found. Install with: sudo apt install gcc"
        exit 1
    fi
    echo "  ✓ $(gcc --version | head -1)"
fi

# Check WebKitGTK (required for Wails WebView)
if ! pkg-config --exists webkit2gtk-4.0 2>/dev/null && \
   ! pkg-config --exists webkit2gtk-4.1 2>/dev/null; then
    echo "⚠  WebKitGTK not found."
    echo "   Install with: sudo apt install libgtk-3-dev libwebkit2gtk-4.0-dev"
    echo "   Continuing anyway (build may fail)..."
fi

# ============================================================================
# Setup Paths
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
WAILS_DIR="$PROJECT_ROOT/backend/cmd/wails"
DIST_SOURCE="$PROJECT_ROOT/dist"
DIST_DEST="$WAILS_DIR/frontend/dist"

echo ""
echo "📁 Project root: $PROJECT_ROOT"

if [[ ! -d "$WAILS_DIR" ]]; then
    echo "❌ Wails directory not found: $WAILS_DIR"
    exit 1
fi

# ============================================================================
# Clean (if requested)
# ============================================================================

if [[ "$CLEAN" == "true" ]]; then
    echo ""
    echo "🧹 Cleaning build artifacts..."
    rm -rf "$WAILS_DIR/build/bin" && echo "  Removed: $WAILS_DIR/build/bin"
    rm -rf "$DIST_DEST" && echo "  Removed: $DIST_DEST"
fi

# ============================================================================
# Step 1: Build Frontend
# ============================================================================

if [[ "$SKIP_FRONTEND" == "false" ]]; then
    echo ""
    echo "📦 [1/3] Building frontend..."

    pushd "$PROJECT_ROOT" > /dev/null

    if [[ ! -d "node_modules" ]]; then
        echo "  Installing npm dependencies..."
        npm ci
    fi

    npm run build
    echo "  ✓ Frontend built successfully"

    popd > /dev/null
else
    echo ""
    echo "⏭️  [1/3] Skipping frontend build (--skip-frontend)"
fi

# ============================================================================
# Step 2: Copy Frontend to Wails Location
# ============================================================================

echo ""
echo "📋 [2/3] Copying frontend to Wails..."

if [[ ! -d "$DIST_SOURCE" ]]; then
    echo "❌ Frontend dist not found: $DIST_SOURCE"
    echo "   Run 'npm run build' first or remove --skip-frontend flag"
    exit 1
fi

rm -rf "$DIST_DEST"
mkdir -p "$(dirname "$DIST_DEST")"
cp -r "$DIST_SOURCE" "$DIST_DEST"
echo "  ✓ Frontend copied to $DIST_DEST"

# ============================================================================
# Step 3: Build with Wails
# ============================================================================

echo ""
echo "🔨 [3/3] Building Wails application ($ARCH)..."

pushd "$WAILS_DIR" > /dev/null

export CGO_ENABLED=1

WAILS_ARGS=(-platform "linux/$ARCH")
[[ "$DEBUG" == "false" ]] && WAILS_ARGS+=(-ldflags "-s -w")

wails build "${WAILS_ARGS[@]}"

popd > /dev/null

# Copy output to /build
BUILD_DIR="$PROJECT_ROOT/build"
mkdir -p "$BUILD_DIR"

WAILS_BIN="$WAILS_DIR/build/bin/ViiB-MediaHub"
if [[ "$ARCH" == "arm64" ]]; then
    FINAL_BIN="$BUILD_DIR/ViiB-MediaHub-arm64"
else
    FINAL_BIN="$BUILD_DIR/ViiB-MediaHub"
fi

cp "$WAILS_BIN" "$FINAL_BIN"
echo "  ✓ Copied to $FINAL_BIN"

# ============================================================================
# Report Results
# ============================================================================

OUTPUT_BIN="$FINAL_BIN"

echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  ✅ BUILD COMPLETE"
echo "═══════════════════════════════════════════════════════════"

if [[ -f "$OUTPUT_BIN" ]]; then
    SIZE_MB=$(du -sm "$OUTPUT_BIN" | cut -f1)
    echo ""
    echo "  Output:  $OUTPUT_BIN"
    echo "  Size:    ~${SIZE_MB} MB"
    echo "  Arch:    $ARCH"
    echo "  Mode:    $([ "$DEBUG" == "true" ] && echo 'Debug' || echo 'Release')"
    echo ""
    echo "  To run:"
    echo "    $OUTPUT_BIN"
    echo "    $OUTPUT_BIN -debug    # With dev tools"
    echo ""
    echo "  To install desktop entry:"
    echo "    sudo cp $WAILS_DIR/build/linux/ViiB-MediaHub.desktop /usr/share/applications/"
    echo "    sudo cp $WAILS_DIR/build/appicon.png /usr/share/pixmaps/viib-mediahub.png"
else
    echo ""
    echo "  ⚠ Output not found at expected location"
    echo "    Expected: $OUTPUT_BIN"
fi

echo ""
