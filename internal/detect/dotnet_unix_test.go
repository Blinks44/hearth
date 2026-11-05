//go:build !windows

package detect

import (
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestDotnetHosts(t *testing.T) {
	dir := root(t)
	home := filepath.Join(dir, "home")
	path := fakeExe(t, filepath.Join(home, ".dotnet", "dotnet"))
	shim := fakeExe(t, filepath.Join(home, ".local", "share", "mise", "shims", "dotnet"))
	writeFile(t, filepath.Join(home, ".dotnet", "tools", ".store", "dotnet-ef", "9.0.1", "dotnet-ef", "9.0.1", "tools", "net8.0", "any", "DotnetToolSettings.xml"), `<DotNetCliTool><Commands><Command Name="dotnet-ef" /></Commands></DotNetCliTool>`)
	env := sysenv.Env{"HOME=" + home, "PATH=" + filepath.Dir(path), "DOTNET_ROOT=" + filepath.Dir(path)}
	r := &run.Fake{}
	r.Stdout("8.0.405 ["+filepath.Join(home, ".dotnet", "sdk")+"]\n9.0.102 ["+filepath.Join(home, ".dotnet", "sdk")+"]", path, "--list-sdks")
	r.Stdout("Microsoft.NETCore.App 9.0.1 ["+filepath.Join(home, ".dotnet", "shared", "Microsoft.NETCore.App")+"]", path, "--list-runtimes")
	st, err := Dotnet(t.Context(), r, env, sysenv.Env{"PATH=" + filepath.Dir(shim)})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Installations) < 2 {
		t.Fatalf("%+v", st.Installations)
	}
	host := st.Installations[0]
	if host.Version != "9.0.102" || host.OnPath != 1 || len(host.Dotnet.SDKs) != 2 || len(host.Dotnet.Runtimes) != 1 || len(host.Packages) != 1 || host.Packages[0].Commands[0] != "dotnet-ef" {
		t.Fatalf("%+v %+v", host, host.Dotnet)
	}
	if !st.Installations[1].Source.Shim {
		t.Fatalf("shim ran: %+v", r.Calls())
	}
	for _, call := range r.Calls() {
		if len(call.Args) != 2 || call.Dir != "" || call.Args[0] == shim {
			t.Fatalf("unexpected call %+v", call)
		}
	}
}

func TestDotnetProbeFailure(t *testing.T) {
	home := root(t)
	path := fakeExe(t, filepath.Join(home, ".dotnet", "dotnet"))
	r := &run.Fake{}
	r.Stdout("Microsoft.NETCore.App 9.0.1 [/root/shared/Microsoft.NETCore.App]", path, "--list-runtimes")
	st, err := Dotnet(t.Context(), r, sysenv.Env{"HOME=" + home, "PATH=" + filepath.Dir(path)}, sysenv.Env{"PATH="})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Installations) < 1 || st.Installations[0].Dotnet.Err == "" || len(st.Installations[0].Dotnet.Runtimes) != 1 {
		t.Fatalf("%+v", st)
	}
}
