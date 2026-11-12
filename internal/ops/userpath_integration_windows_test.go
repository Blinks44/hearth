//go:build integration

package ops

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// TestUserPathRollbackReal changes the user's Path in the registry for
// real and fails the check: the old value comes back, as when putting an
// entry first does not pass. Run it only on a test machine.
func TestUserPathRollbackReal(t *testing.T) {
	before, err := sysenv.UserPath()
	if err != nil {
		t.Fatal(err)
	}
	items := strings.Split(strings.TrimRight(before, ";"), ";")
	if len(items) < 2 {
		t.Skip("the user's Path has fewer than two entries")
	}
	after := strings.Join(append([]string{items[len(items)-1]}, items[:len(items)-1]...), ";")
	p := Plan{Title: "t", Steps: []Step{{UserPath: &UserPathEdit{Before: before, After: after}}},
		Verify: func(context.Context) (string, error) {
			if now, err := sysenv.UserPath(); err != nil || now != after {
				t.Errorf("before the check the Path is %q, %v", now, err)
			}
			return "", errors.New("check failed")
		}}
	s := Execute(t.Context(), nil, p, io.Discard, func(State) {})
	got, err := sysenv.UserPath()
	if err != nil {
		t.Fatal(err)
	}
	if s.Phase != NotVerified || got != before {
		t.Fatalf("phase %v, err %q, Path %q, want %q", s.Phase, s.Err, got, before)
	}
}
