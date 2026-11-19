//go:build !windows

package run

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// TerminalCommand formats argv for pasting into a POSIX shell.
func TerminalCommand(argv []string) string {
	quoted := make([]string, len(argv))
	for i, arg := range argv {
		quoted[i] = terminalQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func terminalQuote(arg string) string {
	const plain = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._/:=@"
	if arg != "" && !strings.ContainsFunc(arg, func(r rune) bool {
		return !strings.ContainsRune(plain, r)
	}) {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

// OpenTerminal hands a one-shot script to the desktop terminal. The script
// removes itself before starting the command and keeps stderr visible on failure.
func OpenTerminal(ctx context.Context, r Runner, argv, env []string, path, kubeconfig, closeHint string) error {
	f, err := os.CreateTemp("", "hearth-*.command")
	if err != nil {
		return fmt.Errorf("create terminal script: %w", err)
	}
	name := f.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(name)
		}
	}()
	err = writeTerminalScript(f, argv, path, kubeconfig, closeHint)
	if err != nil {
		return err
	}
	args := terminalLauncher(name)
	res, err := r.Run(ctx, Cmd{Args: args, Env: env})
	if err != nil {
		return fmt.Errorf("open terminal: %w", err)
	}
	if res.ExitCode != 0 {
		err = fmt.Errorf("%s: exit %d: %s", args[0], res.ExitCode, strings.TrimSpace(string(res.Stderr)))
		return err
	}
	return nil
}

func writeTerminalScript(f *os.File, argv []string, path, kubeconfig, closeHint string) error {
	defer func() { _ = f.Close() }()
	if err := f.Chmod(0o700); err != nil {
		return fmt.Errorf("chmod terminal script: %w", err)
	}
	_, err := fmt.Fprintf(f, "#!/bin/sh\n/bin/rm -f -- \"$0\"\nexport PATH=%s KUBECONFIG=%s\n%s\nstatus=$?\nif [ \"$status\" -ne 0 ]; then\n  printf '%%s\\n' %s\n  read -r answer\nfi\nexit \"$status\"\n",
		terminalQuote(path), terminalQuote(kubeconfig), TerminalCommand(argv), terminalQuote(closeHint))
	if err != nil {
		return fmt.Errorf("write terminal script: %w", err)
	}
	return nil
}
