package ops

// UserPathName names the user's Path in the Windows registry, as the
// operations log and the dialog show it.
const UserPathName = `HKCU\Environment\Path`

// UserPathEdit changes the user's Path in the Windows registry, as a step
// of a plan. Before is the value the plan was made from, After what it
// becomes, both as the registry holds them, %VARIABLES% unexpanded. Add,
// in their place, puts an entry at the end of the Path as it is when the
// step runs, as an installer before it may have changed it.
type UserPathEdit struct {
	Before, After string
	Add           string
}
