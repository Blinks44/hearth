package ops

import "github.com/Blinks44/hearth/internal/sysenv"

// installerSteps: on Windows, tools install through WinGet.
func installerSteps(string, sysenv.Env) ([]Step, bool) { return nil, false }

// homebrew: Hearth installs Homebrew on macOS alone.
func homebrew(sysenv.Env) (string, []Step, []Note) { return "", nil, nil }

// cltSteps: the Command Line Tools are macOS's.
func cltSteps() ([]Step, []Note) { return nil, nil }
