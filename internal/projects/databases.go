package projects

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/Blinks44/hearth/internal/inventory"
)

// DatabaseReference keeps only a local database's kind and literal host ports.
// Connection strings, credentials and container environment never leave the reader.
type DatabaseReference struct {
	Kind, Service, File string
	Ports               []string
}

func readDatabases(p *Project) {
	seen := map[string]bool{}
	var composeFiles []string
	for _, file := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
		seen[file] = true
		composeFiles = append(composeFiles, file)
	}
	for _, file := range p.Files {
		if IsComposeFile(file) && !seen[file] {
			seen[file] = true
			composeFiles = append(composeFiles, file)
		}
	}
	for _, file := range composeFiles {
		full := filepath.Join(p.Dir, file)
		info, err := os.Stat(full)
		if os.IsNotExist(err) || (err == nil && info.IsDir()) {
			continue
		}
		if err != nil {
			p.Errs = append(p.Errs, fmt.Errorf("read %s: %w", file, err))
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			p.Errs = append(p.Errs, fmt.Errorf("read %s: %w", file, err))
			continue
		}
		refs, err := composeDatabases(data)
		if err != nil {
			p.Errs = append(p.Errs, fmt.Errorf("read %s: invalid database configuration", file))
			continue
		}
		for _, ref := range refs {
			ref.File = file
			p.Databases = append(p.Databases, ref)
		}
	}
	envPath := filepath.Join(p.Dir, ".env")
	info, err := os.Stat(envPath)
	if os.IsNotExist(err) || (err == nil && info.IsDir()) {
		return
	}
	if err != nil {
		p.Errs = append(p.Errs, fmt.Errorf("read .env: %w", err))
		return
	}
	data, err := os.ReadFile(envPath)
	if err != nil {
		p.Errs = append(p.Errs, fmt.Errorf("read .env: %w", err))
		return
	}
	if refs := envDatabases(string(data)); len(refs) > 0 {
		p.Files = append(p.Files, ".env")
		p.Databases = append(p.Databases, refs...)
	}
}

func composeDatabases(data []byte) ([]DatabaseReference, error) {
	var cfg struct {
		Services map[string]struct {
			Image string
			Ports []any
		}
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(cfg.Services))
	for name := range cfg.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []DatabaseReference
	for _, name := range names {
		service := cfg.Services[name]
		kind := inventory.DatabaseKind(service.Image)
		if kind == "" {
			continue
		}
		ref := DatabaseReference{Kind: kind, Service: name}
		for _, raw := range service.Ports {
			var port string
			switch v := raw.(type) {
			case string:
				// Bare target ports request an ephemeral host port; do not guess it.
				v = strings.TrimSuffix(v, "/tcp")
				if strings.Contains(v, "/") {
					continue
				}
				parts := strings.Split(v, ":")
				if len(parts) > 1 {
					port = parts[len(parts)-2]
				}
			case map[string]any:
				if protocol, ok := v["protocol"]; ok && protocol != "tcp" {
					continue
				}
				if published, ok := v["published"]; ok {
					port = fmt.Sprint(published)
				}
			}
			// Compose often binds one port on 127.0.0.1 and on [::1].
			if validDatabasePort(port) && !slices.Contains(ref.Ports, port) {
				ref.Ports = append(ref.Ports, port)
			}
		}
		out = append(out, ref)
	}
	return out, nil
}

func envDatabases(data string) []DatabaseReference {
	var out []DatabaseReference
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(sc.Text()), "export "))
		if strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.HasSuffix(strings.ToUpper(strings.TrimSpace(key)), "URL") {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		// Prisma's sqlserver://localhost:1433;database=app;user=sa keeps
		// its settings after the first semicolon.
		value, _, _ = strings.Cut(value, ";")
		u, err := url.Parse(value)
		if err != nil {
			continue
		}
		kind, port := "", ""
		switch u.Scheme {
		case "postgres", "postgresql":
			kind, port = "PostgreSQL", "5432"
		case "mysql":
			kind, port = "MySQL", "3306"
		case "redis", "rediss":
			kind, port = "Redis", "6379"
		case "mongodb":
			kind, port = "MongoDB", "27017"
		case "sqlserver", "mssql":
			kind, port = "SQL Server", "1433"
		default:
			continue
		}
		host := u.Hostname()
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			continue
		}
		if u.Port() != "" {
			port = u.Port()
		}
		if !validDatabasePort(port) {
			continue
		}
		out = append(out, DatabaseReference{Kind: kind, File: ".env", Ports: []string{port}})
	}
	return out
}

func validDatabasePort(port string) bool {
	n, err := strconv.Atoi(port)
	return err == nil && n > 0 && n <= 65535
}
