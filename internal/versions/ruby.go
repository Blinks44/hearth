package versions

import "context"

// Ruby is what is known of MRI release lines and their security support.
type Ruby = Cycles

// FetchRuby gets Ruby's release lines through the shared offline cache.
func FetchRuby(ctx context.Context, c *Cache) (Ruby, error) {
	return fetchCycles(ctx, c, "Ruby", "ruby-cycles.json", "https://endoflife.date/api/ruby.json")
}
