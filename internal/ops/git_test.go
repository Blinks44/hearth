package ops

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
)

func TestGitCredentialHelperPlan(t *testing.T) {
	helper := gitDefaultCredentialHelper()
	detectOk := func(context.Context) (inventory.ToolState, error) {
		return inventory.ToolState{Tool: "git", Git: &inventory.GitState{Helpers: []string{helper}}}, nil
	}
	p, ok := GitCredentialHelperPlan(detectOk)
	if !ok {
		t.Fatal("expected GitCredentialHelperPlan")
	}
	wantArgv := []string{"git", "config", "--global", "--replace-all", "credential.helper", helper}
	if len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, wantArgv) {
		t.Fatalf("argv = %q, want %q", p.Steps[0].Argv, wantArgv)
	}
	if len(p.Notes) == 0 {
		t.Error("expected warning note about ~/.git-credentials")
	}
	f := &run.Fake{}
	f.Stdout("", wantArgv...)
	if got := Execute(t.Context(), f, p, &strings.Builder{}, func(State) {}); got.Phase != Succeeded {
		t.Errorf("phase = %v, err = %q", got.Phase, got.Err)
	}

	detectStillStore := func(context.Context) (inventory.ToolState, error) {
		return inventory.ToolState{Tool: "git", Git: &inventory.GitState{Helpers: []string{"store"}}}, nil
	}
	p2, _ := GitCredentialHelperPlan(detectStillStore)
	if got := Execute(t.Context(), f, p2, &strings.Builder{}, func(State) {}); got.Phase != NotVerified {
		t.Errorf("still store: phase = %v, want NotVerified", got.Phase)
	}
}
