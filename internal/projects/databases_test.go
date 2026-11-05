package projects

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectDatabases(t *testing.T) {
	dir := t.TempDir()
	compose := `services:
  pg:
    image: postgres:16
    ports: ["127.0.0.1:5433:5432", "5432", "${PORT}:5432"]
    environment:
      POSTGRES_PASSWORD: compose-secret
  cache:
    image: redis:7
    ports:
      - target: 6379
        published: "6380"
        protocol: tcp
  exporter:
    image: postgres-exporter:latest
`
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(compose), 0600); err != nil {
		t.Fatal(err)
	}
	env := "DATABASE_URL=postgres://user:env-secret@localhost:5433/db?token=secret\nREDIS_URL=redis://user:pass@remote.example:6379\nAPI_URL=https://localhost:443\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0600); err != nil {
		t.Fatal(err)
	}
	p := Read(dir)
	if len(p.Errs) > 0 || len(p.Databases) != 3 {
		t.Fatalf("%+v errs=%v", p.Databases, p.Errs)
	}
	if p.Databases[0].Service != "cache" || strings.Join(p.Databases[0].Ports, ",") != "6380" || strings.Join(p.Databases[1].Ports, ",") != "5433" {
		t.Fatal(p.Databases)
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"compose-secret", "env-secret", "remote.example", "token=secret"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("leaked %s", secret)
		}
	}
}

func TestComposeDatabasePorts(t *testing.T) {
	for _, tc := range []struct{ name, ports, want string }{{"udp", `["5432:5432/udp"]`, ""}, {"long udp", `[{target: 5432, published: 5433, protocol: udp}]`, ""}, {"range", `["5432-5433:5432-5433"]`, ""}, {"ipv6", `["[::1]:5433:5432"]`, "5433"}, {"both loopbacks", `["127.0.0.1:5433:5432", "[::1]:5433:5432"]`, "5433"}, {"ephemeral", `[5432]`, ""}} {
		t.Run(tc.name, func(t *testing.T) {
			refs, err := composeDatabases([]byte("services:\n  db:\n    image: postgres:16\n    ports: " + tc.ports))
			if err != nil || len(refs) != 1 || strings.Join(refs[0].Ports, ",") != tc.want {
				t.Fatalf("%+v %v", refs, err)
			}
		})
	}
}
