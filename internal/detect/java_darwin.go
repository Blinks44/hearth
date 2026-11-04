package detect

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

const javaExe = "java"

func javaLauncher(path string) bool { return path == "/usr/bin/java" }

func javaSystemPaths(ctx context.Context, r run.Runner, env sysenv.Env) ([]string, string) {
	paths := []string{"/Library/Java/JavaVirtualMachines/*/Contents/Home/bin/java"}
	if home := sources.ManagerDirs(env).Home; home != "" {
		paths = append(paths, filepath.Join(home, "Library/Java/JavaVirtualMachines/*/Contents/Home/bin/java"))
	}
	for _, p := range []string{env.Get("HOMEBREW_PREFIX"), "/opt/homebrew", "/usr/local"} {
		if p != "" {
			paths = append(paths, filepath.Join(p, "Cellar/openjdk*/*/libexec/openjdk.jdk/Contents/Home/bin/java"))
		}
	}
	res, err := r.Run(ctx, run.Cmd{Args: []string{"/usr/libexec/java_home", "-V"}, Env: env})
	def := ""
	if err == nil && res.ExitCode == 0 {
		for line := range strings.Lines(string(res.Stderr) + "\n" + string(res.Stdout)) {
			line = strings.TrimSpace(line)
			start := strings.Index(line, "/")
			if start < 0 {
				continue
			}
			home := strings.TrimSpace(line[start:])
			if exists(filepath.Join(home, "bin", javaExe)) {
				paths = append(paths, filepath.Join(home, "bin", javaExe))
				if strings.HasPrefix(line, "/") {
					def = home
				}
			}
		}
	}
	return paths, def
}
