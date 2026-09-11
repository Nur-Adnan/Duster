// Package e2e drives the real du.exe the way a person does: keys typed into a
// Windows pseudo console (ConPTY), Windows' own dialogs answered, and the
// result checked on disk and in the registry.
//
// The tests change the machine (startup entries, installed apps, the Recycle
// Bin's size), so they run only when DU_E2E_BIN names the binary.
// windows-smoke.yml sets it on a throwaway runner; never set it on a workstation.
package e2e
