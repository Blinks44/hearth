//go:build !windows

package detect

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// sshBlob is a public key's blob: its fields as SSH strings.
func sshBlob(fields ...[]byte) []byte {
	var out []byte
	for _, f := range fields {
		out = binary.BigEndian.AppendUint32(out, uint32(len(f)))
		out = append(out, f...)
	}
	return out
}

// Git's settings come only from the ones shown, never a token in a URL;
// helpers from every level; gh's hosts without their tokens; SSH keys by
// their public halves, with the hosts the config gives them to and
// whether the agent holds them.
func TestGit(t *testing.T) {
	dir := root(t)
	bin := filepath.Join(dir, "bin")
	git := fakeExe(t, filepath.Join(bin, "git"))
	gh := fakeExe(t, filepath.Join(bin, "gh"))
	ed := sshBlob([]byte("ssh-ed25519"), make([]byte, 32))
	modulus := append([]byte{0}, make([]byte, 256)...)
	modulus[1] = 0x80 // 2048 bits
	rsa := sshBlob([]byte("ssh-rsa"), []byte{1, 0, 1}, modulus)
	ssh := filepath.Join(dir, ".ssh")
	writeFile(t, filepath.Join(ssh, "id_ed25519.pub"), "ssh-ed25519 "+base64.StdEncoding.EncodeToString(ed)+" ann@laptop\n")
	writeFile(t, filepath.Join(ssh, "id_rsa.pub"), "ssh-rsa "+base64.StdEncoding.EncodeToString(rsa)+"\n")
	writeFile(t, filepath.Join(ssh, "id_ed25519"), "PRIVATE, NEVER READ")
	writeFile(t, filepath.Join(ssh, "config"), "Host github.com gitlab.com\n  IdentityFile ~/.ssh/id_ed25519\nHost *\n  IdentityFile ~/.ssh/id_rsa\n")
	writeFile(t, filepath.Join(dir, ".gitconfig"), "")
	writeFile(t, filepath.Join(dir, ".config", "gh", "hosts.yml"), "github.com:\n    user: ann\n    oauth_token: gho_secret\n    git_protocol: ssh\n")
	sum := sha256.Sum256(ed)
	r := &run.Fake{}
	r.Stdout("git version 2.47.1\n", git, "--version")
	r.Stdout("user.name=Ann\nuser.email=ann@example.com\nurl.https://tok@github.com/.insteadof=https://github.com/\nhttp.extraheader=AUTHORIZATION: bearer x\ncore.editor=vim\ncore.editor=code --wait\n", git, "config", "--global", "--list")
	r.Stdout("osxkeychain\n\n/usr/local/bin/git-credential-manager --flag\n", git, "config", "--get-all", "credential.helper")
	r.Stdout("gh version 2.63.2 (2024-12-05)\nhttps://github.com/cli/cli/releases/tag/v2.63.2\n", gh, "--version")
	r.Stdout("256 SHA256:"+base64.RawStdEncoding.EncodeToString(sum[:])+" ann@laptop (ED25519)\n", "ssh-add", "-l")
	env := sysenv.Env{"HOME=" + dir, "PATH=" + bin, "SSH_AUTH_SOCK=/tmp/agent"}
	st, err := Git(t.Context(), r, env, sysenv.Env{"PATH=" + bin})
	if err != nil {
		t.Fatal(err)
	}
	if ins := copiesIn(st.Installations, dir); len(ins) != 1 || ins[0].Version != "2.47.1" {
		t.Fatalf("copies: %+v", st.Installations)
	}
	want := &inventory.GitState{
		ConfigFile: filepath.Join(dir, ".gitconfig"),
		Config:     []inventory.Setting{{Name: "user.name", Value: "Ann"}, {Name: "user.email", Value: "ann@example.com"}, {Name: "core.editor", Value: "code --wait"}},
		// The empty value resets the list, as git does.
		Helpers: []string{"manager"},
		GitHub:  &inventory.GitHubCLI{Path: gh, Version: "2.63.2", Hosts: []inventory.GitHubHost{{Host: "github.com", User: "ann"}}},
		Keys: []inventory.SSHKey{
			{Name: "id_ed25519", Type: "ED25519", Hosts: []string{"github.com", "gitlab.com"}, InAgent: true},
			{Name: "id_rsa", Type: "RSA", Bits: 2048},
		},
	}
	if !reflect.DeepEqual(st.Git, want) {
		t.Fatalf("got  %+v\nwant %+v", st.Git, want)
	}
}

func TestGitStubOnMacOnly(t *testing.T) {
	if gitStub("/opt/homebrew/bin/git") {
		t.Error("brew's git is a stub")
	}
}
