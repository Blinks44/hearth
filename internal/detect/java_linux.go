package detect

import (
	"context"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

const javaExe = "java"

func javaLauncher(string) bool { return false }
func javaSystemPaths(context.Context, run.Runner, sysenv.Env) ([]string, string) {
	return []string{"/usr/lib/jvm/*/bin/java", "/opt/java/*/bin/java", "/home/linuxbrew/.linuxbrew/Cellar/openjdk*/*/bin/java"}, ""
}
