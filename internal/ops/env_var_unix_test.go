//go:build unix

package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestSetEnvVarPlanUnix(t *testing.T) {
	home := t.TempDir()
	zshrc := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(zshrc, []byte("export FOO=1\n# >>> hearth >>>\nexport JAVA_HOME='/old/jdk'\n# <<< hearth <<<\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"HOME=" + home, "SHELL=/bin/zsh"}

	p, err := SetEnvVarPlan("JAVA_HOME", "/Library/Java/Home", env, nil)
	if err != nil {
		t.Fatalf("SetEnvVarPlan(JAVA_HOME): %v", err)
	}
	if len(p.Steps) != 1 || p.Steps[0].Edit == nil {
		t.Fatalf("expected one file edit step, got %+v", p.Steps)
	}
	after := p.Steps[0].Edit.After
	if strings.Contains(after, "/old/jdk") {
		t.Errorf("old JAVA_HOME still present:\n%s", after)
	}
	if !strings.Contains(after, "export JAVA_HOME='/Library/Java/Home'") {
		t.Errorf("new JAVA_HOME missing:\n%s", after)
	}
	if !strings.Contains(after, "export FOO=1") {
		t.Errorf("user line outside block lost:\n%s", after)
	}

	// Fish shell syntax
	fishEnv := sysenv.Env{"HOME=" + home, "SHELL=/usr/local/bin/fish"}
	pf, err := SetEnvVarPlan("ANDROID_HOME", "/opt/android-sdk", fishEnv, nil)
	if err != nil {
		t.Fatalf("SetEnvVarPlan fish: %v", err)
	}
	if !strings.Contains(pf.Steps[0].Edit.After, "set -gx ANDROID_HOME '/opt/android-sdk'") {
		t.Errorf("fish line missing:\n%s", pf.Steps[0].Edit.After)
	}

	// Disallowed var or invalid values
	if _, err := SetEnvVarPlan("PATH", "/tmp", env, nil); err == nil {
		t.Error("PATH should be rejected by allowlist")
	}
	if _, err := SetEnvVarPlan("JAVA_HOME", "relative/path", env, nil); err == nil {
		t.Error("relative path should be rejected")
	}
	if _, err := SetEnvVarPlan("JAVA_HOME", "/tmp/bad\nid", env, nil); err == nil {
		t.Error("newline in value should be rejected")
	}
}
