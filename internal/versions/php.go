package versions

import "context"

// PHPCyclesURL lists PHP's release lines with their newest release
// and the end of their security support.
const PHPCyclesURL = "https://endoflife.date/api/php.json"

// PHPLine is a PHP release line, as 8.4.
type PHPLine = Cycle

// PHP is what is known of PHP's release lines.
type PHP = Cycles

// FetchPHP gets PHP's release lines through c.
func FetchPHP(ctx context.Context, c *Cache) (PHP, error) {
	return fetchCycles(ctx, c, "PHP", "php-cycles.json", PHPCyclesURL)
}

// ParsePHPCycles reads endoflife.date's list of PHP's lines.
func ParsePHPCycles(data []byte) ([]PHPLine, error) { return parseCycles("PHP", data) }
