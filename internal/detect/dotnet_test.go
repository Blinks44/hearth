package detect

import (
	"reflect"
	"testing"
)

func TestDotnetList(t *testing.T) {
	for _, tt := range []struct {
		name, out string
		sdk       bool
		want      int
		version   string
	}{
		{"SDK path with spaces", "8.0.405 [/Program Files/dotnet/sdk]\n9.0.102 [/Program Files/dotnet/sdk]", true, 2, "9.0.102"},
		{"frameworks", "Microsoft.NETCore.App 9.0.1 [/root/shared/Microsoft.NETCore.App]\nMicrosoft.AspNetCore.App 9.0.1 [/root/shared/Microsoft.AspNetCore.App]", false, 2, "9.0.1"},
		{"preview", "10.0.100-preview.1 [/root/sdk]", true, 1, "10.0.100-preview.1"},
		{"noise", "Welcome to .NET\n9.0 [/root/sdk]\nnot a list", true, 0, ""},
		{"duplicate", "9.0.102 [/root/sdk]\n9.0.102 [/root/sdk]", true, 1, "9.0.102"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := dotnetList(tt.out, tt.sdk)
			if len(got) != tt.want || len(got) > 0 && got[0].Version != tt.version {
				t.Fatalf("%+v", got)
			}
		})
	}
	if got := dotnetList("9.0.102 [/root/sdk]", true); !reflect.DeepEqual(got, dotnetList("9.0.102 [/root/sdk]\r\n", true)) {
		t.Fatal("CRLF changes list")
	}
}
