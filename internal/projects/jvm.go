package projects

import (
	"encoding/xml"
	"regexp"
	"strconv"
	"strings"
)

// readPomXML reads Java version properties from Maven's pom.xml statically.
func readPomXML(data []byte) ([]Requirement, error) {
	var doc struct {
		Properties struct {
			Entries []struct {
				XMLName xml.Name
				Value   string `xml:",chardata"`
			} `xml:",any"`
		} `xml:"properties"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	props := make(map[string]string, len(doc.Properties.Entries))
	for _, e := range doc.Properties.Entries {
		props[e.XMLName.Local] = strings.TrimSpace(e.Value)
	}
	for _, key := range []string{"maven.compiler.release", "java.version", "maven.compiler.source", "maven.compiler.target"} {
		if v := normalizeJavaVersion(props[key]); v != "" {
			return []Requirement{{Tool: "java", Want: v, Key: "properties." + key}}, nil
		}
	}
	return []Requirement{{Tool: "java"}}, nil
}

var (
	gradleToolchainOf   = regexp.MustCompile(`JavaLanguageVersion\.of\(\s*["']?([0-9]+(?:\.[0-9]+)?)["']?\s*\)`)
	gradleJVMToolchain  = regexp.MustCompile(`jvmToolchain\(\s*["']?([0-9]+(?:\.[0-9]+)?)["']?\s*\)`)
	gradleCompatPattern = regexp.MustCompile(`\b(sourceCompatibility|targetCompatibility)\s*(?:=|\s)\s*(?:JavaVersion\.)?(?:VERSION_([0-9]+(?:_[0-9]+)?)|["']([0-9]+(?:\.[0-9]+)?)["']|([0-9]+(?:\.[0-9]+)?))`)
)

// readGradle reads literal Java version declarations from build.gradle and
// build.gradle.kts statically without executing Groovy or Kotlin.
func readGradle(data []byte) ([]Requirement, error) {
	src := string(stripGradleComments(data))
	if m := gradleToolchainOf.FindStringSubmatch(src); m != nil {
		if v := normalizeJavaVersion(m[1]); v != "" {
			return []Requirement{{Tool: "java", Want: v, Key: "java.toolchain"}}, nil
		}
	}
	if m := gradleJVMToolchain.FindStringSubmatch(src); m != nil {
		if v := normalizeJavaVersion(m[1]); v != "" {
			return []Requirement{{Tool: "java", Want: v, Key: "jvmToolchain"}}, nil
		}
	}
	for _, m := range gradleCompatPattern.FindAllStringSubmatch(src, -1) {
		raw := m[2]
		if raw != "" {
			raw = strings.ReplaceAll(raw, "_", ".")
		} else if m[3] != "" {
			raw = m[3]
		} else {
			raw = m[4]
		}
		if v := normalizeJavaVersion(raw); v != "" {
			return []Requirement{{Tool: "java", Want: v, Key: m[1]}}, nil
		}
	}
	return []Requirement{{Tool: "java"}}, nil
}

// stripGradleComments replaces // and /* ... */ comments outside single- and
// double-quoted string literals with spaces.
func stripGradleComments(data []byte) []byte {
	out := make([]byte, len(data))
	copy(out, data)
	var quote byte
	escaped := false
	for i := 0; i < len(out); i++ {
		b := out[i]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == quote {
				quote = 0
			}
			continue
		}
		if b == '"' || b == '\'' {
			quote = b
			continue
		}
		if b != '/' || i+1 >= len(out) {
			continue
		}
		switch out[i+1] {
		case '/':
			for i < len(out) && out[i] != '\n' {
				out[i] = ' '
				i++
			}
		case '*':
			out[i], out[i+1] = ' ', ' '
			i += 2
			for i+1 < len(out) && (out[i] != '*' || out[i+1] != '/') {
				if out[i] != '\n' {
					out[i] = ' '
				}
				i++
			}
			if i+1 < len(out) {
				out[i], out[i+1] = ' ', ' '
				i++
			} else {
				return out
			}
		}
	}
	return out
}

// normalizeJavaVersion converts Maven/Gradle literal Java versions ("1.8", "21")
// into the major version Hearth uses ("8", "21"); returns "" for placeholders
// or non-numeric values.
func normalizeJavaVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "$") {
		return ""
	}
	if after, ok := strings.CutPrefix(raw, "1."); ok {
		raw = after
	}
	if major, _, ok := strings.Cut(raw, "."); ok {
		raw = major
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}
