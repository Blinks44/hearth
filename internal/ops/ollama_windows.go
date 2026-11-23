package ops

import (
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
)

// ollamaServerStep starts or stops the Ollama server on Windows.
func ollamaServerStep(action ServiceAction, _ inventory.ModelApp) (Step, []Note, bool) {
	if action == ServiceStart {
		return Step{
			Argv:    []string{"powershell", "-NoProfile", "-Command", "Start-Process -FilePath 'ollama' -ArgumentList 'serve' -WindowStyle Hidden"},
			Timeout: 30 * time.Second,
		}, nil, true
	}
	return Step{
		Argv:    []string{"powershell", "-NoProfile", "-Command", "Stop-Process -Name 'ollama','ollama app' -ErrorAction SilentlyContinue"},
		Timeout: 30 * time.Second,
	}, nil, true
}
