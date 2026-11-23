package ops

import "github.com/Blinks44/hearth/internal/inventory"

// ollamaServerStep starts or stops the Ollama systemd unit on Linux.
func ollamaServerStep(action ServiceAction, _ inventory.ModelApp) (Step, []Note, bool) {
	step, ok := systemService(action, "ollama")
	if !ok {
		return Step{}, nil, false
	}
	return step, []Note{AdminNote()}, true
}
