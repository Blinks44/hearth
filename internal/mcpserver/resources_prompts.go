package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Blinks44/hearth/internal/proposals"
)

// SummaryResult is a compact top-level overview served via hearth://summary.
type SummaryResult struct {
	Metadata
	ToolsCount      int    `json:"tools_count"`
	ProjectsCount   int    `json:"projects_count"`
	IssuesWarning   int    `json:"issues_warning"`
	IssuesInfo      int    `json:"issues_info"`
	AgentsFound     int    `json:"agents_found"`
	MCPServersCount int    `json:"mcp_servers_count"`
	DockerContext   string `json:"docker_context,omitempty"`
	KubeContext     string `json:"kube_context,omitempty"`
	DatabasesCount  int    `json:"databases_count"`
	ListenersCount  int    `json:"listeners_count"`
}

// ProjectPrepareQuery requests the Hearth desktop project preparation modal.
type ProjectPrepareQuery struct {
	Project string `json:"project" jsonschema:"Absolute path of a registered project from get_projects."`
}

// StopProcessQuery requests a desktop confirmation plan to stop a listening process.
type StopProcessQuery struct {
	Port string `json:"port" jsonschema:"Listening TCP port reported by get_ports_and_databases."`
	PID  string `json:"pid" jsonschema:"Process ID reported by get_ports_and_databases."`
}

// ServiceActionQuery requests a desktop confirmation plan for a local database service.
type ServiceActionQuery struct {
	Service string `json:"service" jsonschema:"Service identifier or name reported by get_ports_and_databases."`
	Action  string `json:"action" jsonschema:"One of: start, stop, restart."`
}

// SkillActionQuery requests a desktop confirmation plan for a skill management action.
type SkillActionQuery struct {
	Action  string `json:"action" jsonschema:"One of: link_all, remove, remove_broken, merge_duplicates."`
	Skill   string `json:"skill,omitempty" jsonschema:"Skill name from get_skills (required for link_all, remove, remove_broken; optional for merge_duplicates)."`
	Project string `json:"project,omitempty" jsonschema:"Optional registered project path when targeting a project skill."`
}

// ToolInstallQuery requests a desktop confirmation plan to install a tool or version.
type ToolInstallQuery struct {
	Tool    string `json:"tool" jsonschema:"Hearth tool ID such as node, python, go, rust, java, php, ruby, dotnet, or bun."`
	Version string `json:"version,omitempty" jsonschema:"Optional specific version to install via an installed version manager."`
}

// CleanupProposalQuery requests a desktop confirmation plan for reclaiming disk space.
type CleanupProposalQuery struct {
	Kinds []string `json:"kinds,omitempty" jsonschema:"Optional cleanup candidate kinds from get_cleanup; omit to propose default selected candidates."`
}

