package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// DockerCredentialPlan replaces exactly the observed helper value. The
// preview only contains the affected entry, even in a minified config.
func DockerCredentialPlan(env sysenv.Env, dir string, issue inventory.DockerCredentialIssue) (Plan, error) {
	if !filepath.IsAbs(dir) || dir != docker.ConfigDir(env) || !docker.CredentialHelperName(issue.Name) ||
		docker.CredentialHelperAvailable(env, issue.Name) || issue.Replacement == "" ||
		issue.Replacement != docker.CredentialReplacement(env) {
		return Plan{}, errors.New(locales.DockerHelperChanged())
	}
	path := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Plan{}, fmt.Errorf("read Docker configuration: %w", err)
	}
	before := string(data)
	if !json.Valid(data) {
		return Plan{}, errors.New(locales.DockerHelperChanged())
	}
	key := "credsStore"
	if issue.Key != "" {
		if docker.RegistryName(issue.Key) == "" || docker.RegistryName(issue.Key) != issue.Registry {
			return Plan{}, errors.New(locales.DockerHelperChanged())
		}
		key = "credHelpers"
	}
	start, end, found := dockerJSONValue(before, key, true)
	if found && issue.Key != "" {
		innerStart, innerEnd, ok := dockerJSONValue(before[start:end], issue.Key, false)
		end, start, found = start+innerEnd, start+innerStart, ok
	}
	var old string
	if !found || json.Unmarshal(data[start:end], &old) != nil || old != issue.Name {
		return Plan{}, errors.New(locales.DockerHelperChanged())
	}
	newValue, _ := json.Marshal(issue.Replacement)
	line := strings.Count(before[:start], "\n") + 1
	prefix := strconv.Quote(key) + ": "
	if issue.Key != "" {
		prefix = strconv.Quote(key) + " → " + strconv.Quote(issue.Key) + ": "
	}
	edit := &FileEdit{
		Path:   path,
		Before: before,
		After:  before[:start] + string(newValue) + before[end:],
		preview: []DiffLine{
			{Line: line, Text: prefix + strconv.Quote(old)},
			{Line: line, Text: prefix + string(newValue), Added: true},
		},
	}
	return Plan{
		Title:  locales.DockerHelperPlanTitle(),
		Why:    locales.DockerHelperPlanWhy(key, issue.Replacement),
		Action: locales.DockerHelperReplace(),
		Before: func(context.Context) error {
			if docker.CredentialHelperAvailable(env, issue.Name) || !docker.CredentialHelperAvailable(env, issue.Replacement) {
				return errors.New(locales.DockerHelperChanged())
			}
			return nil
		},
		Steps:   []Step{{Edit: edit}},
		Notes:   []Note{{Title: locales.DockerHelperLoginAgainTitle(), Text: locales.DockerHelperLoginAgain()}},
		Changes: []string{locales.DockerHelperLoginAgain()},
	}, nil
}

// dockerJSONValue locates one uniquely named JSON member. A decoder keeps
// other members, indentation, and line endings byte-for-byte unchanged.
func dockerJSONValue(text, key string, fold bool) (start, end int, found bool) {
	start = len(text) - len(strings.TrimLeft(text, " \r\n\t"))
	members, _, err := jsonObject(text, start)
	if err != nil {
		return 0, 0, false
	}
	for _, member := range members {
		if member.key == key || fold && strings.EqualFold(member.key, key) {
			if found {
				return 0, 0, false
			}
			start, end, found = member.valStart, member.end, true
		}
	}
	return start, end, found
}

// DockerCredentialHelperInstallPlan returns a package manager installation
// plan for a missing Docker credential helper when Hearth knows the owning
// package. Currently supports osxkeychain via Homebrew's docker-credential-helper.
func DockerCredentialHelperInstallPlan(helper string, m inventory.PackageManager, reread Reread) (Plan, bool) {
	if helper != "osxkeychain" || m.ID != "brew" {
		return Plan{}, false
	}
	pkg := inventory.SystemPackage{Name: "docker-credential-helper", Kind: "formula"}
	p, ok := PackagePlan(PackageInstall, m, []inventory.SystemPackage{pkg}, reread)
	if !ok {
		return Plan{}, false
	}
	p.Why = locales.DockerHelperInstallWhy("docker-credential-"+helper, m.Name)
	return p, true
}
