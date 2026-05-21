package elevation

import (
	"testing"
)

func TestIsAdmin(t *testing.T) {
	// The function should return a boolean value without panicking
	isAdmin := IsAdmin()
	t.Logf("IsAdmin returned: %v", isAdmin)
}

func TestRequestElevation(t *testing.T) {
	// On non-Windows platforms, RequestElevation should fail gracefully
	err := RequestElevation()
	t.Logf("RequestElevation returned err: %v", err)
}
