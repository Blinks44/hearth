//go:build unix

package ops

import (
	"strconv"
	"strings"
)

// quoteArg quotes a for a POSIX shell where it needs it.
func quoteArg(a string) string {
	if a == "" || strings.ContainsAny(a, " \t\"'$\\;&|<>*?()[]{}`!#~") {
		return strconv.Quote(a)
	}
	return a
}
