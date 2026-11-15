package mcpserver

import "github.com/Blinks44/hearth/internal/issues"

// Explanations are fixed text: never splice settings or command output into them.
func explain(kind issues.Kind) string {
	switch kind {
	case issues.Duplicate:
		return "Multiple installed copies are on PATH; the first matching command normally wins."
	case issues.GUIDiffers:
		return "Applications and the terminal resolve this tool differently because their environments differ."
	case issues.ProjectMissing:
		return "A project requires a tool version not found in the snapshot."
	case issues.ProjectTerminal:
		return "A matching version is installed, but project resolution selects another version."
	case issues.Unsupported:
		return "An installed version is outside its published support window."
	case issues.PathCurrentDir:
		return "An empty or relative PATH entry makes command resolution depend on the current directory."
	case issues.PathMissing:
		return "A PATH entry refers to a directory that does not exist."
	case issues.SkillCopies:
		return "Identical skill folders are copied to multiple locations; later edits can diverge."
	case issues.SkillDiffers:
		return "Skill folders with the same name have different contents."
	case issues.MCPMissing:
		return "A configured MCP server's command was not found in the terminal environment."
	case issues.PlainSecret:
		return "An MCP configuration contains a credential in plain text. Values and launch details are intentionally omitted."
	case issues.SettingsSecret:
		return "Agent settings contain a credential in plain text. The value is intentionally omitted."
	case issues.AgentUnasked:
		return "An agent setting allows arbitrary commands without confirmation; review the permissions locally."
	case issues.StubOnPath:
		return "PATH contains a system placeholder instead of a working development tool."
	case issues.ExtraVersion:
		return "An installed copy is not on PATH, is not selected by its manager, and is not required by a registered project."
	case issues.SlowStartup:
		return "The terminal shell takes at least one second to start."
	case issues.DockerUnavailable:
		return "The selected local container engine could not be reached."
	case issues.DockerDenied:
		return "The current user cannot access the local container engine socket."
	case issues.DockerSignIn:
		return "The container engine group membership may require signing out and back in."
	case issues.DatabasePortConflict:
		return "A database server's port is already occupied by another listener."
	case issues.InstructionDiffers:
		return "Root AGENTS.md and CLAUDE.md differ; compare them before sharing instructions."
	}
	return "Hearth detected a configuration issue; inspect it in the desktop application."
}
