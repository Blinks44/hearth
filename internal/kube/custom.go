package kube

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/util/jsonpath"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

type CustomDefinition struct {
	Group, Scope string
	Names        struct {
		Kind, Plural, Singular string
		ShortNames             []string
	}
	Versions []CustomVersion
}

type CustomVersion struct {
	Name            string
	Served, Storage bool
	Columns         []CustomColumn `json:"additionalPrinterColumns"`
}

type CustomColumn struct {
	Name, Description, Type, Format, JSONPath string
	Priority                                  int
}

type CustomDefinitions struct {
	Items  []CustomDefinition
	Err    error
	Access *ListAccess
}

type CustomCondition struct {
	Type, Status, Reason, Message string
	LastTransitionTime            time.Time
}

type CustomObject struct {
	Definition *CustomDefinition
	Version    string
	Cells      []string
	Conditions []CustomCondition
}

func (d CustomDefinition) Name() string { return d.Names.Plural + "." + d.Group }

func (d CustomDefinition) Namespaced() bool { return d.Scope == "Namespaced" }

// Version returns the served storage version, or the first served version
// when a migration has made the storage version unavailable to clients.
func (d CustomDefinition) Version() CustomVersion {
	var first CustomVersion
	for _, v := range d.Versions {
		if v.Served {
			if v.Storage {
				return v
			}
			if first.Name == "" {
				first = v
			}
		}
	}
	return first
}

func (d CustomDefinition) resourceKind(version string) ResourceKind {
	return ResourceKind(d.Names.Plural + "." + version + "." + d.Group)
}

var customLabels = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)

func (d CustomDefinition) valid() bool {
	version := d.Version().Name
	return (d.Scope == "Namespaced" || d.Scope == "Cluster") &&
		customLabels.MatchString(d.Name()) && customLabels.MatchString(version) &&
		!strings.Contains(d.Names.Plural, ".") && !strings.Contains(version, ".")
}

// ListCustomDefinitions reads the cluster-wide CRD catalog alongside the user's
// list permissions for the active namespace scope.
func ListCustomDefinitions(ctx context.Context, r run.Runner, env sysenv.Env, t Target) *CustomDefinitions {
	result := &CustomDefinitions{}
	var wg sync.WaitGroup
	wg.Go(func() {
		defs := readCustomDefinitions(ctx, r, env, t)
		result.Items, result.Err = defs.Items, defs.Err
	})
	wg.Go(func() {
		result.Access = readListAccess(ctx, r, env, t)
	})
	wg.Wait()
	return result
}

