# Contributing to Duster

Duster is a fully Windows-native system deep-cleaning and optimization utility built entirely in Go. We welcome community contributions to help improve safety, expand cleanup modules, and optimize CLI diagnostics!

## Setup

To develop and build Duster locally:

1. **Go Toolchain**: Install Go version 1.25.x or higher on your system.
2. **Clone the Repository**:
   ```bash
   git clone https://github.com/Nur-Adnan/duster.git
   cd duster
   ```
3. **Format & Quality Check**: Install standard Go formatting and linting tools:
   ```bash
   go install golang.org/x/tools/cmd/goimports@latest
   ```

## Development Workflow

### Building Duster

To compile the native Windows executable:

```powershell
# Build standard binary
go build -o du.exe ./main.go

# Run Duster directly
go run ./main.go status
```

### Running Tests

Implement robust, table-driven unit tests. To execute the test suite:

```powershell
go test -v ./...
```

---

## Architectural & Style Guidelines

### 1. Pure Go Architecture & Windows-Native Focus
- Maintain strict separation of concerns:
  - **TUI Layer (`cmd/`)**: Handles CLI parsing (Cobra), Bubble Tea status rendering, and Lipgloss layouts.
  - **Windows core engine (`lib/`)**: Natively manages deletions, Recycle Bin actions, registry lookups, and subprocess controls.
- Utilize Go standard libraries and explicit Win32/NTFS dynamic API invocations (`golang.org/x/sys/windows`) rather than calling external Unix wrappers or spawn commands.

### 2. High Safety & Security Boundaries
- **Reparse Points / Junctions**: Never walk directory trees blindly. Verify `os.ModeSymlink` and standard reparse points to unlink them directly rather than recursively traversing them.
- **OneDrive Cache Protection**: Always skip files marked with the `FILE_ATTRIBUTE_OFFLINE` (0x1000) attribute to prevent thrashing offline storage caches or triggering heavy cloud downloads.
- **Environment Safety**: Never resolve system paths using mutable environment variables (e.g., `%SYSTEMROOT%`). Always retrieve secure, immutable folders natively using Win32 API calls (`GetSystemDirectoryW`, `GetWindowsDirectoryW`).
- **Registry Operations**: Open registry keys with strict read-only permissions (`registry.QUERY_VALUE` | `registry.READ`).

### 3. Interactive TUI Aesthetics
- Leverage Charmbracelet `bubbletea` and `lipgloss` cleanly.
- Define layout styles as package-level constants to avoid inline dynamic allocation overhead in the main Bubble Tea `View()` loops.

---

## Pull Request Process

1. **Fork the Repository**: Create a feature branch from the latest `main`.
2. **Clean Commits**: Follow standard Conventional Commits rules:
   - `feat:` for new cleanup modules, optimization targets, or diagnostic commands.
   - `fix:` for corrections to scanning logic, unit formats, or privilege checks.
   - `refactor:` for codebase structure updates.
3. **Run Verification**: Ensure `go test ./...` passes.
4. **Submit PR**: Open a Pull Request targeting the `main` branch.
