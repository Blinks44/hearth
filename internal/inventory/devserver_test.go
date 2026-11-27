package inventory

import (
	"slices"
	"testing"
)

func TestRedactArgs(t *testing.T) {
	in := []string{
		"node",
		"/dev/app/node_modules/.bin/vite",
		"--port=5173",
		"-p",
		"3000",
		"--token=ghp_supersecret123",
		"--api-key",
		"sk-live-secret",
		"DATABASE_URL=postgres://admin:topsecret@localhost:5432/app",
		"REDIS_URL=redis://:secret@localhost:6379/0",
		"https://ghp_xxx@github.com/org/repo.git",
		"OPENAI_API_KEY=sk-12345",
		"Bearer my-jwt-token",
		"-H",
		"Authorization: Bearer tok123",
		"-pSECRET",
	}
	got := RedactArgs(in)
	want := []string{
		"node",
		"/dev/app/node_modules/.bin/vite",
		"--port=5173",
		"-p",
		"3000",
		"--token=[redacted]",
		"--api-key",
		"[redacted]",
		"DATABASE_URL=postgres://admin:[redacted]@localhost:5432/app",
		"REDIS_URL=redis://:[redacted]@localhost:6379/0",
		"https://[redacted]@github.com/org/repo.git",
		"OPENAI_API_KEY=[redacted]",
		"Bearer [redacted]",
		"-H",
		"Authorization: Bearer [redacted]",
		"-p[redacted]",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("RedactArgs:\n got: %q\nwant: %q", got, want)
	}
}

func TestClassifyDevServer(t *testing.T) {
	cases := []struct {
		name string
		l    DatabaseListener
		want DevServerInfo
	}{
		{
			name: "vite",
			l:    DatabaseListener{Process: "node", Port: "5173", Args: []string{"node", "/app/node_modules/.bin/vite", "--port", "5173"}},
			want: DevServerInfo{Kind: "Vite", Role: RoleDevServer, HTTPOpenable: true},
		},
		{
			name: "nextjs",
			l:    DatabaseListener{Process: "node", Port: "3000", Args: []string{"next-server", "(v15.2.0)"}},
			want: DevServerInfo{Kind: "Next.js", Role: RoleDevServer, HTTPOpenable: true},
		},
		{
			name: "uvicorn",
			l:    DatabaseListener{Process: "python3", Port: "8000", Args: []string{"python3", "-m", "uvicorn", "main:app", "--reload"}},
			want: DevServerInfo{Kind: "Uvicorn", Role: RoleDevServer, HTTPOpenable: true},
		},
		{
			name: "postgres",
			l:    DatabaseListener{Process: "postgres", Port: "5432"},
			want: DevServerInfo{Kind: "PostgreSQL", Role: RoleDatabase, HTTPOpenable: false},
		},
		{
			name: "airplay",
			l:    DatabaseListener{Process: "ControlCenter", Port: "5000", Path: "/System/Library/CoreServices/ControlCenter.app/Contents/MacOS/ControlCenter"},
			want: DevServerInfo{Kind: "AirPlay", Role: RoleSystem, HTTPOpenable: false},
		},
		{
			name: "ide_helper",
			l:    DatabaseListener{Process: "Code Helper", Port: "41234", Path: "/Applications/Visual Studio Code.app/Contents/Frameworks/Code Helper.app/Contents/MacOS/Code Helper"},
			want: DevServerInfo{Kind: "", Role: RoleIDE, HTTPOpenable: false},
		},
		{
			name: "raycast_backend",
			l:    DatabaseListener{Process: "node", Port: "7265", Args: []string{"Raycast", "Backend"}},
			want: DevServerInfo{Kind: "", Role: RoleIDE, HTTPOpenable: false},
		},
		{
			name: "kubectl_forward",
			l:    DatabaseListener{Process: "kubectl", Port: "8080", Args: []string{"kubectl", "port-forward", "pod/api", "8080:8080"}},
			want: DevServerInfo{Kind: "kubectl", Role: RoleKubeForward, HTTPOpenable: true},
		},
		{
			name: "kubectl_forward_db",
			l:    DatabaseListener{Process: "kubectl", Port: "5432", Args: []string{"kubectl", "port-forward", "svc/db", "5432:5432"}},
			want: DevServerInfo{Kind: "kubectl", Role: RoleKubeForward, HTTPOpenable: false},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyDevServer(tc.l); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
