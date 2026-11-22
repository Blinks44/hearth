package detect

import (
	"context"
	"encoding/json"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

var (
	ollamaVersion = regexp.MustCompile(`version is (\S+)`)
	// ggufQuant is the quantization a GGUF file's name ends with, as in
	// SmolLM2-135M-Instruct-Q4_K_M.gguf or model.f16.gguf.
	ggufQuant = regexp.MustCompile(`(?i)[-._]((?:I?Q\d\w*)|BF16|F16|F32)\.gguf$`)
)

// Models finds Ollama and LM Studio and the models they downloaded, by
// their files: neither server is asked, but ollama ps tells the models
// Ollama keeps in memory when it runs.
func Models(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	home := sources.ManagerDirs(term).Home
	s := &inventory.ModelsState{Ollama: ollama(ctx, r, term, gui, home), LMStudio: lmStudio(home)}
	return inventory.ToolState{Tool: "models", Models: s}, ctx.Err()
}

// ollama is Ollama's command, its models' folder and the models in it;
// nil when neither is found.
func ollama(ctx context.Context, r run.Runner, term, gui sysenv.Env, home string) *inventory.ModelApp {
	app := &inventory.ModelApp{Port: "11434"}
	if host := term.Get("OLLAMA_HOST"); host != "" {
		if _, port, err := net.SplitHostPort(strings.TrimPrefix(strings.TrimPrefix(host, "http://"), "https://")); err == nil {
			app.Port = port
		}
	}
	f := newFinder("ollama", term)
	f.lookPath(exeName("ollama"), term, gui)
	var loaded []string
	if i := inventory.TerminalCopy(f.ins); i >= 0 {
		in := f.ins[i]
		app.Path, app.Source = in.Path, in.Source
		// Without the server it warns: client version is 0.5.4.
		if res, err := r.Run(ctx, run.Cmd{Args: []string{in.Path, "--version"}, Env: term}); err == nil {
			if m := ollamaVersion.FindSubmatch(append(res.Stdout, res.Stderr...)); m != nil {
				app.Version = string(m[1])
			}
		}
		if res, err := r.Run(ctx, run.Cmd{Args: []string{in.Path, "ps"}, Env: term}); err == nil && res.ExitCode == 0 {
			loaded = ollamaLoaded(string(res.Stdout))
		}
	}
	app.Dir = filepath.Join(home, ".ollama", "models")
	if dir := term.Get("OLLAMA_MODELS"); dir != "" {
		app.Dir, app.DirVar = dir, "OLLAMA_MODELS"
	} else if !exists(app.Dir) {
		for _, dir := range ollamaServiceDirs {
			if present(dir) {
				app.Dir = dir
			}
		}
	}
	if app.Path == "" && !present(app.Dir) {
		return nil
	}
	models, err := ollamaModels(app.Dir)
	if err != nil {
		app.Err = err.Error()
		// The Linux service's folder is its user's alone: ollama list
		// asks the server instead, without dates or shared layers.
		if app.Path != "" {
			if res, err := r.Run(ctx, run.Cmd{Args: []string{app.Path, "list"}, Env: term}); err == nil && res.ExitCode == 0 {
				models, app.Err = ollamaList(string(res.Stdout)), ""
			}
		}
	}
	for i := range models {
		models[i].Loaded = slices.Contains(loaded, models[i].Name)
	}
	app.Models = models
	return app
}

// ollamaLoaded are the models in ollama ps's table: NAME ID SIZE ...
func ollamaLoaded(out string) []string {
	var names []string
	for i, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); i > 0 && len(f) > 0 {
			names = append(names, f[0])
		}
	}
	return names
}

// ollamaUnits are ollama list's size units, in powers of 1000.
var ollamaUnits = map[string]float64{"B": 1, "KB": 1e3, "MB": 1e6, "GB": 1e9, "TB": 1e12}

// ollamaList are the models in ollama list's table: NAME ID SIZE MODIFIED,
// the size as "4.7 GB".
func ollamaList(out string) []inventory.Model {
	var models []inventory.Model
	for i, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if i == 0 || len(f) < 4 {
			continue
		}
		n, err := strconv.ParseFloat(f[2], 64)
		if err != nil {
			continue
		}
		size := int64(n * ollamaUnits[f[3]])
		models = append(models, inventory.Model{Name: f[0], Size: size, Frees: size})
	}
	return models
}

// present reports that something is at path, even where it may not be
// read, as a folder another user keeps.
func present(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !os.IsNotExist(err)
}

// ollamaManifest is a model's manifest: its layers are blobs by digest,
// the model's weights among them; its config a blob that tells the
// format and quantization.
type ollamaManifest struct {
	Config ollamaLayer   `json:"config"`
	Layers []ollamaLayer `json:"layers"`
}