// ProbeCustomPresence checks which CRDs have at least one object in t's
// namespace scope using lightweight limit=1 metadata list requests over the
// direct API client. Map entries are present only for CRDs whose emptiness
// could be confirmed; missing keys fail open.
func ProbeCustomPresence(ctx context.Context, env sysenv.Env, t Target, defs []CustomDefinition) map[string]bool {
	if t.API == nil || len(defs) == 0 {
		return nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	workers := min(8, len(defs))
	jobs := make(chan CustomDefinition)
	var (
		mu      sync.Mutex
		present = make(map[string]bool, len(defs))
		wg      sync.WaitGroup
	)
	for range workers {
		wg.Go(func() {
			for def := range jobs {
				if probeCtx.Err() != nil {
					return
				}
				has, known := t.API.hasCustomObject(probeCtx, env, t, def)
				if !known {
					continue
				}
				mu.Lock()
				present[def.Name()] = has
				mu.Unlock()
			}
		})
	}
sendLoop:
	for _, def := range defs {
		select {
		case jobs <- def:
		case <-probeCtx.Done():
			break sendLoop
		}
	}
	close(jobs)
	wg.Wait()
	if len(present) == 0 {
		return nil
	}
	return present
}

func readCustomDefinitions(ctx context.Context, r run.Runner, env sysenv.Env, t Target) *CustomDefinitions {
	var list struct {
		Items []struct{ Spec CustomDefinition }
	}
	result := &CustomDefinitions{}
	result.Err = get(ctx, r, env, t, []string{"customresourcedefinitions.apiextensions.k8s.io"}, &list)
	if result.Err != nil {
		return result
	}
	for _, item := range list.Items {
		if item.Spec.valid() {
			result.Items = append(result.Items, item.Spec)
		}
	}
	slices.SortFunc(result.Items, func(a, b CustomDefinition) int {
		return cmp.Or(strings.Compare(a.Group, b.Group), strings.Compare(a.Names.Plural, b.Names.Plural))
	})
	return result
}

func readCustomCluster(ctx context.Context, r run.Runner, env sysenv.Env, t Target) (Cluster, error) {
	defs := readCustomDefinitions(ctx, r, env, t)
	if defs.Err != nil {
		return Cluster{}, defs.Err
	}
	index := slices.IndexFunc(defs.Items, func(d CustomDefinition) bool { return d.Name() == t.CustomKind })
	if index < 0 {
		return Cluster{}, fmt.Errorf("custom resource definition %q is unavailable", t.CustomKind)
	}
	def := &defs.Items[index]
	version := def.Version()
	list := &ResourceList{Kind: def.resourceKind(version.Name), Custom: def}
	columns := make([]*jsonpath.JSONPath, len(version.Columns))
	for i, col := range version.Columns {
		path := jsonpath.New(col.Name).AllowMissingKeys(true)
		if strings.HasPrefix(col.JSONPath, ".") && path.Parse("{"+col.JSONPath+"}") == nil {
			columns[i] = path
		}
	}
	scopes := [][]string{nil}
	if def.Namespaced() {
		scopes = namespaceScopes(t.Namespaces)
	}
	for _, scope := range scopes {
		var objects struct{ Items []json.RawMessage }
		if err := get(ctx, r, env, t, append([]string{string(list.Kind)}, scope...), &objects); err != nil {
			return Cluster{}, err
		}
		for _, raw := range objects.Items {
			item, err := customResource(raw, def, version, columns)
			if err != nil {
				return Cluster{}, err
			}
			list.Items = append(list.Items, item)
		}
	}
	cluster := Cluster{Taken: time.Now(), Resources: list}
	cluster.readListEvents(ctx, r, env, t, cluster.Resources)
	return cluster, nil
}

func customResource(raw json.RawMessage, def *CustomDefinition, version CustomVersion, paths []*jsonpath.JSONPath) (Resource, error) {
	var object struct {
		meta
		Status json.RawMessage
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return Resource{}, fmt.Errorf("decode Kubernetes custom resource: %w", err)
	}
	custom := &CustomObject{Definition: def, Version: version.Name, Cells: make([]string, len(paths))}
	// Conditions are a convention, not part of the CRD contract. A different
	// status schema must not prevent listing the object.
	var status struct{ Conditions []json.RawMessage }
	_ = json.Unmarshal(object.Status, &status)
	for _, rawCondition := range status.Conditions {
		var condition CustomCondition
		if json.Unmarshal(rawCondition, &condition) == nil && condition.Type != "" {
			custom.Conditions = append(custom.Conditions, condition)
		}
	}
	var fields map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&fields); err != nil {
		return Resource{}, fmt.Errorf("decode Kubernetes custom fields: %w", err)
	}
	customNumbers(fields)
	for i, path := range paths {
		if path != nil && version.Columns[i].Format != "password" {
			custom.Cells[i] = customCell(path, fields, version.Columns[i].Type)
		}
	}
	m := object.Metadata
	return Resource{Kind: def.resourceKind(version.Name), Namespace: m.Namespace, Name: m.Name, UID: m.UID,
		Created: m.CreationTimestamp, Deleting: !m.DeletionTimestamp.IsZero(), Custom: custom}, nil
}

// JSONPath compares typed numbers. Preserve large integers and convert
// decimal numbers to the floating point representation used by Kubernetes.
func customNumbers(value any) any {
	switch value := value.(type) {
	case map[string]any:
		for key, field := range value {
			value[key] = customNumbers(field)
		}
	case []any:
		for i, item := range value {
			value[i] = customNumbers(item)
		}
	case json.Number:
		if n, err := value.Int64(); err == nil {
			return n
		}
		if n, err := value.Float64(); err == nil {
			return n
		}
	}
	return value
}

func customCell(path *jsonpath.JSONPath, fields map[string]any, kind string) string {
	values, err := path.FindResults(fields)
	if err != nil || len(values) == 0 || len(values[0]) == 0 {
		return ""
	}
	switch v := values[0][0].Interface().(type) {
	case string:
		if kind == "string" || kind == "date" {
			return v
		}
	case int64:
		if kind == "integer" || kind == "number" {
			return strconv.FormatInt(v, 10)
		}
	case float64:
		if kind == "number" {
			return strconv.FormatFloat(v, 'g', -1, 64)
		}
	case bool:
		if kind == "boolean" {
			return fmt.Sprint(v)
		}
	}
	return ""
}
