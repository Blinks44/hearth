package projects

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	terraformMaxFiles = 50
	terraformMaxBytes = 256 * 1024
)

// readTerraform reads top-level *.tf and *.tofu files statically for
// terraform { required_version = "..." } blocks. It never executes HCL,
// Terraform, or OpenTofu.
func readTerraform(p *Project) {
	entries, err := os.ReadDir(p.Dir)
	if err != nil {
		return
	}
	var tfFiles, tofuFiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		switch filepath.Ext(name) {
		case ".tf":
			tfFiles = append(tfFiles, name)
		case ".tofu":
			tofuFiles = append(tofuFiles, name)
		}
	}
	if len(tfFiles) == 0 && len(tofuFiles) == 0 {
		return
	}

	hasTerraform := slices.ContainsFunc(p.Requirements, func(r Requirement) bool { return r.Tool == "terraform" })
	hasTofu := slices.ContainsFunc(p.Requirements, func(r Requirement) bool { return r.Tool == "tofu" })
	// OpenTofu reads both .tf and .tofu files; Terraform ignores .tofu.
	// If the project already declares tofu (or has .tofu files) and has no
	// explicit terraform pin, .tf constraints belong to tofu.
	tfTool := "terraform"
	if (len(tofuFiles) > 0 || hasTofu) && !hasTerraform {
		tfTool = "tofu"
	}

	all := make([]string, 0, len(tfFiles)+len(tofuFiles))
	all = append(all, tfFiles...)
	all = append(all, tofuFiles...)
	slices.Sort(all)

	var foundReq bool
	reads := 0
	for _, name := range all {
		if reads >= terraformMaxFiles {
			break
		}
		path := filepath.Join(p.Dir, name)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Size() > terraformMaxBytes {
			continue
		}
		reads++
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		wants := parseTerraformRequiredVersion(string(data))
		if len(wants) == 0 {
			continue
		}
		tool := tfTool
		if filepath.Ext(name) == ".tofu" {
			tool = "tofu"
		}
		if !slices.Contains(p.Files, name) {
			p.Files = append(p.Files, name)
		}
		for _, want := range wants {
			p.Requirements = append(p.Requirements, Requirement{
				Tool: tool,
				Want: want,
				File: name,
				Key:  "terraform.required_version",
			})
			foundReq = true
		}
	}

	// If there are .tf / .tofu files without required_version and no other
	// file already declared the tool, still record that the project uses it.
	if !foundReq {
		if len(tofuFiles) > 0 && !hasTofu {
			first := tofuFiles[0]
			if !slices.Contains(p.Files, first) {
				p.Files = append(p.Files, first)
			}
			p.Requirements = append(p.Requirements, Requirement{Tool: "tofu", File: first})
		} else if len(tfFiles) > 0 && !hasTerraform && !hasTofu {
			first := tfFiles[0]
			if !slices.Contains(p.Files, first) {
				p.Files = append(p.Files, first)
			}
			p.Requirements = append(p.Requirements, Requirement{Tool: tfTool, File: first})
		}
	}
}

type hclTokKind int

const (
	hclIdent hclTokKind = iota
	hclString
	hclLBrace
	hclRBrace
	hclEq
	hclOther
)

type hclTok struct {
	kind hclTokKind
	val  string
}

// parseTerraformRequiredVersion extracts literal string values of
// required_version inside top-level terraform { ... } blocks.
func parseTerraformRequiredVersion(src string) []string {
	toks := tokenizeHCL(src)
	var out []string
	depth := 0
	inTerraform := false
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch t.kind {
		case hclLBrace:
			depth++
		case hclRBrace:
			if depth == 1 {
				inTerraform = false
			}
			if depth > 0 {
				depth--
			}
		case hclIdent:
			if depth == 0 && t.val == "terraform" && i+1 < len(toks) && toks[i+1].kind == hclLBrace {
				inTerraform = true
				depth = 1
				i++ // consume '{'
				continue
			}
			if inTerraform && depth == 1 && t.val == "required_version" &&
				i+2 < len(toks) && toks[i+1].kind == hclEq && toks[i+2].kind == hclString {
				v := strings.TrimSpace(toks[i+2].val)
				if v != "" && !strings.Contains(v, "${") {
					out = append(out, v)
				}
				i += 2
			}
		}
	}
	return out
}

func tokenizeHCL(s string) []hclTok {
	var toks []hclTok
	n := len(s)
	for i := 0; i < n; {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			i++
			continue
		}
		// Line comments: # or //
		if c == '#' || (c == '/' && i+1 < n && s[i+1] == '/') {
			for i < n && s[i] != '\n' {
				i++
			}
			continue
		}
		// Block comments: /* ... */
		if c == '/' && i+1 < n && s[i+1] == '*' {
			i += 2
			for i+1 < n && (s[i] != '*' || s[i+1] != '/') {
				i++
			}
			if i+1 < n {
				i += 2
			} else {
				i = n
			}
			continue
		}
		// Heredoc: <<EOF or <<-EOF
		if c == '<' && i+1 < n && s[i+1] == '<' {
			i += 2
			if i < n && s[i] == '-' {
				i++
			}
			start := i
			for i < n && isIdentPart(s[i]) {
				i++
			}
			marker := s[start:i]
			for i < n && s[i] != '\n' {
				i++
			}
			if i < n {
				i++
			}
			if marker != "" {
				for i < n {
					lineStart := i
					for i < n && s[i] != '\n' {
						i++
					}
					line := strings.TrimSpace(strings.TrimSuffix(s[lineStart:i], "\r"))
					if i < n {
						i++
					}
					if line == marker {
						break
					}
				}
			}
			toks = append(toks, hclTok{kind: hclOther})
			continue
		}
		// Double-quoted string
		if c == '"' {
			i++
			var b strings.Builder
			for i < n && s[i] != '"' {
				if s[i] == '\\' && i+1 < n {
					i++
					switch s[i] {
					case 'n':
						b.WriteByte('\n')
					case 't':
						b.WriteByte('\t')
					case '"', '\\':
						b.WriteByte(s[i])
					default:
						b.WriteByte('\\')
						b.WriteByte(s[i])
					}
					i++
					continue
				}
				b.WriteByte(s[i])
				i++
			}
			if i < n {
				i++ // closing quote
			}
			toks = append(toks, hclTok{kind: hclString, val: b.String()})
			continue
		}
		if c == '{' {
			toks = append(toks, hclTok{kind: hclLBrace})
			i++
			continue
		}
		if c == '}' {
			toks = append(toks, hclTok{kind: hclRBrace})
			i++
			continue
		}
		if c == '=' {
			toks = append(toks, hclTok{kind: hclEq})
			i++
			continue
		}
		if isIdentStart(c) {
			start := i
			for i < n && isIdentPart(s[i]) {
				i++
			}
			toks = append(toks, hclTok{kind: hclIdent, val: s[start:i]})
			continue
		}
		toks = append(toks, hclTok{kind: hclOther})
		i++
	}
	return toks
}

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9') || c == '-'
}
