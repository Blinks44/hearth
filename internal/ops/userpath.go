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

// UserVarEdit sets, updates, or deletes the user's environment variable
// Name in the Windows registry, as a step of a plan. By default an existing
// variable with another value is not changed; Replace updates it and Delete
// removes it, both restoring the previous value on rollback. Public shows
// Value in the plan preview instead of masking it as a secret.
type UserVarEdit struct {
	Name, Value string
	Before      string
	Replace     bool
	Delete      bool
	Public      bool
}
