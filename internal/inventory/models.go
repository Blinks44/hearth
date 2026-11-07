package inventory

import (
	"slices"
	"strings"
	"time"
)

// ModelsState is the apps that run AI models locally and the models they
// downloaded.
type ModelsState struct {
	// Ollama and LMStudio are nil when not found.
	Ollama   *ModelApp `json:",omitempty"`
	LMStudio *ModelApp `json:",omitempty"`
}

// ModelApp is Ollama or LM Studio, as found.
type ModelApp struct {
	// Path is its command, as ollama, when on PATH; Version its own.
	Path, Version string `json:",omitempty"`
	Source        Source
	// Dir is the folder of its models; DirVar the variable that set it,
	// as OLLAMA_MODELS, when one did.
	Dir    string
	DirVar string `json:",omitempty"`
	// Port is where its server listens by default.
	Port   string
	Models []Model `json:",omitempty"`
	// Err says why the models could not be read.
	Err string `json:",omitempty"`
}

// Model is a downloaded model.
type Model struct {
	// Name is as the app names it: "llama3.2:3b" for Ollama, the folder
	// for LM Studio.
	Name string
	// Format is "GGUF" or "MLX"; Quant the quantization, as "Q4_K_M",
	// when known.
	Format string `json:",omitempty"`
	Quant  string `json:",omitempty"`
	// Path is its manifest for Ollama, its folder for LM Studio.
	Path string
	// Size is what it takes on disk; Frees what removing it frees, less
	// when it shares files with another model.
	Size, Frees int64
	// Downloaded is when it was downloaded; Hearth does not know when it
	// was last used.
	Downloaded time.Time
	// Loaded reports that the app keeps it in memory now.
	Loaded bool `json:",omitempty"`
}

// ModelMentions are the MCP servers whose arguments name the model, as
// an embedding model a memory server uses.
func ModelMentions(servers []MCPServer, m Model) []MCPServer {
	name := strings.ToLower(m.Name)
	base, _, _ := strings.Cut(name, ":")
	var out []MCPServer
	for _, s := range servers {
		if slices.ContainsFunc(s.Args, func(arg string) bool {
			for _, f := range strings.FieldsFunc(strings.ToLower(arg), func(r rune) bool { return strings.ContainsRune(" =,;\"'", r) }) {
				if f == name || f == base || strings.HasSuffix(f, "/"+base) {
					return true
				}
			}
			return false
		}) {
			out = append(out, s)
		}
	}
	return out
}
