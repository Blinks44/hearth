//go:build !darwin

package projects

func isGitStub(string) bool { return false }
