package run

import (
	"encoding/base64"
	"encoding/binary"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestTerminalArgs(t *testing.T) {
	argv := []string{`C:\Dev Tools\kubectl.exe`, `--context=team's "dev"; 生产`, "-n", "shop"}
	args := terminalArgs(argv)
	if len(args) != 5 || !slices.Equal(args[:4], []string{"-NoLogo", "-NoProfile", "-NoExit", "-EncodedCommand"}) {
		t.Fatalf("PowerShell args = %q", args)
	}
	data, err := base64.StdEncoding.DecodeString(args[4])
	if err != nil {
		t.Fatal(err)
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(data[2*i:])
	}
	script := string(utf16.Decode(units))
	want := `'C:\Dev Tools\kubectl.exe', '"--context=team''s \"dev\"; 生产" -n shop'`
	if !strings.Contains(script, want) {
		t.Errorf("script %q does not contain %q", script, want)
	}
}

func TestTerminalArgsInDir(t *testing.T) {
	args := terminalArgsInDir(`C:\Users\dev\my app`, nil)
	if len(args) != 5 {
		t.Fatalf("PowerShell args = %q", args)
	}
	data, err := base64.StdEncoding.DecodeString(args[4])
	if err != nil {
		t.Fatal(err)
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(data[2*i:])
	}
	script := string(utf16.Decode(units))
	want := `Set-Location -LiteralPath 'C:\Users\dev\my app'`
	if script != want {
		t.Errorf("script = %q, want %q", script, want)
	}
}
