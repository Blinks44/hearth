package detect

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// gitSettings are the global settings the Git page shows. Others may hold
// tokens, as url.<base>.insteadOf or http.extraHeader, and are not read.
var gitSettings = []string{"user.name", "user.email", "init.defaultbranch", "core.editor", "core.autocrlf", "pull.rebase", "commit.gpgsign", "gpg.format"}

var gitVersion = regexp.MustCompile(`\d+\.\d+(\.\d+)?`)

// Git finds Git's copies, the user's global settings and credential
// helpers, GitHub CLI and the hosts it is signed in to, and the SSH keys
// by their public halves.
func Git(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	f := newFinder("git", term)
	f.lookPath(gitExe, term, gui)
	st := inventory.ToolState{Tool: "git", Installations: f.ins}
	for i := range st.Installations {
		in := &st.Installations[i]
		if gitStub(in.Path) {
			in.Stub = true
			continue
		}
		// git version 2.39.5 (Apple Git-154); git version 2.47.1.windows.1
		in.Version = gitVersion.FindString(versionOf(ctx, r, term, in.Path))
	}
	home := sources.ManagerDirs(term).Home
	g := &inventory.GitState{}
	st.Git = g
	if home != "" {
		g.ConfigFile = filepath.Join(home, ".gitconfig")
		if xdg := filepath.Join(or(term.Get("XDG_CONFIG_HOME"), filepath.Join(home, ".config")), "git", "config"); !exists(g.ConfigFile) && exists(xdg) {
			g.ConfigFile = xdg
		}
	}
	if active := inventory.TerminalCopy(st.Installations); active >= 0 {
		git := st.Installations[active].Path
		// The global settings as git reads them, includes and all.
		res, err := r.Run(ctx, run.Cmd{Args: []string{git, "config", "--global", "--list"}, Env: term})
		switch {
		case err != nil:
			g.Err = err.Error()
		case res.ExitCode == 0:
			g.Config = gitConfig(string(res.Stdout))
		}
		// The helpers of every level: macOS's Git sets osxkeychain in its
		// own config, Git for Windows the Credential Manager in the
		// system's. Exit 1 means none.
		if res, err := r.Run(ctx, run.Cmd{Args: []string{git, "config", "--get-all", "credential.helper"}, Env: term}); err == nil && res.ExitCode == 0 {
			g.Helpers = gitHelpers(string(res.Stdout))
		}
	}
	g.GitHub = gitHub(ctx, r, term, gui, home)
	g.Keys = sshKeys(ctx, r, term, home)
	return st, ctx.Err()
}

// gitConfig picks the shown settings out of git config --list's lines of
// name=value; a later file's value wins, as git reads them.
func gitConfig(out string) []inventory.Setting {
	var settings []inventory.Setting
	for line := range strings.Lines(out) {
		name, value, _ := strings.Cut(strings.TrimRight(line, "\r\n"), "=")
		name = strings.ToLower(name)
		if !slices.Contains(gitSettings, name) {
			continue
		}
		if i := slices.IndexFunc(settings, func(s inventory.Setting) bool { return s.Name == name }); i >= 0 {
			settings[i].Value = value
		} else {
			settings = append(settings, inventory.Setting{Name: name, Value: value})
		}
	}
	return settings
}

// gitHelpers are the credential helpers' names, one per line of git
// config --get-all: a helper may be a command with arguments, and an empty
// one resets the list before it.
func gitHelpers(out string) []string {
	var helpers []string
	for line := range strings.Lines(out) {
		f := strings.Fields(line)
		if len(f) == 0 {
			helpers = nil
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(strings.Trim(f[0], `"'`)), "git-credential-"), ".exe")
		if !slices.Contains(helpers, name) {
			helpers = append(helpers, name)
		}
	}
	return helpers
}

// gitHub finds gh in the terminal and the hosts in its hosts.yml, with
// their accounts; the tokens there are never kept.
func gitHub(ctx context.Context, r run.Runner, term, gui sysenv.Env, home string) *inventory.GitHubCLI {
	f := newFinder("gh", term)
	f.lookPath(ghExe, term, gui)
	active := inventory.TerminalCopy(f.ins)
	if active < 0 {
		return nil
	}
	gh := &inventory.GitHubCLI{Path: f.ins[active].Path}
	// gh version 2.63.2 (2024-12-05)
	gh.Version = gitVersion.FindString(versionOf(ctx, r, term, gh.Path))
	var hosts map[string]struct {
		User string `yaml:"user"`
	}
	if data, err := os.ReadFile(filepath.Join(ghConfigDir(term, home), "hosts.yml")); err == nil && yaml.Unmarshal(data, &hosts) == nil {
		for host, h := range hosts {
			gh.Hosts = append(gh.Hosts, inventory.GitHubHost{Host: host, User: h.User})
		}
		slices.SortFunc(gh.Hosts, func(a, b inventory.GitHubHost) int { return strings.Compare(a.Host, b.Host) })
	}
	return gh
}

