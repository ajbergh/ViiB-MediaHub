#!/usr/bin/env bash
# Build script for ViiB MediaHub (Web browser build - macOS)
#
# Builds the web-embedded binary that serves a local HTTP server and opens
# the default browser. Does NOT require Wails or WebView2.
# Output: build/ViiB-MediaHub (or ViiB-MediaHub-arm64 / ViiB-MediaHub-amd64)
#
# Supported architectures:
#   arm64    - Apple Silicon (M1/M2/M3/M4) [default]
#   amd64    - Intel x86-64
#
# Prerequisites:
#   - Go 1.22+ with CGO_ENABLED=1
#   - Node.js 20+ and npm
#   - Xcode Command Line Tools: xcode-select --install
#
# Usage:
#   ./scripts/build-web-macos.sh
#   ./scripts/build-web-macos.sh --arch amd64
#   ./scripts/build-web-macos.sh --arch arm64 --debug
#   ./scripts/build-web-macos.sh --clean

set -euo pipefail

# ============================================================================
# Defaults
# ============================================================================

ARCH="arm64"
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
echo "🍎 Building ViiB MediaHub (web browser mode) for macOS ($ARCH)..."
echo ""
echo "🔍 Checking prerequisites..."

check_cmd() {
    if ! command -v "$1" &>/dev/null; then
        echo "❌ $1 not found. $2"
        exit 1
    fi
    echo "  ✓ $1 $("$1" --version 2>&1 | head -1)"
}

check_cmd node "Install from https://nodejs.org/"
check_cmd npm  "Should be installed with Node.js."
check_cmd go   "Install from https://go.dev/dl/"

if ! xcode-select -p &>/dev/null; then
    echo "❌ Xcode Command Line Tools not found."
    echo "  Install with: xcode-select --install"
    exit 1
fi
echo "  ✓ Xcode CLT: $(xcode-select -p)"

# ============================================================================
# Setup Paths
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
VIIB_DIR="$PROJECT_ROOT/backend/cmd/viib"
DIST_SOURCE="$PROJECT_ROOT/dist"
DIST_DEST="$VIIB_DIR/dist"
BUILD_DIR="$PROJECT_ROOT/build"

echo ""
echo "📁 Project root: $PROJECT_ROOT"

if [[ ! -d "$VIIB_DIR" ]]; then
    echo "❌ viib cmd directory not found: $VIIB_DIR"
    exit 1
fi

# ============================================================================
# Clean (if requested)
# ============================================================================

if [[ "$CLEAN" == "true" ]]; then
    echo ""
    echo "🧹 Cleaning build artifacts..."
    rm -rf "$BUILD_DIR" && echo "  Removed: $BUILD_DIR"
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
# Step 2: Copy Frontend to backend embed location
# ============================================================================

echo ""
echo "📋 [2/3] Copying frontend to backend..."

if [[ ! -d "$DIST_SOURCE" ]]; then
    echo "❌ Frontend dist not found: $DIST_SOURCE"
    echo "   Run 'npm run build' first or remove --skip-frontend flag"
    exit 1
fi

rm -rf "$DIST_DEST"
cp -r "$DIST_SOURCE" "$DIST_DEST"
echo "  ✓ Frontend copied to $DIST_DEST"

# ============================================================================
# Step 3: Build Go backend
# ============================================================================

echo ""
echo "🔨 [3/3] Building Go backend ($ARCH)..."

mkdir -p "$BUILD_DIR"

pushd "$PROJECT_ROOT/backend" > /dev/null

go mod tidy

export CGO_ENABLED=1
export GOOS=darwin
export GOARCH="$ARCH"

OUTPUT_BIN="$BUILD_DIR/ViiB-MediaHub"
LDFLAGS="-s -w"
[[ "$DEBUG" == "true" ]] && LDFLAGS=""

go build -ldflags="$LDFLAGS" -o "$OUTPUT_BIN" ./cmd/viib
if [[ $? -ne 0 ]]; then
    echo "❌ Go build failed"
    exit 1
fi

popd > /dev/null

# ============================================================================
# Report Results
# ============================================================================

echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  ✅ BUILD COMPLETE (macOS web browser build)"
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
    echo "    $OUTPUT_BIN -no-browser -port 8080    # Headless server"
else
    echo ""
    echo "  ⚠ Output not found at expected location"
    echo "    Expected: $OUTPUT_BIN"
fi

echo ""
