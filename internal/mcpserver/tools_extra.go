package mcpserver

import (
	"cmp"
	"context"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/aiquota"
	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
)

// DockerContextInfo is a safe projection of a configured Docker context.
type DockerContextInfo struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint,omitempty"`
	Engine   string `json:"engine,omitempty"`
	Status   string `json:"status,omitempty"`
	Version  string `json:"version,omitempty"`
	Local    bool   `json:"local"`
	Denied   bool   `json:"denied,omitempty"`
}

// DockerEngineInfo lists a detected local Docker engine binary or app.
type DockerEngineInfo struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

// ColimaProfileInfo describes a local Colima VM profile.
type ColimaProfileInfo struct {
	Name        string `json:"name"`
	Status      string `json:"status,omitempty"`
	Runtime     string `json:"runtime,omitempty"`
	CPU         int    `json:"cpu,omitempty"`
	MemoryBytes int64  `json:"memory_bytes,omitempty"`
	DiskBytes   int64  `json:"disk_bytes,omitempty"`
}

// ColimaInfo summarizes Colima installation and profiles.
type ColimaInfo struct {
	Installed  bool                `json:"installed"`
	Autostart  string              `json:"autostart,omitempty"`
	Unreadable bool                `json:"unreadable,omitempty"`
	Profiles   []ColimaProfileInfo `json:"profiles"`
}

// DockerServiceInfo describes a native Linux Docker systemd unit.
type DockerServiceInfo struct {
	Unit      string `json:"unit,omitempty"`
	Status    string `json:"status,omitempty"`
	Autostart string `json:"autostart,omitempty"`
}

// DockerPortContainerInfo describes a running container publishing TCP ports.
type DockerPortContainerInfo struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Image      string   `json:"image"`
	Status     string   `json:"status,omitempty"`
	ComposeDir string   `json:"compose_dir,omitempty"`
	Ports      []string `json:"ports"`
}

// KindClusterInfo describes a local kind Kubernetes cluster running on Docker.
type KindClusterInfo struct {
	Name    string `json:"name"`
	Nodes   int    `json:"nodes"`
	Running int    `json:"running"`
	Version string `json:"version,omitempty"`
}

// DockerResult is a read-only projection of local Docker contexts, engines, and ports.
type DockerResult struct {
	Metadata
	Installed             bool                      `json:"installed"`
	SelectedContext       string                    `json:"selected_context,omitempty"`
	Override              string                    `json:"override,omitempty"`
	RunningContainers     int                       `json:"running_containers"`
	TotalContainers       int                       `json:"total_containers"`
	Contexts              []DockerContextInfo       `json:"contexts"`
	Engines               []DockerEngineInfo        `json:"engines"`
	Colima                *ColimaInfo               `json:"colima,omitempty"`
	Service               *DockerServiceInfo        `json:"service,omitempty"`
	PortContainers        []DockerPortContainerInfo `json:"port_containers"`
	KindClusters          []KindClusterInfo         `json:"kind_clusters"`
	CredentialIssuesCount int                       `json:"credential_issues_count"`
	BrokenPlugins         []string                  `json:"broken_plugins,omitempty"`
	DefaultSocketIssue    string                    `json:"default_socket_issue,omitempty"`
}

// PortsQuery optionally filters listening ports by port number or project path.
type PortsQuery struct {
	Query
	Port    string `json:"port,omitempty" jsonschema:"Optional TCP port number filter."`
	Project string `json:"project,omitempty" jsonschema:"Optional registered project absolute path filter."`
}

// DatabaseServerInfo is a safe projection of a local database service or container.
type DatabaseServerInfo struct {
	Kind      string   `json:"kind"`
	Name      string   `json:"name,omitempty"`
	Version   string   `json:"version,omitempty"`
	Source    string   `json:"source,omitempty"`
	Status    string   `json:"status,omitempty"`
	Service   string   `json:"service,omitempty"`
	Project   string   `json:"project,omitempty"`
	Ports     []string `json:"ports"`
	Autostart string   `json:"autostart,omitempty"`
}

// ListenerInfo is a safe projection of a listening TCP socket; raw command args are omitted.
type ListenerInfo struct {
	Address      string  `json:"address"`
	Port         string  `json:"port"`
	Process      string  `json:"process,omitempty"`
	PID          string  `json:"pid,omitempty"`
	Project      string  `json:"project,omitempty"`
	Role         string  `json:"role"`
	Kind         string  `json:"kind,omitempty"`
	HTTPOpenable bool    `json:"http_openable,omitempty"`
	CPUPercent   float64 `json:"cpu_percent,omitempty"`
	MemoryBytes  int64   `json:"memory_bytes,omitempty"`
	Own          bool    `json:"own,omitempty"`
}

