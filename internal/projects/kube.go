package projects

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// kubeFiles are the files in a project folder that tell it is deployed to
// Kubernetes: a Helm chart, Kustomize, Skaffold, Tilt, helmfile.
var kubeFiles = []string{"Chart.yaml", "kustomization.yaml", "kustomization.yml", "Kustomization", "skaffold.yaml", "Tiltfile", "helmfile.yaml"}

// kubeDirs are the folders projects usually keep Kubernetes manifests in.
// One counts only when a YAML file in it starts a Kubernetes object, so
// that a deploy folder of scripts does not.
var kubeDirs = []string{"k8s", "kubernetes", "manifests", "charts", "helm", "deploy"}

// kubeDepth and kubeReads bound the look into a folder: charts/app/
// templates is deep enough, and a big folder is not read whole.
const (
	kubeDepth = 3
	kubeReads = 50
)

// readKube finds the project's Kubernetes files and folders.
func readKube(p *Project) {
	for _, name := range kubeFiles {
		if info, err := os.Stat(filepath.Join(p.Dir, name)); err == nil && !info.IsDir() {
			p.Kube = append(p.Kube, name)
		}
	}
	for _, name := range kubeDirs {
		if hasManifest(filepath.Join(p.Dir, name)) {
			p.Kube = append(p.Kube, name+"/")
		}
	}
}

// hasManifest reports that dir has, kubeDepth folders down at most, a
// YAML file with a Kubernetes object's apiVersion.
func hasManifest(dir string) bool {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return false
	}
	reads, found := 0, false
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found || reads >= kubeReads {
			return fs.SkipAll
		}
		rel, _ := filepath.Rel(dir, path)
		if d.IsDir() {
			if rel != "." && strings.Count(rel, string(filepath.Separator)) >= kubeDepth {
				return fs.SkipDir
			}
			return nil
		}
		switch d.Name() {
		case "kustomization.yaml", "kustomization.yml", "Kustomization":
			found = true
			return fs.SkipAll
		}
		if ext := filepath.Ext(path); ext != ".yaml" && ext != ".yml" {
			return nil
		}
		reads++
		found = startsObject(path)
		return nil
	})
	return found
}

// startsObject reports that a line of the file at path begins with
// "apiVersion:", as every Kubernetes object and Chart.yaml do.
func startsObject(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }() // read only
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "apiVersion:") {
			return true
		}
	}
	return false
}