func registerExtraProposals(s *mcp.Server, propose []func(context.Context, proposals.Request) error) {
	dispatch := func(ctx context.Context, req proposals.Request, invalidMsg string) (*mcp.CallToolResult, ProposalResult, error) {
		if !req.Valid() {
			return nil, ProposalResult{}, errors.New(invalidMsg)
		}
		if len(propose) == 0 {
			return nil, ProposalResult{}, proposals.ErrNoWindow
		}
		if err := propose[0](ctx, req); err != nil {
			return nil, ProposalResult{}, err
		}
		return nil, ProposalResult{
			Status:  "requested",
			Message: "Review the plan in the Hearth desktop window. Nothing has been executed; confirm there to proceed.",
		}, nil
	}

	propTool := func(name, description string) *mcp.Tool {
		return &mcp.Tool{
			Name:        name,
			Description: description,
			Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
		}
	}

	mcp.AddTool(s, propTool("propose_project_prepare", "Request the Hearth desktop window to prepare a registered project's missing runtimes or tools. Returns requested, never executed; a human must confirm in Hearth."), func(ctx context.Context, _ *mcp.CallToolRequest, q ProjectPrepareQuery) (*mcp.CallToolResult, ProposalResult, error) {
		return dispatch(ctx, proposals.Request{
			Kind:    proposals.KindProjectPrepare,
			Project: strings.TrimSpace(q.Project),
		}, "invalid project path; pass an absolute path from get_projects")
	})

	mcp.AddTool(s, propTool("propose_stop_process", "Request a confirmation plan in Hearth to stop a user-owned listening TCP process. Returns requested, never executed; a human must confirm in Hearth."), func(ctx context.Context, _ *mcp.CallToolRequest, q StopProcessQuery) (*mcp.CallToolResult, ProposalResult, error) {
		return dispatch(ctx, proposals.Request{
			Kind: proposals.KindStopProcess,
			Port: strings.TrimSpace(q.Port),
			PID:  strings.TrimSpace(q.PID),
		}, "invalid port or PID; call get_ports_and_databases first")
	})

	mcp.AddTool(s, propTool("propose_service_action", "Request a confirmation plan in Hearth to start, stop, or restart a local database service. Returns requested, never executed; a human must confirm in Hearth."), func(ctx context.Context, _ *mcp.CallToolRequest, q ServiceActionQuery) (*mcp.CallToolResult, ProposalResult, error) {
		return dispatch(ctx, proposals.Request{
			Kind:    proposals.KindServiceAction,
			Service: strings.TrimSpace(q.Service),
			Action:  strings.TrimSpace(q.Action),
		}, "invalid service or action; use start, stop, or restart for a service from get_ports_and_databases")
	})

	mcp.AddTool(s, propTool("propose_skill_action", "Request a confirmation plan in Hearth to share (link_all), deduplicate (merge_duplicates), or remove a skill. Returns requested, never executed; a human must confirm in Hearth."), func(ctx context.Context, _ *mcp.CallToolRequest, q SkillActionQuery) (*mcp.CallToolResult, ProposalResult, error) {
		return dispatch(ctx, proposals.Request{
			Kind:    proposals.KindSkillAction,
			Action:  strings.TrimSpace(q.Action),
			Skill:   strings.TrimSpace(q.Skill),
			Project: strings.TrimSpace(q.Project),
		}, "invalid skill action; use link_all, remove, remove_broken, or merge_duplicates")
	})

	mcp.AddTool(s, propTool("propose_tool_install", "Request a confirmation plan in Hearth to install a development tool or specific runtime version. Returns requested, never executed; a human must confirm in Hearth."), func(ctx context.Context, _ *mcp.CallToolRequest, q ToolInstallQuery) (*mcp.CallToolResult, ProposalResult, error) {
		return dispatch(ctx, proposals.Request{
			Kind:    proposals.KindToolInstall,
			Tool:    strings.TrimSpace(q.Tool),
			Version: strings.TrimSpace(q.Version),
		}, "invalid tool ID or version")
	})

	mcp.AddTool(s, propTool("propose_cleanup", "Request a confirmation plan in Hearth to reclaim disk space for selected cleanup candidates. Returns requested, never executed; a human must confirm in Hearth."), func(ctx context.Context, _ *mcp.CallToolRequest, q CleanupProposalQuery) (*mcp.CallToolResult, ProposalResult, error) {
		return dispatch(ctx, proposals.Request{
			Kind:  proposals.KindCleanup,
			Kinds: q.Kinds,
		}, "invalid cleanup kinds; call get_cleanup first")
	})
}

