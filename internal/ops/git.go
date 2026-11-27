package ops

import (
	"context"
	"errors"
	"slices"

	"github.com/Blinks44/hearth/locales"
)

// GitCredentialHelperPlan returns a plan that switches the global Git
// credential.helper away from plain-text "store" to the platform default.
func GitCredentialHelperPlan(detect Detect) (Plan, bool) {
	helper := gitDefaultCredentialHelper()
	if helper == "" {
		return Plan{}, false
	}
	p := Plan{
		Title:   locales.GitCredFixTitle(helper),
		Why:     locales.GitCredFixWhy(),
		Changes: []string{locales.GitCredFixChange(helper)},
		Steps:   []Step{{Argv: []string{"git", "config", "--global", "--replace-all", "credential.helper", helper}}},
		Notes: []Note{{
			Title: locales.GitCredFixNoteTitle(),
			Text:  locales.GitCredFixNoteBody(),
		}},
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			if st.Git != nil && slices.Contains(st.Git.Helpers, "store") {
				return "", errors.New("git credential.helper is still store")
			}
			return locales.GitCredFixChange(helper), nil
		}
	}
	return p, true
}
