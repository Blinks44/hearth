package ops

import (
	"encoding/json"
	"testing"
)

func TestJSONSet(t *testing.T) {
	const cfg = `{
  "theme": "dark",
  "mcpServers": {
    "github": {
      "command": "npx"
    }
  }
}
`
	tests := []struct {
		name, text string
		path       []string
		raw        string
		want       string
	}{
		{"a new member at the end", cfg, []string{"mcpServers", "linear"}, `{"url":"https://mcp.linear.app/mcp"}`, `{
  "theme": "dark",
  "mcpServers": {
    "github": {
      "command": "npx"
    },
    "linear": {
      "url": "https://mcp.linear.app/mcp"
    }
  }
}
`},
		{"a member in place", cfg, []string{"mcpServers", "github", "command"}, `"docker"`, `{
  "theme": "dark",
  "mcpServers": {
    "github": {
      "command": "docker"
    }
  }
}
`},
		{"the objects on the way", "{\n  \"theme\": \"dark\"\n}\n", []string{"mcpServers", "github"}, `{"command":"npx"}`, `{
  "theme": "dark",
  "mcpServers": {
    "github": {
      "command": "npx"
    }
  }
}
`},
		{"an empty file's object", "{}\n", []string{"mcpServers", "github"}, `{"command":"npx"}`, `{
  "mcpServers": {
    "github": {
      "command": "npx"
    }
  }
}
`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := JSONSet(tt.text, tt.path, json.RawMessage(tt.raw))
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got\n%s\nwant\n%s", got, tt.want)
			}
			if !json.Valid([]byte(got)) {
				t.Error("not valid JSON")
			}
		})
	}
}

func TestJSONDelete(t *testing.T) {
	const cfg = `{
  "mcpServers": {
    "a": {"command": "x"},
    "b": {"command": "y"},
    "c": {"command": "z"}
  },
  "theme": "dark"
}`
	tests := []struct {
		name string
		path []string
		want string
	}{
		{"the first", []string{"mcpServers", "a"}, `{
  "mcpServers": {
    "b": {"command": "y"},
    "c": {"command": "z"}
  },
  "theme": "dark"
}`},
		{"the last", []string{"mcpServers", "c"}, `{
  "mcpServers": {
    "a": {"command": "x"},
    "b": {"command": "y"}
  },
  "theme": "dark"
}`},
		{"one not there", []string{"mcpServers", "d"}, cfg},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := JSONDelete(cfg, tt.path)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
	got, err := JSONDelete("{\n  \"mcpServers\": {\n    \"a\": {}\n  }\n}", []string{"mcpServers", "a"})
	if err != nil || got != "{\n  \"mcpServers\": {}\n}" {
		t.Errorf("the only member: %q, %v", got, err)
	}
}
