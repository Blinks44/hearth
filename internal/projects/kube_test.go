package projects

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadKube(t *testing.T) {
	object := "apiVersion: apps/v1\nkind: Deployment\n"
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{name: "none", files: map[string]string{"package.json": "{}"}},
		{
			name: "files and folders",
			files: map[string]string{
				"skaffold.yaml":                      "apiVersion: skaffold/v4beta11\n",
				"Tiltfile":                           "k8s_yaml('k8s/app.yaml')\n",
				"k8s/app.yaml":                       object,
				"charts/shop/Chart.yaml":             "apiVersion: v2\nname: shop\n",
				"charts/shop/templates/service.yaml": object,
			},
			want: []string{"skaffold.yaml", "Tiltfile", "k8s/", "charts/"},
		},
		{
			name:  "a deploy folder of scripts is not one",
			files: map[string]string{"deploy/run.sh": "#!/bin/sh\n", "deploy/vars.yml": "region: eu\n"},
		},
		{
			name:  "deeper than charts/app/templates is not looked at",
			files: map[string]string{"helm/a/b/c/d/app.yaml": object},
		},
		{
			name:  "a folder named as a file",
			files: map[string]string{"Chart.yaml/x": ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, text := range tt.files {
				path := filepath.Join(dir, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := Read(dir).Kube; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Kube = %q, want %q", got, tt.want)
			}
		})
	}
}
