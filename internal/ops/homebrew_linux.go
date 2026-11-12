package ops

import "github.com/Blinks44/hearth/internal/sysenv"

// homebrew: Hearth installs Homebrew on macOS alone; on Linux the
// system's manager installs.
func homebrew(sysenv.Env) (string, []Step, []Note) { return "", nil, nil }

// cltSteps: the Command Line Tools are macOS's.
func cltSteps() ([]Step, []Note) { return nil, nil }
