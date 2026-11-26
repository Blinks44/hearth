//go:build darwin

package issues

import (
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestDarwinOSDirsIgnored(t *testing.T) {
	snap := &inventory.Snapshot{TerminalPath: []inventory.PathEntry{
		{Dir: "/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/bin", Missing: true},
		{Dir: "/pkg/env/global/bin", Missing: true},
		{Dir: "/usr/local/bin", Missing: true},
	}}
	if got := Find(snap, nil, nil, time.Now()); len(got) != 0 {
		t.Fatalf("darwin OS dirs reported as issues: %+v", got)
	}
}
