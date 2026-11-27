package ops

func gitDefaultCredentialHelper() string { return "cache" }

// SSHAddArgv returns the ssh-add command for adding keyPath on Linux.
func SSHAddArgv(keyPath string) []string {
	return []string{"ssh-add", keyPath}
}