// PortsAndDatabasesResult reports local databases, DB CLI clients, and listening TCP ports.
type PortsAndDatabasesResult struct {
	Metadata
	Servers   []DatabaseServerInfo `json:"servers"`
	Clients   []Copy               `json:"clients"`
	Listeners []ListenerInfo       `json:"listeners"`
}

// SkillsQuery optionally filters skills by agent ID or registered project path.
type SkillsQuery struct {
	Query
	Agent   string `json:"agent,omitempty" jsonschema:"Optional agent ID such as claude or codex."`
	Project string `json:"project,omitempty" jsonschema:"Optional registered project absolute path."`
}

// SkillInfo describes an installed AI skill without returning SKILL.md body contents.
type SkillInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Dir         string   `json:"dir"`
	Project     string   `json:"project,omitempty"`
	Agents      []string `json:"agents"`
	Synced      bool     `json:"synced,omitempty"`
	Broken      bool     `json:"broken,omitempty"`
	Shared      bool     `json:"shared,omitempty"`
}

// SkillsResult lists user and registered-project AI skills.
type SkillsResult struct {
	Metadata
	Skills []SkillInfo `json:"skills"`
}

// KubeFileInfo describes a kubeconfig file path and validity without reading credentials.
type KubeFileInfo struct {
	Path    string `json:"path"`
	Missing bool   `json:"missing,omitempty"`
	Invalid bool   `json:"invalid,omitempty"`
}

