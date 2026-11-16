package inventory

// InstructionFile describes a file without keeping its contents in the cache.
type InstructionFile struct {
	Path, Target, Sum, Err string
	Exists, Link, Empty    bool
}

// GlobalInstructions compares Codex's global files with the optional shared file.
// Active is the file Codex reads; State describes their relationship.
type GlobalInstructions struct {
	Shared, Native, Override InstructionFile
	Active, State            string
}
