//go:build !darwin

package issues

// osDirs are OS-default PATH prefixes that may legitimately be absent.
// Non-darwin platforms do not suppress macOS-only cryptex or /usr/local/bin entries.
var osDirs []string