// KubeContextInfo describes a named kubeconfig context without tokens or server secrets.
type KubeContextInfo struct {
	Name      string `json:"name"`
	Cluster   string `json:"cluster,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	File      string `json:"file,omitempty"`
	Shadowed  bool   `json:"shadowed,omitempty"`
	Auth      string `json:"auth,omitempty"`
}

// KubernetesResult summarizes local Kubernetes CLIs, kubeconfig contexts, and kind clusters.
type KubernetesResult struct {
	Metadata
	Tools           []Tool            `json:"tools"`
	SelectedContext string            `json:"selected_context,omitempty"`
	KubeconfigSet   bool              `json:"kubeconfig_set,omitempty"`
	HelmRepos       int               `json:"helm_repos,omitempty"`
	Files           []KubeFileInfo    `json:"files"`
	Contexts        []KubeContextInfo `json:"contexts"`
	KindClusters    []KindClusterInfo `json:"kind_clusters"`
}

// CleanupCandidateInfo is a safe projection of one reclaimable disk candidate.
type CleanupCandidateInfo struct {
	Kind       string   `json:"kind"`
	ID         string   `json:"id"`
	Manager    string   `json:"manager,omitempty"`
	Project    string   `json:"project,omitempty"`
	Role       string   `json:"role,omitempty"`
	Count      int      `json:"count,omitempty"`
	FreesBytes int64    `json:"frees_bytes"`
	Known      bool     `json:"known"`
	Selected   bool     `json:"selected"`
	Reasons    []string `json:"reasons"`
}

// CleanupResult summarizes reclaimable disk space from the cached snapshot and size count.
type CleanupResult struct {
	Metadata
	SizesAvailable        bool                   `json:"sizes_available"`
	SizesTakenAt          time.Time              `json:"sizes_taken_at,omitzero"`
	TotalReclaimableBytes int64                  `json:"total_reclaimable_bytes"`
	Candidates            []CleanupCandidateInfo `json:"candidates"`
}

// UsageQuery controls the lookback window for local AI token usage aggregation.
type UsageQuery struct {
	Query
	Days int `json:"days,omitempty" jsonschema:"Number of days to include (default 30, max 365)."`
}

// UsageAgentSummary aggregates local token usage and estimated API cost per agent.
type UsageAgentSummary struct {
	Agent            string  `json:"agent"`
	Sessions         int     `json:"sessions"`
	Tokens           int64   `json:"tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

// UsageModelSummary aggregates local token usage and estimated API cost per model.
type UsageModelSummary struct {
	Model            string  `json:"model"`
	Tokens           int64   `json:"tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

// UsageDaySummary aggregates local token usage and estimated API cost per calendar day.
type UsageDaySummary struct {
	Day              string  `json:"day"`
	Tokens           int64   `json:"tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

// UsageResult reports aggregate AI token/cost statistics without session IDs or transcripts.
type UsageResult struct {
	Metadata
	Days             int                 `json:"days"`
	PricesAsOf       string              `json:"prices_as_of"`
	SessionCount     int                 `json:"session_count"`
	TotalTokens      int64               `json:"total_tokens"`
	EstimatedCostUSD float64             `json:"estimated_cost_usd"`
	ByAgent          []UsageAgentSummary `json:"by_agent"`
	ByModel          []UsageModelSummary `json:"by_model"`
	ByDay            []UsageDaySummary   `json:"by_day"`
}

// LocalModelInfo describes a downloaded Ollama or LM Studio model.
type LocalModelInfo struct {
	Name       string    `json:"name"`
	Format     string    `json:"format,omitempty"`
	Quant      string    `json:"quant,omitempty"`
	SizeBytes  int64     `json:"size_bytes"`
	FreesBytes int64     `json:"frees_bytes"`
	Downloaded time.Time `json:"downloaded,omitzero"`
	Loaded     bool      `json:"loaded,omitempty"`
}

// ModelAppInfo describes a local model runner (Ollama or LM Studio).
type ModelAppInfo struct {
	Found   bool             `json:"found"`
	Version string           `json:"version,omitempty"`
	Port    string           `json:"port,omitempty"`
	Dir     string           `json:"dir,omitempty"`
	Models  []LocalModelInfo `json:"models"`
}

// ModelsResult reports local Ollama and LM Studio runtimes and downloaded models.
type ModelsResult struct {
	Metadata
	Ollama   ModelAppInfo `json:"ollama"`
	LMStudio ModelAppInfo `json:"lm_studio"`
}

// GlobalInstructionInfo describes an agent's global instruction file status without file text.
type GlobalInstructionInfo struct {
	Agent          string `json:"agent"`
	State          string `json:"state,omitempty"`
	Active         string `json:"active,omitempty"`
	SharedPath     string `json:"shared_path,omitempty"`
	SharedExists   bool   `json:"shared_exists,omitempty"`
	NativePath     string `json:"native_path,omitempty"`
	NativeExists   bool   `json:"native_exists,omitempty"`
	NativeLink     bool   `json:"native_link,omitempty"`
	OverridePath   string `json:"override_path,omitempty"`
	OverrideExists bool   `json:"override_exists,omitempty"`
}

// ProjectInstructionFileInfo reports presence of one project instruction file without contents.
type ProjectInstructionFileInfo struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Link   bool   `json:"link,omitempty"`
}

// ProjectInstructionsInfo summarizes instruction files present in a registered project.
type ProjectInstructionsInfo struct {
	Project string                       `json:"project"`
	State   string                       `json:"state,omitempty"`
	Files   []ProjectInstructionFileInfo `json:"files"`
}

// InstructionsResult lists global and project instruction file metadata without contents.
type InstructionsResult struct {
	Metadata
	Global   []GlobalInstructionInfo   `json:"global"`
	Projects []ProjectInstructionsInfo `json:"projects"`
}

func registerExtraReadTools(s *mcp.Server, source Source, tool func(name, description string) *mcp.Tool) {
	mcp.AddTool(s, tool("get_docker", "Local Docker contexts, installed engines, Colima VM profiles, kind clusters, and port-publishing containers."), func(ctx context.Context, _ *mcp.CallToolRequest, q Query) (*mcp.CallToolResult, DockerResult, error) {
		st, err := source.Get(ctx, q.Refresh)
		if err != nil {
			return nil, DockerResult{}, safeError(err)
		}
		return nil, buildDocker(st), nil
	})
	mcp.AddTool(s, tool("get_ports_and_databases", "Local database servers, DB CLI clients, and listening TCP ports with static dev-server classification. Command arguments and credentials are omitted."), func(ctx context.Context, _ *mcp.CallToolRequest, q PortsQuery) (*mcp.CallToolResult, PortsAndDatabasesResult, error) {
		st, err := source.Get(ctx, q.Refresh)
		if err != nil {
			return nil, PortsAndDatabasesResult{}, safeError(err)
		}
		return nil, buildPortsAndDatabases(st, q.Port, q.Project), nil
	})
	mcp.AddTool(s, tool("get_skills", "Installed user and registered-project AI skills metadata without SKILL.md contents."), func(ctx context.Context, _ *mcp.CallToolRequest, q SkillsQuery) (*mcp.CallToolResult, SkillsResult, error) {
		st, err := source.Get(ctx, q.Refresh)
		if err != nil {
			return nil, SkillsResult{}, safeError(err)
		}
		return nil, buildSkills(st, q.Agent, q.Project), nil
	})
	mcp.AddTool(s, tool("get_kubernetes", "Local Kubernetes CLIs, kubeconfig files, named contexts, Helm repo count, and local kind clusters. Credentials and server endpoints are omitted."), func(ctx context.Context, _ *mcp.CallToolRequest, q Query) (*mcp.CallToolResult, KubernetesResult, error) {
		st, err := source.Get(ctx, q.Refresh)
		if err != nil {
			return nil, KubernetesResult{}, safeError(err)
		}
		return nil, buildKubernetes(st), nil
	})
	mcp.AddTool(s, tool("get_cleanup", "Reclaimable disk-space candidates (caches, unused copies, Docker data, stale project deps, old extensions, local models, and stale AI history) computed read-only."), func(ctx context.Context, _ *mcp.CallToolRequest, q Query) (*mcp.CallToolResult, CleanupResult, error) {
		st, err := source.Get(ctx, q.Refresh)
		if err != nil {
			return nil, CleanupResult{}, safeError(err)
		}
		return nil, buildCleanup(st, time.Now()), nil
	})
	mcp.AddTool(s, tool("get_usage", "Aggregated local AI token usage and estimated API cost by agent, model, and day. Prompts, session IDs, and transcripts are never returned."), func(ctx context.Context, _ *mcp.CallToolRequest, q UsageQuery) (*mcp.CallToolResult, UsageResult, error) {
		st, err := source.Get(ctx, q.Refresh)
		if err != nil {
			return nil, UsageResult{}, safeError(err)
		}
		return nil, buildUsage(st, q.Days, time.Now()), nil
	})
	mcp.AddTool(s, tool("get_models", "Local Ollama and LM Studio runtimes and downloaded models with disk sizes and loaded status."), func(ctx context.Context, _ *mcp.CallToolRequest, q Query) (*mcp.CallToolResult, ModelsResult, error) {
		st, err := source.Get(ctx, q.Refresh)
		if err != nil {
			return nil, ModelsResult{}, safeError(err)
		}
		return nil, buildModels(st), nil
	})
	mcp.AddTool(s, tool("get_instructions", "Global and registered-project AI instruction file paths and sync status. File contents are never returned."), func(ctx context.Context, _ *mcp.CallToolRequest, q Query) (*mcp.CallToolResult, InstructionsResult, error) {
		st, err := source.Get(ctx, q.Refresh)
		if err != nil {
			return nil, InstructionsResult{}, safeError(err)
		}
		return nil, buildInstructions(st), nil
	})
	mcp.AddTool(s, tool("get_ai_quotas", "Active AI subscription quotas, remaining percentages, reset times, and consumption pace. Tokens, cookies, API keys, and emails are never returned."), func(ctx context.Context, _ *mcp.CallToolRequest, q Query) (*mcp.CallToolResult, AIQuotasResult, error) {
		st, err := source.Get(ctx, q.Refresh)
		if err != nil {
			return nil, AIQuotasResult{}, safeError(err)
		}
		return nil, buildAIQuotas(st, time.Now()), nil
	})
}

func buildDocker(st State) DockerResult {
	out := DockerResult{
		Metadata:       metadata(st),
		Contexts:       []DockerContextInfo{},
		Engines:        []DockerEngineInfo{},
		PortContainers: []DockerPortContainerInfo{},
		KindClusters:   []KindClusterInfo{},
	}
	ts, ok := st.Snapshot.Tool("docker")
	if !ok {
		return out
	}
	out.Installed = inventory.TerminalCopy(ts.Installations) >= 0 || len(ts.Installations) > 0
	if ts.Docker == nil {
		return out
	}
	d := ts.Docker
	out.SelectedContext = d.Selected
	out.Override = d.Override
	out.RunningContainers = d.Running
	out.TotalContainers = d.Containers
	out.CredentialIssuesCount = len(d.CredentialIssues)
	if len(d.BrokenPlugins) > 0 {
		out.BrokenPlugins = slices.Clone(d.BrokenPlugins)
	}
	out.DefaultSocketIssue = d.DefaultSocketIssue
	for _, k := range d.Contexts {
		endpoint := ""
		if k.Local {
			endpoint = k.Endpoint
		}
		out.Contexts = append(out.Contexts, DockerContextInfo{
			Name:     k.Name,
			Endpoint: endpoint,
			Engine:   k.Engine,
			Status:   k.Status,
			Version:  k.Version,
			Local:    k.Local,
			Denied:   k.Denied,
		})
	}
	for _, e := range d.Engines {
		out.Engines = append(out.Engines, DockerEngineInfo{Name: e.Name, Path: e.Path})
	}
	if d.Colima != nil {
		col := &ColimaInfo{
			Installed:  d.Colima.Path != "" || d.Colima.Brew != "",
			Autostart:  d.Colima.Autostart,
			Unreadable: d.Colima.Unreadable,
			Profiles:   []ColimaProfileInfo{},
		}
		for _, p := range d.Colima.Profiles {
			col.Profiles = append(col.Profiles, ColimaProfileInfo{
				Name:        p.Name,
				Status:      p.Status,
				Runtime:     p.Runtime,
				CPU:         p.CPU,
				MemoryBytes: p.Memory,
				DiskBytes:   p.Disk,
			})
		}
		out.Colima = col
	}
	if d.Service != nil {
		out.Service = &DockerServiceInfo{
			Unit:      d.Service.Unit,
			Status:    d.Service.Status,
			Autostart: d.Service.Autostart,
		}
	}
	for _, ct := range d.PortContainers {
		ports := slices.Clone(ct.Ports)
		if ports == nil {
			ports = []string{}
		}
		out.PortContainers = append(out.PortContainers, DockerPortContainerInfo{
			ID:         ct.ID,
			Name:       ct.Name,
			Image:      ct.Image,
			Status:     ct.Status,
			ComposeDir: ct.ComposeDir,
			Ports:      ports,
		})
	}
	for _, kc := range d.KindClusters {
		out.KindClusters = append(out.KindClusters, KindClusterInfo{
			Name:    kc.Name,
			Nodes:   kc.Nodes,
			Running: kc.Running,
			Version: kc.Version,
		})
	}
	return out
}

func matchListenerProject(l inventory.DatabaseListener, ps []projects.Project) string {
	best := ""
	for _, d := range l.Dirs {
		cleanDir := filepath.ToSlash(filepath.Clean(d))
		for _, p := range ps {
			cleanProj := filepath.ToSlash(filepath.Clean(p.Dir))
			if cleanDir == cleanProj || strings.HasPrefix(cleanDir, cleanProj+"/") {
				if len(p.Dir) > len(best) {
					best = p.Dir
				}
			}
		}
	}
	return best
}

func buildPortsAndDatabases(st State, portFilter, projectFilter string) PortsAndDatabasesResult {
	out := PortsAndDatabasesResult{
		Metadata:  metadata(st),
		Servers:   []DatabaseServerInfo{},
		Clients:   []Copy{},
		Listeners: []ListenerInfo{},
	}
	for _, srv := range inventory.DatabaseServers(&st.Snapshot) {
		if portFilter != "" && !slices.Contains(srv.Ports, portFilter) {
			continue
		}
		if projectFilter != "" && srv.Project != projectFilter {
			continue
		}
		ports := slices.Clone(srv.Ports)
		if ports == nil {
			ports = []string{}
		}
		out.Servers = append(out.Servers, DatabaseServerInfo{
			Kind:      srv.Kind,
			Name:      srv.Name,
			Version:   srv.Version,
			Source:    srv.Source,
			Status:    srv.Status,
			Service:   srv.Service,
			Project:   srv.Project,
			Ports:     ports,
			Autostart: srv.Autostart,
		})
	}
	if ts, ok := st.Snapshot.Tool("databases"); ok && ts.Databases != nil {
		for _, cl := range ts.Databases.Clients {
			out.Clients = append(out.Clients, copyOf(cl))
		}
		for _, l := range ts.Databases.Listeners {
			if portFilter != "" && l.Port != portFilter {
				continue
			}
			proj := matchListenerProject(l, st.Projects)
			if projectFilter != "" && proj != projectFilter {
				continue
			}
			cls := inventory.ClassifyDevServer(l)
			out.Listeners = append(out.Listeners, ListenerInfo{
				Address:      l.Address,
				Port:         l.Port,
				Process:      l.Process,
				PID:          l.PID,
				Project:      proj,
				Role:         listenerRoleName(cls.Role),
				Kind:         cls.Kind,
				HTTPOpenable: cls.HTTPOpenable,
				CPUPercent:   math.Round(l.CPUPercent*10) / 10,
				MemoryBytes:  l.MemoryBytes,
				Own:          l.Own,
			})
		}
	}
	return out
}

func listenerRoleName(r inventory.ListenerRole) string {
	switch r {
	case inventory.RoleDevServer:
		return "dev_server"
	case inventory.RoleDatabase:
		return "database"
	case inventory.RoleKubeForward:
		return "kube_forward"
	case inventory.RoleIDE:
		return "ide"
	case inventory.RoleSystem:
		return "system"
	default:
		return "other"
	}
}

func buildSkills(st State, agentFilter, projectFilter string) SkillsResult {
	out := SkillsResult{Metadata: metadata(st), Skills: []SkillInfo{}}
	add := func(list []inventory.Skill) {
		for _, sk := range list {
			if projectFilter != "" && sk.Project != projectFilter {
				continue
			}
			var agents []string
			shared := false
			for _, sp := range sk.Paths {
				if sp.Link {
					shared = true
				}
				for _, ag := range sp.Agents {
					if !slices.Contains(agents, ag) {
						agents = append(agents, ag)
					}
				}
			}
			if agentFilter != "" && !slices.ContainsFunc(agents, func(a string) bool { return strings.EqualFold(a, agentFilter) }) {
				continue
			}
			if agents == nil {
				agents = []string{}
			}
			out.Skills = append(out.Skills, SkillInfo{
				Name:        sk.Name,
				Description: sk.Description,
				Dir:         sk.Dir,
				Project:     sk.Project,
				Agents:      agents,
				Synced:      sk.Synced,
				Broken:      sk.Broken,
				Shared:      shared,
			})
		}
	}
	add(st.Snapshot.Skills)
	for _, p := range st.Projects {
		add(p.Skills)
	}
	return out
}

func buildKubernetes(st State) KubernetesResult {
	out := KubernetesResult{
		Metadata:     metadata(st),
		Tools:        []Tool{},
		Files:        []KubeFileInfo{},
		Contexts:     []KubeContextInfo{},
		KindClusters: []KindClusterInfo{},
	}
	for _, id := range []string{"kubectl", "helm", "kind"} {
		res := tools(st, id)
		out.Tools = append(out.Tools, res.Tools...)
	}
	if ts, ok := st.Snapshot.Tool("cli"); ok && ts.Cli != nil {
		out.KubeconfigSet = ts.Cli.KubeconfigSet
		out.HelmRepos = ts.Cli.HelmRepos
		for _, c := range ts.Cli.Contexts {
			if c.Tool == "kubectl" {
				out.SelectedContext = c.Name
				break
			}
		}
		for _, f := range ts.Cli.KubeFiles {
			out.Files = append(out.Files, KubeFileInfo{
				Path:    f.Path,
				Missing: f.Missing,
				Invalid: f.Invalid,
			})
		}
		for _, k := range ts.Cli.KubeContexts {
			out.Contexts = append(out.Contexts, KubeContextInfo{
				Name:      k.Name,
				Cluster:   k.Cluster,
				Namespace: k.Namespace,
				File:      k.File,
				Shadowed:  k.Shadowed,
				Auth:      k.Auth,
			})
		}
	}
	if ts, ok := st.Snapshot.Tool("docker"); ok && ts.Docker != nil {
		for _, kc := range ts.Docker.KindClusters {
			out.KindClusters = append(out.KindClusters, KindClusterInfo{
				Name:    kc.Name,
				Nodes:   kc.Nodes,
				Running: kc.Running,
				Version: kc.Version,
			})
		}
	}
	return out
}

func buildCleanup(st State, now time.Time) CleanupResult {
	out := CleanupResult{
		Metadata:   metadata(st),
		Candidates: []CleanupCandidateInfo{},
	}
	if st.Sizes != nil && !st.Sizes.Taken.IsZero() {
		out.SizesAvailable = true
		out.SizesTakenAt = st.Sizes.Taken
	}
	cands := cleanup.Find(&st.Snapshot, st.Sizes, st.Projects, nil, now)
	for _, c := range cands {
		reasons := make([]string, 0, len(c.Reasons))
		for _, r := range c.Reasons {
			reasons = append(reasons, string(r))
		}
		frees := c.Frees()
		if c.Known && frees > 0 {
			out.TotalReclaimableBytes += frees
		}
		out.Candidates = append(out.Candidates, CleanupCandidateInfo{
			Kind:       string(c.Kind),
			ID:         c.ID,
			Manager:    c.Manager,
			Project:    c.Project,
			Role:       c.Role,
			Count:      c.Count,
			FreesBytes: frees,
			Known:      c.Known,
			Selected:   c.Selected,
			Reasons:    reasons,
		})
	}
	return out
}

func buildUsage(st State, days int, now time.Time) UsageResult {
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	cutoff := now.AddDate(0, 0, -(days - 1)).Format("2006-01-02")

	type agg struct {
		sessions int
		tokens   int64
		cost     float64
	}
	byAgent := map[string]*agg{}
	byModel := map[string]*agg{}
	byDay := map[string]*agg{}

	out := UsageResult{
		Metadata:   metadata(st),
		Days:       days,
		PricesAsOf: ai.PricesAsOf,
		ByAgent:    []UsageAgentSummary{},
		ByModel:    []UsageModelSummary{},
		ByDay:      []UsageDaySummary{},
	}

	for _, sess := range st.Snapshot.Sessions {
		var sessTokens int64
		var sessCost float64
		for _, u := range sess.Usage {
			if u.Day != "" && u.Day < cutoff {
				continue
			}
			tok := u.Tokens()
			if tok <= 0 {
				continue
			}
			cost, _ := ai.Cost(u)
			sessTokens += tok
			sessCost += cost

			m := cmp.Or(u.Model, "unknown")
			if byModel[m] == nil {
				byModel[m] = &agg{}
			}
			byModel[m].tokens += tok
			byModel[m].cost += cost

			if u.Day != "" {
				if byDay[u.Day] == nil {
					byDay[u.Day] = &agg{}
				}
				byDay[u.Day].tokens += tok
				byDay[u.Day].cost += cost
			}
		}
		if sessTokens == 0 {
			continue
		}
		out.SessionCount++
		out.TotalTokens += sessTokens
		out.EstimatedCostUSD += sessCost
		ag := cmp.Or(sess.Agent, "unknown")
		if byAgent[ag] == nil {
			byAgent[ag] = &agg{}
		}
		byAgent[ag].sessions++
		byAgent[ag].tokens += sessTokens
		byAgent[ag].cost += sessCost
	}

	out.EstimatedCostUSD = roundUSD(out.EstimatedCostUSD)
	for ag, v := range byAgent {
		out.ByAgent = append(out.ByAgent, UsageAgentSummary{
			Agent:            ag,
			Sessions:         v.sessions,
			Tokens:           v.tokens,
			EstimatedCostUSD: roundUSD(v.cost),
		})
	}
	slices.SortFunc(out.ByAgent, func(a, b UsageAgentSummary) int {
		if c := cmp.Compare(b.EstimatedCostUSD, a.EstimatedCostUSD); c != 0 {
			return c
		}
		return cmp.Compare(a.Agent, b.Agent)
	})
	for m, v := range byModel {
		out.ByModel = append(out.ByModel, UsageModelSummary{
			Model:            m,
			Tokens:           v.tokens,
			EstimatedCostUSD: roundUSD(v.cost),
		})
	}
	slices.SortFunc(out.ByModel, func(a, b UsageModelSummary) int {
		if c := cmp.Compare(b.EstimatedCostUSD, a.EstimatedCostUSD); c != 0 {
			return c
		}
		return cmp.Compare(a.Model, b.Model)
	})
	for d, v := range byDay {
		out.ByDay = append(out.ByDay, UsageDaySummary{
			Day:              d,
			Tokens:           v.tokens,
			EstimatedCostUSD: roundUSD(v.cost),
		})
	}
	slices.SortFunc(out.ByDay, func(a, b UsageDaySummary) int {
		return cmp.Compare(a.Day, b.Day)
	})
	return out
}

func roundUSD(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func buildModelApp(app *inventory.ModelApp) ModelAppInfo {
	out := ModelAppInfo{Models: []LocalModelInfo{}}
	if app == nil {
		return out
	}
	out.Found = true
	out.Version = app.Version
	out.Port = app.Port
	out.Dir = app.Dir
	for _, m := range app.Models {
		out.Models = append(out.Models, LocalModelInfo{
			Name:       m.Name,
			Format:     m.Format,
			Quant:      m.Quant,
			SizeBytes:  m.Size,
			FreesBytes: m.Frees,
			Downloaded: m.Downloaded,
			Loaded:     m.Loaded,
		})
	}
	return out
}

func buildModels(st State) ModelsResult {
	out := ModelsResult{
		Metadata: metadata(st),
		Ollama:   ModelAppInfo{Models: []LocalModelInfo{}},
		LMStudio: ModelAppInfo{Models: []LocalModelInfo{}},
	}
	if ts, ok := st.Snapshot.Tool("models"); ok && ts.Models != nil {
		out.Ollama = buildModelApp(ts.Models.Ollama)
		out.LMStudio = buildModelApp(ts.Models.LMStudio)
	}
	return out
}

func buildInstructions(st State) InstructionsResult {
	out := InstructionsResult{
		Metadata: metadata(st),
		Global:   []GlobalInstructionInfo{},
		Projects: []ProjectInstructionsInfo{},
	}
	for _, ag := range st.Snapshot.Agents {
		if ag.Instructions == nil {
			continue
		}
		gi := ag.Instructions
		out.Global = append(out.Global, GlobalInstructionInfo{
			Agent:          ag.ID,
			State:          gi.State,
			Active:         gi.Active,
			SharedPath:     gi.Shared.Path,
			SharedExists:   gi.Shared.Exists,
			NativePath:     gi.Native.Path,
			NativeExists:   gi.Native.Exists,
			NativeLink:     gi.Native.Link,
			OverridePath:   gi.Override.Path,
			OverrideExists: gi.Override.Exists,
		})
	}
	for _, p := range st.Projects {
		files := []ProjectInstructionFileInfo{}
		add := func(kind string, f projects.InstructionFile) {
			if f.Path == "" && !f.Exists {
				return
			}
			files = append(files, ProjectInstructionFileInfo{
				Kind:   kind,
				Path:   f.Path,
				Exists: f.Exists,
				Link:   f.Link,
			})
		}
		add("agents", p.Instructions.Agents)
		add("claude", p.Instructions.Claude)
		add("gemini", p.Instructions.Gemini)
		add("copilot", p.Instructions.Copilot)
		for _, rule := range p.Instructions.CursorRules {
			add("cursor_rule", rule)
		}
		out.Projects = append(out.Projects, ProjectInstructionsInfo{
			Project: p.Dir,
			State:   p.Instructions.State(),
			Files:   files,
		})
	}
	return out
}

// AIQuotaWindowInfo is a safe projection of one subscription rate-limit window.
type AIQuotaWindowInfo struct {
	Kind         string   `json:"kind"`
	Group        string   `json:"group,omitempty"`
	Model        string   `json:"model,omitempty"`
	LeftPercent  float64  `json:"left_percent"`
	UsedPercent  float64  `json:"used_percent"`
	ResetsAt     string   `json:"resets_at,omitempty"`
	RunningLow   bool     `json:"running_low"`
	Exhausted    bool     `json:"exhausted"`
	Pace         string   `json:"pace,omitempty"`
	RunsOutAt    string   `json:"runs_out_at,omitempty"`
	DollarsUsed  *float64 `json:"dollars_used,omitempty"`
	DollarsLimit *float64 `json:"dollars_limit,omitempty"`
}

// AIQuotaProviderInfo is a safe projection of one AI provider's quota state.
// Credentials, tokens, cookies, emails, file paths, and raw errors are omitted.
type AIQuotaProviderInfo struct {
	Provider  string              `json:"provider"`
	Name      string              `json:"name"`
	Plan      string              `json:"plan,omitempty"`
	Status    string              `json:"status"`
	UpdatedAt string              `json:"updated_at,omitempty"`
	Stale     bool                `json:"stale,omitempty"`
	Windows   []AIQuotaWindowInfo `json:"windows"`
}

// AIQuotasResult is the read-only MCP payload returned by get_ai_quotas.
type AIQuotasResult struct {
	Metadata
	CheckedAt      string                `json:"checked_at,omitempty"`
	ConnectedCount int                   `json:"connected_count"`
	LowCount       int                   `json:"low_count"`
	Providers      []AIQuotaProviderInfo `json:"providers"`
}

func buildAIQuotas(st State, now time.Time) AIQuotasResult {
	out := AIQuotasResult{
		Metadata:  metadata(st),
		Providers: []AIQuotaProviderInfo{},
	}
	if st.Quotas == nil {
		return out
	}
	q := *st.Quotas
	if !q.CheckedAt.IsZero() {
		out.CheckedAt = q.CheckedAt.UTC().Format(time.RFC3339)
	}
	out.ConnectedCount = q.ConnectedCount()
	out.LowCount = q.LowCount(now)
	for _, p := range q.Providers {
		if p.Status == "" || p.Status == aiquota.StatusNotFound {
			continue
		}
		info := AIQuotaProviderInfo{
			Provider: string(p.Provider),
			Name:     p.Name,
			Plan:     p.Plan,
			Status:   string(p.Status),
			Stale:    p.IsStale(now),
			Windows:  []AIQuotaWindowInfo{},
		}
		if !p.UpdatedAt.IsZero() {
			info.UpdatedAt = p.UpdatedAt.UTC().Format(time.RFC3339)
		}
		for _, w := range p.Windows {
			wi := AIQuotaWindowInfo{
				Kind:         string(w.Kind),
				Group:        w.Group,
				Model:        w.Model,
				LeftPercent:  math.Round(w.EffectiveLeft(now)*10) / 10,
				UsedPercent:  math.Round(w.Used*10) / 10,
				RunningLow:   w.IsRunningLow(now),
				Exhausted:    w.IsExhausted(now),
				Pace:         string(w.PaceStatus(now)),
				DollarsUsed:  w.DollarsUsed,
				DollarsLimit: w.DollarsLimit,
			}
			if !w.ResetsAt.IsZero() {
				wi.ResetsAt = w.ResetsAt.UTC().Format(time.RFC3339)
			}
			if ro := w.RunsOutAt(now); !ro.IsZero() {
				wi.RunsOutAt = ro.UTC().Format(time.RFC3339)
			}
			info.Windows = append(info.Windows, wi)
		}
		out.Providers = append(out.Providers, info)
	}
	return out
}
