package detect

import (
	"context"
	"path/filepath"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

const javaExe = "java.exe"

func javaLauncher(string) bool { return false }
func javaSystemPaths(_ context.Context, _ run.Runner, env sysenv.Env) ([]string, string) {
	var paths []string
	for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		if p := env.Get(key); p != "" {
			for _, vendor := range []string{"Java", "Eclipse Adoptium", "Microsoft", "Amazon Corretto", "Zulu", "BellSoft"} {
				paths = append(paths, filepath.Join(p, vendor, "*", "bin", javaExe))
			}
		}
	}
	return paths, ""
}
