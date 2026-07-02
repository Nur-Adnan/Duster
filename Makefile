# ╔══════════════════════════════════════════════════════════════════════╗
# ║             Duster — Production Build System                       ║
# ║             Windows-native Deep Cleaner & Optimizer                ║
# ╚══════════════════════════════════════════════════════════════════════╝

BINARY_NAME = du.exe
VERSION     ?= 1.0.2
BUILD_DATE  = $(shell date +%Y-%m-%d)
COMMIT      = $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
DIST_DIR    = dist

# Go build flags
GO          = go
GOFLAGS     = -trimpath
LDFLAGS     = -ldflags="-s -w \
	-X main.Version=$(VERSION) \
	-X main.BuildDate=$(BUILD_DATE) \
	-X main.Commit=$(COMMIT)"

# Environment
export CGO_ENABLED = 0

.PHONY: all build build-amd64 build-arm64 build-all resources test vet lint \
        clean release portable installer verify help hooks

# ── Default target ───────────────────────────────────────────────────
all: test build

# ── Help ─────────────────────────────────────────────────────────────
help:
	@echo ""
	@echo "  Duster Build System — v$(VERSION)"
	@echo ""
	@echo "  Usage: make [target]"
	@echo ""
	@echo "  Build Targets:"
	@echo "    build          Build Windows AMD64 binary (default)"
	@echo "    build-amd64    Build Windows AMD64 binary to dist/"
	@echo "    build-arm64    Build Windows ARM64 binary to dist/"
	@echo "    build-all      Build both architectures"
	@echo "    resources      Generate Windows PE version resources (.syso)"
	@echo ""
	@echo "  Release Targets:"
	@echo "    release        Full production release (test + build + package)"
	@echo "    portable       Build portable ZIP archives"
	@echo "    installer      Stage binaries for Inno Setup installer"
	@echo ""
	@echo "  Quality Targets:"
	@echo "    test           Run full test suite"
	@echo "    vet            Run Go vet static analysis"
	@echo "    lint           Run golangci-lint (if installed)"
	@echo "    verify         Run build verification script"
	@echo ""
	@echo "  Maintenance:"
	@echo "    clean          Remove build artifacts"
	@echo "    hooks          Install the repo's git pre-commit hooks"
	@echo ""

# ── Build ────────────────────────────────────────────────────────────
# Generate Windows PE resources (VERSIONINFO, icon, longPathAware manifest)
# from versioninfo.json. Optional locally: builds proceed without them.
# The .syso filenames carry GOOS/GOARCH suffixes, so both can coexist and
# the Go linker picks the matching one per target architecture.
resources:
	@if command -v goversioninfo > /dev/null 2>&1; then \
		GOARCH=amd64 goversioninfo -o resource_windows_amd64.syso; \
		GOARCH=arm64 goversioninfo -o resource_windows_arm64.syso; \
		echo "✓ Windows version resources generated."; \
	else \
		echo "⚠ goversioninfo not found; building without PE version resources."; \
		echo "  Install: go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.7.0"; \
	fi

build: resources
	@echo "Building Duster $(VERSION) for Windows AMD64..."
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_NAME) .
	@echo "✓ Built: $(BINARY_NAME)"

build-amd64: resources
	@mkdir -p $(DIST_DIR)
	@echo "Building Duster $(VERSION) for Windows AMD64..."
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(DIST_DIR)/duster-windows-amd64.exe .
	@echo "✓ Built: $(DIST_DIR)/duster-windows-amd64.exe"

build-arm64: resources
	@mkdir -p $(DIST_DIR)
	@echo "Building Duster $(VERSION) for Windows ARM64..."
	GOOS=windows GOARCH=arm64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(DIST_DIR)/duster-windows-arm64.exe .
	@echo "✓ Built: $(DIST_DIR)/duster-windows-arm64.exe"

build-all: build-amd64 build-arm64
	@echo "✓ All architectures built."

# ── Test & Quality ───────────────────────────────────────────────────
test:
	@echo "Running test suite..."
	$(GO) test -count=1 -v ./...

