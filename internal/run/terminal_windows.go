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

// OpenTerminal opens a visible PowerShell console through the system's
// default terminal host. PowerShell owns the interactive child and stays open
// so the user can read errors from kubectl.
func OpenTerminal(ctx context.Context, _ Runner, _ string, argv, env []string, _, _, _ string, overrides ...string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("open terminal: %w", err)
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return fmt.Errorf("locate PowerShell: %w", err)
	}
	program := filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe")
	line, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{program}, terminalArgs(argv)...)))
	if err != nil {
		return fmt.Errorf("encode PowerShell command: %w", err)
	}
	// Leave standard handles unset so Windows wires them to the new console.
	// os/exec supplies redirected handles even for a new console.
	info := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var process windows.ProcessInformation
	flags := uint32(windows.CREATE_NEW_CONSOLE | windows.CREATE_UNICODE_ENVIRONMENT)
	if err := windows.CreateProcess(nil, line, nil, nil, false, flags, terminalEnvironment(append(slices.Clone(env), overrides...)), nil, &info, &process); err != nil {
		return fmt.Errorf("open PowerShell: %w", err)
	}
	_ = windows.CloseHandle(process.Thread)
	_ = windows.CloseHandle(process.Process)
	return nil
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
	// ProcessStartInfo preserves native argv even in Windows PowerShell 5,
	// whose invocation operator can change arguments that contain quotes.
	args := make([]string, 0, len(argv)-1)
	for _, arg := range argv[1:] {
		args = append(args, syscall.EscapeArg(arg))
	}
	script := "$p = [Diagnostics.ProcessStartInfo]::new(" + powershellQuote(argv[0]) + ", " + powershellQuote(strings.Join(args, " ")) + ")\n" +
		"$p.UseShellExecute = $false\n[Diagnostics.Process]::Start($p).WaitForExit()"
	var data []byte
	for _, unit := range utf16.Encode([]rune(script)) {
		data = binary.LittleEndian.AppendUint16(data, unit)
	}
	return []string{"-NoLogo", "-NoProfile", "-NoExit", "-EncodedCommand", base64.StdEncoding.EncodeToString(data)}
}
