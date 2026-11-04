package detect

import "github.com/Blinks44/hearth/internal/sysenv"

const phpBin = "bin/php"

func phpOSGlobs(_ sysenv.Env, _ string) []string { return []string{"/usr/bin/php[0-9]*"} }
func phpHerd(_, _ string) bool                   { return false }