vet:
	@echo "Running Go vet..."
	$(GO) vet ./...
	@echo "✓ Go vet passed."

lint:
	@echo "Running golangci-lint..."
	@if command -v golangci-lint > /dev/null 2>&1; then \
		golangci-lint run ./...; \
		echo "✓ Lint passed."; \
	else \
		echo "⚠ golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"; \
	fi

# Install the repo's git hooks (pre-commit mirrors CI's gofmt/vet gates)
hooks:
	git config core.hooksPath .githooks
	@echo "✓ Git hooks installed (core.hooksPath = .githooks)."

verify:
	@echo "Running build verification..."
	bash scripts/verify_build.sh

# ── Release ──────────────────────────────────────────────────────────
release: clean test vet build-all portable installer checksums
	@echo ""
	@echo "╔══════════════════════════════════════════════════╗"
	@echo "║  Release v$(VERSION) build complete!            ║"
	@echo "╚══════════════════════════════════════════════════╝"
	@echo ""
	@echo "Artifacts in $(DIST_DIR)/:"
	@ls -lh $(DIST_DIR)/*.exe $(DIST_DIR)/*.zip $(DIST_DIR)/*.txt 2>/dev/null || true
	@echo ""
	@echo "Next: Run Inno Setup on installer/duster-setup.iss to build the .exe installer."

portable: build-all
	@echo "Creating portable ZIP archives..."
	@mkdir -p $(DIST_DIR)/portable/Duster-$(VERSION)-Portable-x64
	@mkdir -p $(DIST_DIR)/portable/Duster-$(VERSION)-Portable-arm64
	@cp $(DIST_DIR)/duster-windows-amd64.exe $(DIST_DIR)/portable/Duster-$(VERSION)-Portable-x64/du.exe
	@cp $(DIST_DIR)/duster-windows-arm64.exe $(DIST_DIR)/portable/Duster-$(VERSION)-Portable-arm64/du.exe
	@cp README.md LICENSE SECURITY.md $(DIST_DIR)/portable/Duster-$(VERSION)-Portable-x64/ 2>/dev/null || true
	@cp README.md LICENSE SECURITY.md $(DIST_DIR)/portable/Duster-$(VERSION)-Portable-arm64/ 2>/dev/null || true
	@printf '@echo off\ntitle Duster\n"%%~dp0du.exe" --version\ncmd /K\n' > $(DIST_DIR)/portable/Duster-$(VERSION)-Portable-x64/Launch-Duster.bat
	@printf '@echo off\ntitle Duster\n"%%~dp0du.exe" --version\ncmd /K\n' > $(DIST_DIR)/portable/Duster-$(VERSION)-Portable-arm64/Launch-Duster.bat
	@cd $(DIST_DIR)/portable && zip -rq ../Duster-$(VERSION)-Portable-x64.zip Duster-$(VERSION)-Portable-x64/
	@cd $(DIST_DIR)/portable && zip -rq ../Duster-$(VERSION)-Portable-arm64.zip Duster-$(VERSION)-Portable-arm64/
	@echo "✓ Portable archives created."

installer: build-amd64
	@echo "Staging installer artifacts..."
	@mkdir -p $(DIST_DIR)/installer
	@cp $(DIST_DIR)/duster-windows-amd64.exe $(DIST_DIR)/installer/
	@echo "✓ Installer artifacts staged in $(DIST_DIR)/installer/"
	@echo "  → Open installer/duster-setup.iss in Inno Setup to compile the .exe installer."

checksums:
	@echo "Generating SHA-256 checksums..."
	@cd $(DIST_DIR) && shasum -a 256 *.exe *.zip 2>/dev/null > checksums-sha256.txt || \
		sha256sum *.exe *.zip 2>/dev/null > checksums-sha256.txt || true
	@echo "✓ Checksums: $(DIST_DIR)/checksums-sha256.txt"

# ── Clean ────────────────────────────────────────────────────────────
clean:
	@echo "Cleaning build artifacts..."
	@rm -f $(BINARY_NAME)
	@rm -f resource_windows_*.syso
	@rm -rf $(DIST_DIR)
	@$(GO) clean
	@echo "✓ Clean."
