package kube

import (
	"slices"
	"testing"
)

func TestShellArgs(t *testing.T) {
	target := Shell{Kubectl: "/opt/dev tools/kubectl", Context: "team's dev", Namespace: "shop", Pod: "checkout", Container: "sidecar"}
	want := []string{target.Kubectl, "--context=team's dev", "-n", "shop", "exec", "-it", "checkout", "-c", "sidecar", "--", "sh"}
	if got := target.Args(); !slices.Equal(got, want) {
		t.Fatalf("args = %q, want %q", got, want)
	}
}
