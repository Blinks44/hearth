package detect

// ollamaServiceDirs are where Ollama's Linux service keeps its models: its
// install script runs it as the user ollama, at home in /usr/share/ollama.
var ollamaServiceDirs = []string{"/usr/share/ollama/.ollama/models"}
