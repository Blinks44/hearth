package ops

import (
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
)

// ollamaServerStep starts or stops the macOS Ollama desktop app when Ollama is
// not managed by Homebrew services.
func ollamaServerStep(action ServiceAction, _ inventory.ModelApp) (Step, []Note, bool) {
	if action == ServiceStart {
		return Step{Argv: []string{"open", "-a", "Ollama"}, Timeout: 30 * time.Second}, nil, true
	}
	return Step{Argv: []string{"osascript", "-e", `quit app "Ollama"`}, Timeout: 30 * time.Second}, nil, true
}
