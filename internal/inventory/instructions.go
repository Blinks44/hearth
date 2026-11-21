package inventory

// InstructionFile describes a file without keeping its contents in the cache.
type InstructionFile struct {
	Path, Target, Sum, Err string
	Exists, Link, Empty    bool
}

// GlobalInstructions compares an agent's global instruction files with the
// optional shared file (~/.agents/AGENTS.md). Active is the file the agent
// reads; State describes their relationship.
type GlobalInstructions struct {
	Shared, Native, Override InstructionFile
	Active, State            string
}
