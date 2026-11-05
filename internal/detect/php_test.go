package detect

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestPHPIniFiles(t *testing.T) {
	ini := filepath.Join(t.TempDir(), "php.ini")
	extra := filepath.Join(filepath.Dir(ini), "conf.d", "redis.ini")
	for _, tc := range []struct {
		name, output string
		want         []string
	}{
		{"loaded and additional", "Configuration File (php.ini) Path: /etc\nLoaded Configuration File: " + ini + "\nScan for additional .ini files in: /etc/conf.d\nAdditional .ini files parsed: " + extra + ",\n" + extra, []string{ini, extra}},
		{"none", "Loaded Configuration File: (none)\nAdditional .ini files parsed: (none)", nil},
		{"quoted, as PHP 8.5 prints it", "Configuration File (php.ini) Path: \"/etc\"\nLoaded Configuration File:         \"" + ini + "\"\nAdditional .ini files parsed:      (none)", []string{ini}},
		{"failure", "PHP Warning: cannot load module", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := phpIniFiles(tc.output); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPHPModules(t *testing.T) {
	got := phpModules("PHP Warning: ignored\n[PHP Modules]\nredis\nCore\nZend OPcache\n\n[Zend Modules]\nZend OPcache\n")
	if want := []string{"Core", "Zend OPcache", "redis"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestComposerPackages(t *testing.T) {
	for _, tc := range []struct{ name, installed string }{
		{"composer 2", `{"packages":[{"name":"laravel/installer","version":"v5.11.2","bin":["bin/laravel"]},{"name":"dependency/lib","version":"1.0"}]}`},
		{"composer 1", `[{"name":"laravel/installer","version":"v5.11.2","bin":["bin/laravel"]}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "composer.json"), `{"require":{"laravel/installer":"^5"}}`)
			writeFile(t, filepath.Join(home, "vendor", "composer", "installed.json"), tc.installed)
			pkgs := composerPackages(home)
			if len(pkgs) != 1 || pkgs[0].Name != "laravel/installer" || pkgs[0].Version != "5.11.2" || !reflect.DeepEqual(pkgs[0].Commands, []string{"laravel"}) {
				t.Fatalf("packages %+v", pkgs)
			}
		})
	}
}
