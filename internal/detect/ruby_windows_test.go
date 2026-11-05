package detect

import "testing"

func TestRubyInstallerRoot(t *testing.T) {
	for path, want := range map[string]string{
		`C:\Ruby40-arm\bin\ruby.exe`:                            `C:\Ruby40-arm`,
		`D:\Ruby34-x64\bin\ruby.exe`:                            `D:\Ruby34-x64`,
		`C:\Users\Ann\scoop\apps\ruby\3.4.7\bin\ruby.exe`:       "",
		`C:\Ruby40-arm\lib\ruby.exe`:                            "",
		`C:\Users\Ann\AppData\Local\Programs\Ruby\bin\ruby.exe`: "",
	} {
		if got := rubyInstallerRoot(path); got != want {
			t.Errorf("%s: got %q, want %q", path, got, want)
		}
	}
}
