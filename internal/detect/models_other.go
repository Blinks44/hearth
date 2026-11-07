//go:build !linux

package detect

// ollamaServiceDirs are where an Ollama service keeps its models; outside
// Linux it runs as the user.
var ollamaServiceDirs []string
