#!/usr/bin/env bash
# Build script for ViiB MediaHub (Wails macOS build)
#
# Builds the native macOS desktop application using Wails.
# Output: build/ViiB-MediaHub.app
#
# Supported architectures:
#   arm64    - Apple Silicon (M1/M2/M3/M4) [default]
#   amd64    - Intel x86-64
#   universal - Fat binary (both arm64 + amd64 via lipo)
#
# Prerequisites:
#   - Go 1.22+ with CGO_ENABLED=1
#   - Node.js 20+ and npm
#   - Wails CLI: go install github.com/wailsapp/wails/v2/cmd/wails@latest
#   - Xcode Command Line Tools: xcode-select --install
#   - For universal builds: both architectures require lipo (included with Xcode)
#
# Usage:
#   ./scripts/build-wails-macos.sh
#   ./scripts/build-wails-macos.sh --arch amd64
#   ./scripts/build-wails-macos.sh --arch universal
#   ./scripts/build-wails-macos.sh --arch arm64 --debug
#   ./scripts/build-wails-macos.sh --arch arm64 --clean

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
            echo "Usage: $0 [--arch amd64|arm64|universal] [--debug] [--clean] [--skip-frontend]"
            exit 1
            ;;
    esac
done

case "$ARCH" in
    amd64|arm64|universal) ;;
    *)
        echo "❌ Invalid arch: $ARCH. Use amd64, arm64, or universal."
        exit 1
        ;;
esac

# ============================================================================
# Preflight Checks
# ============================================================================

echo ""
echo "🍎 Building ViiB MediaHub for macOS ($ARCH)..."
echo ""
echo "🔍 Checking prerequisites..."

check_cmd() {
    if ! command -v "$1" &>/dev/null; then
        echo "❌ $1 not found. $2"
        exit 1
    fi
    echo "  ✓ $1 $(${1} --version 2>&1 | head -1)"
}

check_cmd node  "Install from https://nodejs.org/"
check_cmd npm   "Should be installed with Node.js."
check_cmd go    "Install from https://go.dev/dl/"

if ! command -v wails &>/dev/null; then
    echo "❌ Wails CLI not found."
    echo "  Install with: go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    exit 1
fi
echo "  ✓ wails $(wails version 2>&1 | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+')"

# Check Xcode CLT (needed for CGO)
if ! xcode-select -p &>/dev/null; then
    echo "❌ Xcode Command Line Tools not found."
    echo "  Install with: xcode-select --install"
    exit 1
fi
echo "  ✓ Xcode CLT: $(xcode-select -p)"

# For universal builds, lipo is required (bundled with Xcode)
if [[ "$ARCH" == "universal" ]]; then
    if ! command -v lipo &>/dev/null; then
        echo "❌ lipo not found. Install Xcode Command Line Tools."
        exit 1
    fi
    echo "  ✓ lipo available"
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

# Universal builds: build amd64 + arm64 then lipo-combine
if [[ "$ARCH" == "universal" ]]; then
    echo "  Building amd64 slice..."
    WAILS_ARGS=(-platform darwin/amd64)
    [[ "$DEBUG" == "false" ]] && WAILS_ARGS+=(-ldflags "-s -w")
    wails build "${WAILS_ARGS[@]}"
    mv build/bin/ViiB-MediaHub.app build/bin/ViiB-MediaHub-amd64.app

    echo "  Building arm64 slice..."
    WAILS_ARGS=(-platform darwin/arm64)
    [[ "$DEBUG" == "false" ]] && WAILS_ARGS+=(-ldflags "-s -w")
    wails build "${WAILS_ARGS[@]}"
    mv build/bin/ViiB-MediaHub.app build/bin/ViiB-MediaHub-arm64.app

    echo "  Combining with lipo..."
    cp -R build/bin/ViiB-MediaHub-arm64.app build/bin/ViiB-MediaHub.app
    lipo -create \
        "build/bin/ViiB-MediaHub-amd64.app/Contents/MacOS/ViiB-MediaHub" \
        "build/bin/ViiB-MediaHub-arm64.app/Contents/MacOS/ViiB-MediaHub" \
        -output "build/bin/ViiB-MediaHub.app/Contents/MacOS/ViiB-MediaHub"
    rm -rf build/bin/ViiB-MediaHub-amd64.app build/bin/ViiB-MediaHub-arm64.app
    echo "  ✓ Universal binary created"
else
    WAILS_ARGS=(-platform "darwin/$ARCH")
    [[ "$DEBUG" == "false" ]] && WAILS_ARGS+=(-ldflags "-s -w")
    wails build "${WAILS_ARGS[@]}"
fi

popd > /dev/null

# Copy output to /build
BUILD_DIR="$PROJECT_ROOT/build"
mkdir -p "$BUILD_DIR"
cp -R "$WAILS_DIR/build/bin/ViiB-MediaHub.app" "$BUILD_DIR/ViiB-MediaHub.app"
echo "  ✓ Copied to $BUILD_DIR/ViiB-MediaHub.app"

# ============================================================================
# Report Results
# ============================================================================

OUTPUT_APP="$BUILD_DIR/ViiB-MediaHub.app"
OUTPUT_BIN="$OUTPUT_APP/Contents/MacOS/ViiB-MediaHub"

echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  ✅ BUILD COMPLETE"
echo "═══════════════════════════════════════════════════════════"

if [[ -f "$OUTPUT_BIN" ]]; then
    SIZE_MB=$(du -sm "$OUTPUT_APP" | cut -f1)
    ARCHS=$(lipo -archs "$OUTPUT_BIN" 2>/dev/null || echo "unknown")
    echo ""
    echo "  Output:  $OUTPUT_APP"
    echo "  Size:    ~${SIZE_MB} MB"
    echo "  Archs:   $ARCHS"
    echo "  Mode:    $([ "$DEBUG" == "true" ] && echo 'Debug' || echo 'Release')"
    echo ""
    echo "  To run:"
    echo "    open $OUTPUT_APP"
    echo "    $OUTPUT_BIN -debug    # With dev tools"
else
    echo ""
    echo "  ⚠ Output not found at expected location"
    echo "    Expected: $OUTPUT_APP"
fi

echo ""
