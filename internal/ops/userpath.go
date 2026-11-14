package ops

// UserPathName names the user's Path in the Windows registry, as the
// operations log and the dialog show it.
const UserPathName = `HKCU\Environment\Path`

// UserPathEdit changes the user's Path in the Windows registry, as a step
// of a plan. Before is the value the plan was made from, After what it
// becomes, both as the registry holds them, %VARIABLES% unexpanded. Add,
// in their place, puts an entry at the start of the Path as it is when the
// step runs, as an installer before it may have changed it.
type UserPathEdit struct {
	Before, After string
	Add           string
}

// UserVarsName names the user's environment variables in the Windows
// registry.
const UserVarsName = `HKCU\Environment`

// UserVarEdit sets the user's environment variable Name to Value in the
// Windows registry, as a step of a plan: one there already, at another
// value, is not changed. It comes back as an Edit does.
type UserVarEdit struct {
	Name, Value string
}