// sshKeys are the key pairs in ~/.ssh by their .pub files, the hosts
// ~/.ssh/config gives each to, and whether ssh-agent holds it.
func sshKeys(ctx context.Context, r run.Runner, env sysenv.Env, home string) []inventory.SSHKey {
	if home == "" {
		return nil
	}
	dir := filepath.Join(home, ".ssh")
	pubs, _ := filepath.Glob(filepath.Join(dir, "*.pub"))
	if len(pubs) == 0 {
		return nil
	}
	hosts := sshHosts(filepath.Join(dir, "config"), home)
	agent := map[string]bool{}
	if env.Get("SSH_AUTH_SOCK") != "" {
		// ssh-add -l lists fingerprints only: 256 SHA256:... comment (ED25519)
		if res, err := r.Run(ctx, run.Cmd{Args: []string{"ssh-add", "-l"}, Env: env}); err == nil && res.ExitCode == 0 {
			for line := range strings.Lines(string(res.Stdout)) {
				if f := strings.Fields(line); len(f) >= 2 {
					agent[f[1]] = true
				}
			}
		}
	}
	var out []inventory.SSHKey
	for _, pub := range pubs {
		raw := strings.TrimSpace(readFile(pub))
		fields := strings.Fields(raw)
		if len(fields) < 2 {
			continue
		}
		blob, err := base64.StdEncoding.DecodeString(fields[1])
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(pub), ".pub")
		k := inventory.SSHKey{Name: name, Hosts: hosts[filepath.Join(dir, name)], PublicKey: raw}
		k.Type, k.Bits = sshKeyType(fields[0], blob)
		sum := sha256.Sum256(blob)
		k.InAgent = agent["SHA256:"+base64.RawStdEncoding.EncodeToString(sum[:])]
		out = append(out, k)
	}
	return out
}

// sshKeyType names a key's type as ssh-keygen -l does, and an RSA key's
// size from its modulus in the public blob.
func sshKeyType(kind string, blob []byte) (string, int) {
	switch kind {
	case "ssh-ed25519":
		return "ED25519", 0
	case "ssh-rsa":
		// string "ssh-rsa", mpint e, mpint n
		var parts [][]byte
		for rest := blob; len(rest) >= 4 && len(parts) < 3; {
			n := binary.BigEndian.Uint32(rest)
			if uint64(len(rest)-4) < uint64(n) {
				break
			}
			parts, rest = append(parts, rest[4:4+n]), rest[4+n:]
		}
		if len(parts) == 3 {
			return "RSA", new(big.Int).SetBytes(parts[2]).BitLen()
		}
		return "RSA", 0
	case "sk-ssh-ed25519@openssh.com":
		return "ED25519-SK", 0
	}
	if strings.HasPrefix(kind, "ecdsa-") || strings.HasPrefix(kind, "sk-ecdsa-") {
		return "ECDSA", 0
	}
	return strings.ToUpper(strings.TrimPrefix(kind, "ssh-")), 0
}

// sshHosts maps each IdentityFile in the SSH config to the Host patterns
// it is given for; ~ is the home.
func sshHosts(config, home string) map[string][]string {
	out := map[string][]string{}
	f, err := os.Open(config)
	if err != nil {
		return out
	}
	defer f.Close() //nolint:errcheck // read only
	var current []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(strings.TrimSpace(sc.Text()))
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "host":
			current = fields[1:]
		case "match":
			current = nil
		case "identityfile":
			path := strings.Trim(fields[1], `"`)
			if rest, ok := strings.CutPrefix(path, "~/"); ok {
				path = filepath.Join(home, rest)
			}
			path = filepath.Clean(path)
			for _, h := range current {
				if h != "*" && !slices.Contains(out[path], h) {
					out[path] = append(out[path], h)
				}
			}
		}
	}
	return out
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
