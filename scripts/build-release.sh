#!/bin/bash
# ╔══════════════════════════════════════════════════════════════════════╗
# ║        Duster — Production Release Build Script                    ║
# ║        Generates portable .exe + installer-ready artifacts         ║
# ╚══════════════════════════════════════════════════════════════════════╝
#
# Usage:
#   ./scripts/build-release.sh              # Build all artifacts
#   ./scripts/build-release.sh --portable   # Build portable .exe only
#   ./scripts/build-release.sh --installer  # Build installer-ready artifacts
#   ./scripts/build-release.sh --version 1.0.2  # Override version
#
# Requirements:
#   - Go 1.25.x or later
#   - goversioninfo (optional, for Windows PE metadata)
#   - Inno Setup 6.x (optional, Windows-only, for .exe installer)

set -euo pipefail

# ── Color Codes ──────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# ── Configuration ────────────────────────────────────────────────────
VERSION="${DUSTER_VERSION:-1.0.1}"
BUILD_DATE=$(date +%Y-%m-%d)
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "dev")
DIST_DIR="dist"
BUILD_PORTABLE=true
BUILD_INSTALLER=true

# ── Parse Arguments ──────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case $1 in
        --portable)
            BUILD_INSTALLER=false
            shift
            ;;
        --installer)
            BUILD_PORTABLE=false
            shift
            ;;
        --version)
            VERSION="$2"
            shift 2
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            exit 1
            ;;
    esac
done

# ── LDFLAGS ──────────────────────────────────────────────────────────
LDFLAGS="-s -w -X main.Version=${VERSION} -X main.BuildDate=${BUILD_DATE} -X main.Commit=${COMMIT}"

echo ""
echo -e "${CYAN}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║           Duster Production Release Builder                ║${NC}"
echo -e "${CYAN}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "  ${BOLD}Version:${NC}     ${VERSION}"
echo -e "  ${BOLD}Build Date:${NC}  ${BUILD_DATE}"
echo -e "  ${BOLD}Commit:${NC}      ${COMMIT}"
echo ""

# ── Step 1: Validate ─────────────────────────────────────────────────
echo -e "${YELLOW}[1/6] Validating project integrity...${NC}"

if ! command -v go &> /dev/null; then
    echo -e "${RED}✗ Go toolchain not found in PATH.${NC}"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}')
echo -e "  Go version: ${GREEN}${GO_VERSION}${NC}"

go mod verify
echo -e "${GREEN}✓ Dependencies verified.${NC}"

# ── Step 2: Test ─────────────────────────────────────────────────────
echo ""
echo -e "${YELLOW}[2/6] Running test suite...${NC}"

CGO_ENABLED=0 go test -count=1 ./... 2>&1 | while IFS= read -r line; do
    if echo "$line" | grep -q "^ok"; then
        echo -e "  ${GREEN}✓${NC} $line"
    elif echo "$line" | grep -q "FAIL"; then
        echo -e "  ${RED}✗${NC} $line"
    fi
done

# Check test exit code
CGO_ENABLED=0 go test -count=1 ./... > /dev/null 2>&1
echo -e "${GREEN}✓ All tests passed.${NC}"

# ── Step 3: Vet ──────────────────────────────────────────────────────
echo ""
echo -e "${YELLOW}[3/6] Running static analysis...${NC}"
go vet ./...
echo -e "${GREEN}✓ Go vet passed.${NC}"

# ── Step 4: Clean & Prepare ──────────────────────────────────────────
echo ""
echo -e "${YELLOW}[4/6] Preparing build directory...${NC}"
rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}/portable" "${DIST_DIR}/installer"
echo -e "${GREEN}✓ Build directory ready: ${DIST_DIR}/${NC}"

# ── Step 5: Compile ─────────────────────────────────────────────────
echo ""
echo -e "${YELLOW}[5/6] Compiling production binaries...${NC}"

# Windows AMD64 (primary target)
echo -e "  Building ${BOLD}Windows AMD64${NC}..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
    go build -trimpath -ldflags="${LDFLAGS}" \
    -o "${DIST_DIR}/duster-windows-amd64.exe" .
AMD64_SIZE=$(ls -lh "${DIST_DIR}/duster-windows-amd64.exe" | awk '{print $5}')
echo -e "  ${GREEN}✓${NC} duster-windows-amd64.exe  (${AMD64_SIZE})"

# Windows ARM64
echo -e "  Building ${BOLD}Windows ARM64${NC}..."
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 \
    go build -trimpath -ldflags="${LDFLAGS}" \
    -o "${DIST_DIR}/duster-windows-arm64.exe" .
ARM64_SIZE=$(ls -lh "${DIST_DIR}/duster-windows-arm64.exe" | awk '{print $5}')
echo -e "  ${GREEN}✓${NC} duster-windows-arm64.exe  (${ARM64_SIZE})"

echo -e "${GREEN}✓ All binaries compiled successfully.${NC}"

# ── Step 6: Package ─────────────────────────────────────────────────
echo ""
echo -e "${YELLOW}[6/6] Packaging release artifacts...${NC}"

