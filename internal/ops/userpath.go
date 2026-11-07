package ops

// UserPathName names the user's Path in the Windows registry, as the
// operations log and the dialog show it.
const UserPathName = `HKCU\Environment\Path`

// UserPathEdit changes the user's Path in the Windows registry, as a step
// of a plan. Before is the value the plan was made from, After what it
// becomes, both as the registry holds them, %VARIABLES% unexpanded.
type UserPathEdit struct {
	Before, After string
}
