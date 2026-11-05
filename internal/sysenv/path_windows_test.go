package sysenv

import "testing"

// A fresh Windows user Path ends in ";": the empty entry is not the working directory.
func TestPathsSkipEmpty(t *testing.T) {
	got := Paths(Env{`PATH=C:\WINDOWS;;C:\Users\me\AppData\Local\Microsoft\WindowsApps;`})
	if len(got) != 2 || got[0].Dir != `C:\WINDOWS` {
		t.Fatalf("entries: %+v", got)
	}
}
