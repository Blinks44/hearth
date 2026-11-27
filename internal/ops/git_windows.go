package ops

func gitDefaultCredentialHelper() string { return "manager" }

// SSHAddArgv returns the ssh-add command for adding keyPath on Windows.
func SSHAddArgv(keyPath string) []string {
	return []string{"ssh-add", keyPath}
}
