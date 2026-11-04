//go:build !windows

package detect

func jsLaunchers(name string) []string { return []string{name} }

// userCacheDir is where Corepack caches under the home when no cache
// variable is set.
const userCacheDir = ".cache"