func registerResourcesAndPrompts(s *mcp.Server, source Source) {
	jsonResource := func(uri string, payload any) (*mcp.ReadResourceResult, error) {
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode resource: %w", err)
		}
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      uri,
				MIMEType: "application/json",
				Text:     string(data),
			}},
		}, nil
	}

	s.AddResource(&mcp.Resource{
		URI:         "hearth://summary",
		Name:        "Hearth Environment Summary",
		Description: "Compact JSON overview of installed tools, registered projects, issue counts, active Docker/Kubernetes contexts, databases, and ports.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		st, err := source.Get(ctx, false)
		if err != nil {
			return nil, safeError(err)
		}
		return jsonResource(req.Params.URI, buildSummary(st))
	})

	s.AddResource(&mcp.Resource{
		URI:         "hearth://projects",
		Name:        "Registered Projects",
		Description: "Registered projects, version requirements, and snapshot-based resolution status.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		st, err := source.Get(ctx, false)
		if err != nil {
			return nil, safeError(err)
		}
		return jsonResource(req.Params.URI, buildProjects(st, ""))
	})

	s.AddResource(&mcp.Resource{
		URI:         "hearth://issues",
		Name:        "Environment Issues",
		Description: "Actionable environment issues computed from the latest Hearth snapshot.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		st, err := source.Get(ctx, false)
		if err != nil {
			return nil, safeError(err)
		}
		return jsonResource(req.Params.URI, buildIssues(st))
	})

	s.AddResource(&mcp.Resource{
		URI:         "hearth://ports",
		Name:        "Ports and Databases",
		Description: "Local database servers, DB clients, and listening TCP ports with static dev-server classification.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		st, err := source.Get(ctx, false)
		if err != nil {
			return nil, safeError(err)
		}
		return jsonResource(req.Params.URI, buildPortsAndDatabases(st, "", ""))
	})

	s.AddPrompt(&mcp.Prompt{
		Name:        "diagnose-project",
		Title:       "Diagnose Project Environment",
		Description: "Inspect a registered project's missing runtimes, version mismatches, Docker requirements, and ports using Hearth read-only tools.",
		Arguments: []*mcp.PromptArgument{{
			Name:        "path",
			Title:       "Project path",
			Description: "Absolute path of a project registered in Hearth.",
			Required:    true,
		}},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		path := strings.TrimSpace(req.Params.Arguments["path"])
		if path == "" {
			return nil, errors.New("path argument is required")
		}
		text := fmt.Sprintf(
			"Diagnose the local development environment for project %q using Hearth MCP tools:\n"+
				"1. Call get_projects(path=%q) to inspect required tool versions and unmet checks.\n"+
				"2. Call get_issues() and filter for issues referencing this project or its required tools.\n"+
				"3. Call get_ports_and_databases(project=%q) and get_docker() if the project uses Docker or databases.\n"+
				"4. Summarize root causes clearly. If automatic fixes are available, offer to call propose_project_prepare or propose_fix (nothing executes until the user confirms in the Hearth window).",
			path, path, path,
		)
		return &mcp.GetPromptResult{
			Description: "Project environment diagnosis workflow",
			Messages: []*mcp.PromptMessage{{
				Role:    "user",
				Content: &mcp.TextContent{Text: text},
			}},
		}, nil
	})

	s.AddPrompt(&mcp.Prompt{
		Name:        "troubleshoot-port",
		Title:       "Troubleshoot Listening Port",
		Description: "Identify what process, Docker container, or database is holding a local TCP port and recommend safe next steps.",
		Arguments: []*mcp.PromptArgument{{
			Name:        "port",
			Title:       "TCP Port",
			Description: "Local TCP port number (for example 3000 or 5432).",
			Required:    true,
		}},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		port := strings.TrimSpace(req.Params.Arguments["port"])
		if port == "" {
			return nil, errors.New("port argument is required")
		}
		text := fmt.Sprintf(
			"Investigate what is using local TCP port %s using Hearth MCP tools:\n"+
				"1. Call get_ports_and_databases(port=%q) to check matching listeners and local database servers.\n"+
				"2. Call get_docker() to see whether a Docker container publishes port %s.\n"+
				"3. Explain clearly what owns the port (dev server, database service, Docker container, or other process) and whether it belongs to a registered project.\n"+
				"4. Only if the user asks to free or restart it, call propose_stop_process or propose_service_action so the user can review and confirm the plan in Hearth.",
			port, port, port,
		)
		return &mcp.GetPromptResult{
			Description: "Port conflict troubleshooting workflow",
			Messages: []*mcp.PromptMessage{{
				Role:    "user",
				Content: &mcp.TextContent{Text: text},
			}},
		}, nil
	})

	s.AddPrompt(&mcp.Prompt{
		Name:        "audit-ai-setup",
		Title:       "Audit AI Coding Setup",
		Description: "Review installed AI coding agents, MCP servers (including plaintext secret warnings), skills duplication, instruction files, local models, and usage.",
	}, func(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		text := "Audit the local AI coding agent setup using Hearth MCP read-only tools:\n" +
			"1. Call get_agents() to inspect installed agents and MCP servers (flag missing commands or plaintext_secret_detected).\n" +
			"2. Call get_skills() and get_instructions() to check for broken symlinks, unshared duplicates, or out-of-sync global/project instruction files.\n" +
			"3. Call get_models() and get_usage(days=30) for local model disk usage and recent token/cost breakdown.\n" +
			"4. Call get_issues() to cross-check AI-related warnings, then present prioritized findings. Never ask the user to paste secret values; suggest Hearth proposals (propose_fix, propose_skill_action, propose_cleanup) for confirmed fixes."
		return &mcp.GetPromptResult{
			Description: "AI coding agent setup audit workflow",
			Messages: []*mcp.PromptMessage{{
				Role:    "user",
				Content: &mcp.TextContent{Text: text},
			}},
		}, nil
	})
}

func buildSummary(st State) SummaryResult {
	iss := buildIssues(st)
	warnCount, infoCount := 0, 0
	for _, is := range iss.Issues {
		if is.Severity == "warning" {
			warnCount++
		} else {
			infoCount++
		}
	}
	agentsFound := 0
	for _, a := range st.Snapshot.Agents {
		if a.Found() {
			agentsFound++
		}
	}
	mcpCount := len(st.Snapshot.MCP)
	for _, p := range st.Projects {
		mcpCount += len(p.MCP)
	}
	dk := buildDocker(st)
	kb := buildKubernetes(st)
	pd := buildPortsAndDatabases(st, "", "")
	return SummaryResult{
		Metadata:        metadata(st),
		ToolsCount:      len(tools(st, "").Tools),
		ProjectsCount:   len(st.Projects),
		IssuesWarning:   warnCount,
		IssuesInfo:      infoCount,
		AgentsFound:     agentsFound,
		MCPServersCount: mcpCount,
		DockerContext:   dk.SelectedContext,
		KubeContext:     kb.SelectedContext,
		DatabasesCount:  len(pd.Servers),
		ListenersCount:  len(pd.Listeners),
	}
}