type ollamaLayer struct {
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

// ollamaModels reads the manifests under dir/manifests/<host>/<namespace>
// /<model>/<tag>. A blob two models share frees nothing alone.
func ollamaModels(dir string) ([]inventory.Model, error) {
	root := filepath.Join(dir, "manifests")
	var models []inventory.Model
	var manifests []ollamaManifest
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) != 4 {
			return nil
		}
		var m ollamaManifest
		data, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(data, &m) != nil {
			return nil //nolint:nilerr // a manifest being written or broken is no model
		}
		info, err := d.Info()
		if err != nil {
			return nil //nolint:nilerr // gone while read
		}
		model := inventory.Model{Name: ollamaName(parts), Path: path, Downloaded: info.ModTime()}
		var cfg struct {
			Format string `json:"model_format"`
			Quant  string `json:"file_type"`
		}
		if json.Unmarshal([]byte(readFile(ollamaBlob(dir, m.Config.Digest))), &cfg) == nil {
			model.Format, model.Quant = strings.ToUpper(cfg.Format), cfg.Quant
		}
		models = append(models, model)
		manifests = append(manifests, m)
		return nil
	})
	if os.IsNotExist(err) {
		err = nil
	}
	users := map[string]int{}
	for _, m := range manifests {
		for _, l := range append(m.Layers, m.Config) {
			users[l.Digest]++
		}
	}
	for i, m := range manifests {
		for _, l := range append(m.Layers, m.Config) {
			models[i].Size += l.Size
			if users[l.Digest] == 1 {
				models[i].Frees += l.Size
			}
		}
	}
	return models, err
}

// ollamaName is a model's name as ollama list says it: llama3.2:3b for
// the library's, user/model:tag for another namespace, and with the host
// for another registry.
func ollamaName(parts []string) string {
	host, namespace, model, tag := parts[0], parts[1], parts[2], parts[3]
	switch {
	case host == "registry.ollama.ai" && namespace == "library":
		return model + ":" + tag
	case host == "registry.ollama.ai":
		return namespace + "/" + model + ":" + tag
	}
	return host + "/" + namespace + "/" + model + ":" + tag
}

// ollamaBlob is the file of a blob: sha256:abc is blobs/sha256-abc.
func ollamaBlob(dir, digest string) string {
	return filepath.Join(dir, "blobs", strings.Replace(digest, ":", "-", 1))
}

// lmStudio is LM Studio's models' folder and the models in it, a folder
// each under a publisher's; nil when LM Studio's own folder is not there.
func lmStudio(home string) *inventory.ModelApp {
	if home == "" {
		return nil
	}
	own := filepath.Join(home, ".lmstudio")
	legacy := filepath.Join(home, ".cache", "lm-studio")
	app := &inventory.ModelApp{Port: "1234", Dir: filepath.Join(own, "models")}
	cfgRoot := own
	var settings struct {
		Downloads string `json:"downloadsFolder"`
	}
	switch {
	case json.Unmarshal([]byte(readFile(filepath.Join(own, "settings.json"))), &settings) == nil && settings.Downloads != "":
		app.Dir = settings.Downloads
	case exists(own):
	case exists(legacy):
		// Before 0.3 LM Studio kept its files there.
		cfgRoot = legacy
		app.Dir = filepath.Join(legacy, "models")
	default:
		return nil
	}
	if port := lmStudioPort(cfgRoot); port != "" {
		app.Port = port
	} else if cfgRoot == own {
		if port := lmStudioPort(legacy); port != "" {
			app.Port = port
		}
	}
	repos, _ := filepath.Glob(filepath.Join(app.Dir, "*", "*"))
	for _, repo := range repos {
		if m, ok := lmStudioModel(repo); ok {
			app.Models = append(app.Models, m)
		}
	}
	return app
}

// lmStudioPort reads the configured HTTP server port from LM Studio's
// .internal/http-server-config.json; "" when absent or invalid.
func lmStudioPort(root string) string {
	var cfg struct {
		Port int `json:"port"`
	}
	if json.Unmarshal([]byte(readFile(filepath.Join(root, ".internal", "http-server-config.json"))), &cfg) != nil {
		return ""
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return ""
	}
	return strconv.Itoa(cfg.Port)
}

// lmStudioModel is a model's folder: its files' size, the newest file's
// time, GGUF for .gguf files and MLX for safetensors, and the quantization
// a .gguf file's name tells.
func lmStudioModel(dir string) (inventory.Model, bool) {
	m := inventory.Model{Name: filepath.Base(dir), Path: dir}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return nil //nolint:nilerr // gone while read
		}
		m.Size += info.Size()
		if info.ModTime().After(m.Downloaded) {
			m.Downloaded = info.ModTime()
		}
		switch filepath.Ext(path) {
		case ".gguf":
			m.Format = "GGUF"
			if q := ggufQuant.FindStringSubmatch(d.Name()); q != nil && !strings.HasPrefix(d.Name(), "mmproj") {
				m.Quant = strings.ToUpper(q[1])
			}
		case ".safetensors":
			if m.Format == "" {
				m.Format = "MLX"
			}
		}
		return nil
	})
	m.Frees = m.Size
	return m, err == nil && m.Size > 0
}
