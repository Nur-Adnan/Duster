package elevation

import (
	"runtime"
	"testing"
)

func TestIsAdmin(t *testing.T) {
	// The function should return a boolean value without panicking
	isAdmin := IsAdmin()
	t.Logf("IsAdmin returned: %v", isAdmin)
}

func TestRequestElevation(t *testing.T) {
	if runtime.GOOS == "windows" {
		// On Windows this triggers a live ShellExecuteW "runas" — a real UAC
		// prompt that, if approved, relaunches the test binary elevated and
		// re-enters this test in a loop. Only the non-Windows graceful-failure
		// path is safe to exercise.
		t.Skip("skipping live elevation request on Windows")
	}
	err := RequestElevation()
	t.Logf("RequestElevation returned err: %v", err)
}
