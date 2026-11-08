package ops

import "testing"

// A step started through UAC that the user did not allow exits with
// ERROR_CANCELLED, which says so; other codes fail as the program says.
func TestAdminRefusedWindows(t *testing.T) {
	if adminRefused(1223, "") == "" {
		t.Error("1223 is not a refusal")
	}
	if got := adminRefused(1, ""); got != "" {
		t.Errorf("exit 1: %q", got)
	}
}
