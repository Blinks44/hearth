package ops

import "github.com/Blinks44/hearth/internal/inventory"

// ollamaServerStep starts or stops the Ollama systemd unit on Linux.
func ollamaServerStep(action ServiceAction, _ inventory.ModelApp) (Step, []Note, bool) {
	steps, ok := systemService(action, "ollama")
	if !ok || len(steps) == 0 {
		return Step{}, nil, false
	}
	return steps[0], []Note{AdminNote()}, true
}
