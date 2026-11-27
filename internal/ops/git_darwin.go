package ops

func gitDefaultCredentialHelper() string { return "osxkeychain" }

// SSHAddArgv returns the ssh-add command for adding keyPath on macOS.
func SSHAddArgv(keyPath string) []string {
	return []string{"ssh-add", "--apple-use-keychain", keyPath}
}
