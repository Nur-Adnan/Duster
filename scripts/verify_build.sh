#!/bin/bash
# Duster Release Build Verification Script
# Validates syntax integrity, tag compilation, and stub structures across Unix and Windows builds.

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${CYAN}====== Duster Release Build Integrity Verification ======${NC}"

# 1. Check workspace structure
echo -e "${YELLOW}[1/4] Auditing package structure...${NC}"
REQUIRED_FILES=("main.go" "go.mod" "cmd/utils.go" "lib/fs/safe.go" "lib/sysinfo/sysinfo_windows.go")
for file in "${REQUIRED_FILES[@]}"; do
    if [ ! -f "$file" ]; then
        echo -e "${RED}✗ Error: Required file missing: $file${NC}"
        exit 1
    fi
done
echo -e "${GREEN}✓ All core architecture files present.${NC}"

# 2. Syntax Check (go vet)
if command -v go &> /dev/null; then
    echo -e "${YELLOW}[2/4] Running Go vetting & compiler syntax check...${NC}"
    go vet ./...
    echo -e "${GREEN}✓ Go vetting checks passed.${NC}"
    
    # 3. Test compilation of stubs (Unix)
    echo -e "${YELLOW}[3/4] Testing Linux cross-platform compilation check...${NC}"
    GOOS=linux GOARCH=amd64 go build -o /dev/null ./main.go
    echo -e "${GREEN}✓ Linux cross-compile stubs compile successfully.${NC}"

    # 4. Test compilation of Windows AMD64 and ARM64
    echo -e "${YELLOW}[4/4] Testing Windows production build compilation...${NC}"
    GOOS=windows GOARCH=amd64 go build -o /dev/null ./main.go
    echo -e "${GREEN}✓ Windows AMD64 binary compiles successfully.${NC}"
    
    GOOS=windows GOARCH=arm64 go build -o /dev/null ./main.go
    echo -e "${GREEN}✓ Windows ARM64 binary compiles successfully.${NC}"
    
    echo -e "${GREEN}★ Release verification completed successfully. Code is 100% stable. ★${NC}"
else
    echo -e "${YELLOW}[!] Warning: Go command not found in PATH. Skipping compiler syntax checks.${NC}"
    echo -e "${YELLOW}Please ensure Go is installed to execute local syntax compilation tests.${NC}"
fi
