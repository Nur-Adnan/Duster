//go:build !windows

package winapi

// Dynamic placeholders to ensure non-Windows cross-compilation resolves cleanly.
type shQueryRecycleBinStub struct{}
