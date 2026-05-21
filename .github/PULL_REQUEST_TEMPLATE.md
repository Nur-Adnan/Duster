## Description

Please include a summary of the changes and the related issue/feature request. 

- Fixes # (issue)
- Feature description:

## Type of Change

Please select the option that applies:

- [ ] 🐛 Bug fix (non-breaking change which fixes an issue)
- [ ] ✨ New feature (non-breaking change which adds functionality)
- [ ] 🧹 Cleanup rule addition (adds a new cache or temporary directory target)
- [ ] ⚡ Performance optimization (improves speed, CPU, or memory metrics)
- [ ] 📝 Documentation update (README, CONTRIBUTING, inline docs)

## Checklist

Before submitting this Pull Request, please ensure:

- [ ] My code follows the code style guidelines detailed in [CONTRIBUTING.md](file:///absolute/path/to/CONTRIBUTING.md).
- [ ] I have run unit tests locally using `go test ./...` and confirmed all tests pass.
- [ ] I have formatted my code using standard Go formatting tools (`go fmt` / `goimports`).
- [ ] My changes do not widen destructive cleanup boundaries to folders outside standard temp/AppCaches without explicit user confirmation.
- [ ] All dynamic reparse points, junctions, or symbolic links are safely handled to prevent recursive directory traversal.
- [ ] Offline placeholders (e.g. OneDrive cloud-only files) are skipped using standard attributes (`FILE_ATTRIBUTE_OFFLINE`).
- [ ] I have verified this change works stably across Windows Terminal, standard CMD, and PowerShell.
