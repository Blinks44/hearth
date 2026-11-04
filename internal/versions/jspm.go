package versions

import (
	"context"
	"encoding/json"
	"fmt"
)

// JSManagers holds the latest stable registry releases of the JS managers.
type JSManagers struct {
	Npm, Pnpm, Yarn Latest
}

// FetchJSManagers gets the three stable releases through the daily cache.
func FetchJSManagers(ctx context.Context, c *Cache) (JSManagers, error) {
	var out JSManagers
	for _, p := range []struct {
		name string
		dst  *Latest
	}{{"npm", &out.Npm}, {"pnpm", &out.Pnpm}} {
		data, at, err := c.Get(ctx, p.name+"-latest.json", "https://registry.npmjs.org/"+p.name+"/latest")
		if err != nil {
			return out, err
		}
		v, err := ParseNpmLatest(data)
		if err != nil {
			return out, err
		}
		*p.dst = Latest{Version: v, Fetched: at}
	}
	data, at, err := c.Get(ctx, "yarn-tags.json", "https://repo.yarnpkg.com/tags")
	if err != nil {
		return out, err
	}
	var tags struct{ Latest struct{ Stable string } }
	if err := json.Unmarshal(data, &tags); err != nil {
		return out, fmt.Errorf("read Yarn tags: %w", err)
	}
	if tags.Latest.Stable == "" {
		return out, fmt.Errorf("read Yarn tags: no stable version")
	}
	out.Yarn = Latest{Version: tags.Latest.Stable, Fetched: at}
	return out, nil
}
