package run

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Terminals is none on Windows: commands open in PowerShell's console.
func Terminals() []string { return nil }

// TerminalCommand formats argv for pasting into PowerShell.
func TerminalCommand(argv []string) string {
	quoted := make([]string, 0, len(argv)+1)
	quoted = append(quoted, "&")
	for _, arg := range argv {
		quoted = append(quoted, powershellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func powershellQuote(s string) string {
	return "'" + strings.NewReplacer("'", "''", "‘", "‘‘", "’", "’’", "‚", "‚‚", "‛", "‛‛").Replace(s) + "'"
}

// DefaultShellArgs returns nil on Windows so OpenTerminalInDir opens an
// interactive PowerShell session directly.
func DefaultShellArgs(_ []string) []string { return nil }

// OpenTerminal opens a visible PowerShell console through the chosen terminal
// executable or the system's default terminal host. PowerShell owns the
// interactive child and stays open so the user can read errors from kubectl.
func OpenTerminal(ctx context.Context, r Runner, app string, argv, env []string, path, kubeconfig, closeHint string, overrides ...string) error {
	return OpenTerminalInDir(ctx, r, app, "", argv, env, path, kubeconfig, closeHint, overrides...)
}

// OpenTerminalInDir opens a visible PowerShell console starting in dir when dir
// is non-empty.
func OpenTerminalInDir(ctx context.Context, _ Runner, app, dir string, argv, env []string, _, _, _ string, overrides ...string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("open terminal: %w", err)
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return fmt.Errorf("locate PowerShell: %w", err)
	}
	program := filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd, newConsole := windowsTerminalCmd(app, program, dir, argv)
	line, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(cmd))
	if err != nil {
		return fmt.Errorf("encode PowerShell command: %w", err)
	}
	var currentDir *uint16
	if dir != "" {
		currentDir, err = windows.UTF16PtrFromString(dir)
		if err != nil {
			return fmt.Errorf("encode working directory: %w", err)
		}
	}
	// Leave standard handles unset so Windows wires them to the new console.
	// os/exec supplies redirected handles even for a new console.
	info := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var process windows.ProcessInformation
	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT)
	if newConsole {
		flags |= windows.CREATE_NEW_CONSOLE
	}
	if err := windows.CreateProcess(nil, line, nil, nil, false, flags, terminalEnvironment(append(slices.Clone(env), overrides...)), currentDir, &info, &process); err != nil {
		return fmt.Errorf("open terminal: %w", err)
	}
	_ = windows.CloseHandle(process.Thread)
	_ = windows.CloseHandle(process.Process)
	return nil
}

func windowsTerminalCmd(app, powershell, dir string, argv []string) ([]string, bool) {
	psArgs := append([]string{powershell}, terminalArgsInDir(dir, argv)...)
	trimmed := strings.TrimSpace(app)
	if trimmed == "" {
		return psArgs, true
	}
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(trimmed), ".exe"))
	switch base {
	case "wt":
		return append([]string{trimmed, "new-tab", "--"}, psArgs...), false
	case "wezterm", "wezterm-gui":
		return append([]string{trimmed, "start", "--"}, psArgs...), false
	case "alacritty", "ghostty":
		return append([]string{trimmed, "-e"}, psArgs...), false
	case "pwsh", "powershell":
		return append([]string{trimmed}, terminalArgsInDir(dir, argv)...), true
	default:
		return append([]string{trimmed}, psArgs...), false
	}
}

func terminalEnvironment(env []string) *uint16 {
	if env == nil {
		return nil
	}
	var ordered []string
	seen := make(map[string]bool)
	for _, entry := range slices.Backward(env) {
		name := strings.TrimPrefix(entry, "=")
		key, _, _ := strings.Cut(name, "=")
		key = strings.ToUpper(key)
		if len(name) != len(entry) {
			key = "=" + key
		}
		if !seen[key] {
			ordered = append(ordered, entry)
			seen[key] = true
		}
	}
	slices.SortFunc(ordered, func(a, b string) int {
		return strings.Compare(strings.ToUpper(a), strings.ToUpper(b))
	})
	block := utf16.Encode([]rune(strings.Join(ordered, "\x00") + "\x00\x00"))
	return &block[0]
}

func terminalArgs(argv []string) []string {
	return terminalArgsInDir("", argv)
}

func terminalArgsInDir(dir string, argv []string) []string {
	var script string
	if len(argv) == 0 {
		if dir != "" {
			script = "Set-Location -LiteralPath " + powershellQuote(dir)
		}
	} else {
		// ProcessStartInfo preserves native argv even in Windows PowerShell 5,
		// whose invocation operator can change arguments that contain quotes.
		args := make([]string, 0, len(argv)-1)
		for _, arg := range argv[1:] {
			args = append(args, syscall.EscapeArg(arg))
		}
		script = "$p = [Diagnostics.ProcessStartInfo]::new(" + powershellQuote(argv[0]) + ", " + powershellQuote(strings.Join(args, " ")) + ")\n"
		if dir != "" {
			script += "$p.WorkingDirectory = " + powershellQuote(dir) + "\n"
		}
		script += "$p.UseShellExecute = $false\n[Diagnostics.Process]::Start($p).WaitForExit()"
	}
	if script == "" {
		return []string{"-NoLogo", "-NoExit"}
	}
	var data []byte
	for _, unit := range utf16.Encode([]rune(script)) {
		data = binary.LittleEndian.AppendUint16(data, unit)
	}
	return []string{"-NoLogo", "-NoProfile", "-NoExit", "-EncodedCommand", base64.StdEncoding.EncodeToString(data)}
}
