package ui

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
)

// dockerImageBrand maps a Docker image reference (such as "postgres:16-alpine",
// "ghcr.io/bitnami/redis:7", or "kindest/node:v1.31") to a bundled brand slug
// in resources/icons/brand/ & logo/, or "" when no specific brand matches.
func dockerImageBrand(image string) string {
	ref := strings.ToLower(strings.TrimSpace(image))
	if ref == "" || strings.HasPrefix(ref, "sha256:") || ref == "<none>" {
		return ""
	}
	if i := strings.IndexByte(ref, '@'); i >= 0 {
		ref = ref[:i]
	}
	slash := strings.LastIndexByte(ref, '/')
	if colon := strings.LastIndexByte(ref, ':'); colon > slash {
		ref = ref[:colon]
	}
	repo := ref
	namespace := ""
	if slash >= 0 {
		repo = ref[slash+1:]
		prefix := ref[:slash]
		if _, ns, ok := strings.CutLast(prefix, "/"); ok {
			namespace = ns
		} else {
			namespace = prefix
		}
	}
	if namespace == "kindest" {
		return "kubernetes"
	}
	if slug := matchDockerBrand(repo); slug != "" {
		return slug
	}
	if namespace != "" && !strings.Contains(namespace, ".") && !strings.Contains(namespace, ":") && namespace != "library" {
		return matchDockerBrand(namespace)
	}
	return ""
}

func matchDockerBrand(name string) string {
	tokens := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	has := func(words ...string) bool {
		for _, w := range words {
			if name == w || slices.Contains(tokens, w) {
				return true
			}
		}
		return false
	}
	hasPrefix := func(prefixes ...string) bool {
		for _, p := range prefixes {
			if strings.HasPrefix(name, p) {
				return true
			}
		}
		return false
	}
	switch {
	case has("postgres", "postgresql", "postgis", "pgvector", "timescaledb", "pgbouncer") || hasPrefix("postgres"):
		return "postgresql"
	case has("mariadb") || hasPrefix("mariadb"):
		return "mariadb"
	case has("mysql", "percona") || hasPrefix("mysql"):
		return "mysql"
	case has("redis", "valkey", "dragonfly", "dragonflydb") || hasPrefix("redis"):
		return "redis"
	case has("nginx", "openresty") || hasPrefix("nginx"):
		return "nginx"
	case has("mongo", "mongodb") || hasPrefix("mongo"):
		return "mongodb"
	case has("rabbitmq") || hasPrefix("rabbitmq"):
		return "rabbitmq"
	case has("elasticsearch", "opensearch", "kibana") || hasPrefix("elastic"):
		return "elasticsearch"
	case has("caddy") || hasPrefix("caddy"):
		return "caddy"
	case has("traefik") || hasPrefix("traefik"):
		return "traefikproxy"
	case has("minio") || hasPrefix("minio"):
		return "minio"
	case has("clickhouse") || hasPrefix("clickhouse"):
		return "clickhouse"
	case has("grafana") || hasPrefix("grafana"):
		return "grafana"
	case has("prometheus") || hasPrefix("prometheus"):
		return "prometheus"
	case has("kafka", "redpanda") || hasPrefix("kafka"):
		return "apachekafka"
	case has("mattermost") || hasPrefix("mattermost"):
		return "mattermost"
	case has("meilisearch") || hasPrefix("meilisearch"):
		return "meilisearch"
	case has("php", "frankenphp", "roadrunner") || hasPrefix("php"):
		return "php"
	case has("node", "nodejs"):
		return "nodedotjs"
	case has("python"):
		return "python"
	case has("golang", "go"):
		return "go"
	case has("rust"):
		return "rust"
	case has("ruby"):
		return "ruby"
	case has("openjdk", "temurin", "corretto", "amazoncorretto", "maven", "gradle", "java"):
		return "openjdk"
	case has("dotnet", "aspnet"):
		return "dotnet"
	case has("bun"):
		return "bun"
	case has("deno"):
		return "deno"
	case has("kubectl", "kube", "kubernetes", "k3s", "etcd"):
		return "kubernetes"
	case has("docker", "dind", "buildkit"):
		return "docker"
	case has("alpine"):
		return "alpinelinux"
	case has("ubuntu"):
		return "ubuntu"
	case has("debian"):
		return "debian"
	default:
		return ""
	}
}

// dockerTechTile draws an OrbStack-style rounded technology tile for a Docker
// container or image, with an optional live status dot at its bottom-right
// corner.
func dockerTechTile(c *ui.Context, image string, size float32, statusDot *ui.Color) {
	t := c.Theme()
	pal := Colors(t)
	iconSize := size * 0.62
	radius := size * 0.25
	ui.Box(c).Size(size, size).Shrink(0).Radius(radius).Center().
		Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Children(func() {
		if slug := dockerImageBrand(image); slug != "" {
			iconOrLogo(c, "brand/"+slug, iconSize, pal.TextFaint)
		} else {
			ui.Icon(c, Icon("container")).Size(iconSize, iconSize).TextColor(pal.TextFaint)
		}
		if statusDot != nil {
			ui.Box(c).Absolute().Right(-2).Bottom(-2).Size(8, 8).Radius(4).
				Background(*statusDot).BorderWidth(1.5).BorderColor(t.Background)
		}
	})
}
