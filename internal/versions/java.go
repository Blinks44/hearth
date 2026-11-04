package versions

import "context"

// Java lists Eclipse Temurin release lines. Other vendors have their own support policies.
type Java = Cycles

// FetchJava gets Temurin's release lines using the shared offline cache.
func FetchJava(ctx context.Context, c *Cache) (Java, error) {
	return fetchCycles(ctx, c, "Temurin", "java-temurin-cycles.json", "https://endoflife.date/api/eclipse-temurin.json")
}