if [ "$BUILD_PORTABLE" = true ]; then
    echo -e "  Creating ${BOLD}portable archives${NC}..."

    # AMD64 portable ZIP
    PORTABLE_AMD64="${DIST_DIR}/portable/Duster-${VERSION}-Portable-x64"
    mkdir -p "${PORTABLE_AMD64}"
    cp "${DIST_DIR}/duster-windows-amd64.exe" "${PORTABLE_AMD64}/du.exe"
    cp README.md LICENSE SECURITY.md "${PORTABLE_AMD64}/" 2>/dev/null || true

    # Create portable launcher script
    cat > "${PORTABLE_AMD64}/Launch-Duster.bat" << 'BATCH'
@echo off
title Duster — Windows System Cleaner
echo.
echo   ╔══════════════════════════════════════╗
echo   ║     Duster System Cleaner            ║
echo   ║     Type 'du --help' to begin        ║
echo   ╚══════════════════════════════════════╝
echo.
"%~dp0du.exe" --version
echo.
cmd /K
BATCH

    (cd "${DIST_DIR}/portable" && zip -r "../Duster-${VERSION}-Portable-x64.zip" "Duster-${VERSION}-Portable-x64/")
    echo -e "  ${GREEN}✓${NC} Duster-${VERSION}-Portable-x64.zip"

    # ARM64 portable ZIP
    PORTABLE_ARM64="${DIST_DIR}/portable/Duster-${VERSION}-Portable-arm64"
    mkdir -p "${PORTABLE_ARM64}"
    cp "${DIST_DIR}/duster-windows-arm64.exe" "${PORTABLE_ARM64}/du.exe"
    cp README.md LICENSE SECURITY.md "${PORTABLE_ARM64}/" 2>/dev/null || true
    cp "${PORTABLE_AMD64}/Launch-Duster.bat" "${PORTABLE_ARM64}/"

    (cd "${DIST_DIR}/portable" && zip -r "../Duster-${VERSION}-Portable-arm64.zip" "Duster-${VERSION}-Portable-arm64/")
    echo -e "  ${GREEN}✓${NC} Duster-${VERSION}-Portable-arm64.zip"
fi

if [ "$BUILD_INSTALLER" = true ]; then
    echo -e "  Preparing ${BOLD}installer artifacts${NC}..."

    # Copy installer-ready binary for Inno Setup
    cp "${DIST_DIR}/duster-windows-amd64.exe" "${DIST_DIR}/installer/"
    echo -e "  ${GREEN}✓${NC} Installer binary staged"
    echo -e "  ${YELLOW}→${NC} Run Inno Setup on ${BOLD}installer/duster-setup.iss${NC} to build the .exe installer"
fi

# ── Generate Checksums ───────────────────────────────────────────────
echo ""
echo -e "${YELLOW}Generating SHA-256 checksums...${NC}"
(
    cd "${DIST_DIR}"
    shasum -a 256 duster-windows-amd64.exe duster-windows-arm64.exe \
        Duster-*-Portable-*.zip 2>/dev/null \
        > checksums-sha256.txt
)
echo -e "${GREEN}✓ Checksums written to ${DIST_DIR}/checksums-sha256.txt${NC}"

# ── Summary ──────────────────────────────────────────────────────────
echo ""
echo -e "${CYAN}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║                  Build Summary                             ║${NC}"
echo -e "${CYAN}╠══════════════════════════════════════════════════════════════╣${NC}"
echo -e "${CYAN}║${NC}  Version:    ${BOLD}${VERSION}${NC}"
echo -e "${CYAN}║${NC}  Commit:     ${COMMIT}"
echo -e "${CYAN}║${NC}  Date:       ${BUILD_DATE}"
echo -e "${CYAN}║${NC}"
echo -e "${CYAN}║${NC}  ${BOLD}Artifacts:${NC}"

for f in "${DIST_DIR}"/duster-windows-*.exe "${DIST_DIR}"/Duster-*-Portable-*.zip; do
    if [ -f "$f" ]; then
        SIZE=$(ls -lh "$f" | awk '{print $5}')
        NAME=$(basename "$f")
        echo -e "${CYAN}║${NC}    ${GREEN}✓${NC} ${NAME}  (${SIZE})"
    fi
done

echo -e "${CYAN}║${NC}"
echo -e "${CYAN}║${NC}  Checksums:  ${DIST_DIR}/checksums-sha256.txt"
echo -e "${CYAN}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""

# ── Next Steps ───────────────────────────────────────────────────────
echo -e "${BOLD}Next Steps:${NC}"
echo -e "  1. ${YELLOW}Test portable build:${NC} Unzip and run du.exe on a Windows machine"
echo -e "  2. ${YELLOW}Build installer:${NC} Open installer/duster-setup.iss in Inno Setup Compiler"
echo -e "  3. ${YELLOW}Code sign:${NC} Run scripts/sign.ps1 on a Windows machine with a certificate"
echo -e "  4. ${YELLOW}Publish:${NC} git tag v${VERSION} && git push --tags"
echo ""
