package ui

import "testing"

func TestDockerImageBrand(t *testing.T) {
	cases := []struct {
		image string
		want  string
	}{
		{"postgres:16-alpine", "postgresql"},
		{"postgis/postgis:16-3.4", "postgresql"},
		{"kindest/node:v1.31.2", "kubernetes"},
		{"ghcr.io/bitnami/redis:7.4", "redis"},
		{"valkey/valkey:8-alpine", "redis"},
		{"php:8.4-fpm-alpine", "php"},
		{"dunglas/frankenphp:latest", "php"},
		{"nginx:1.27-alpine", "nginx"},
		{"mariadb:11", "mariadb"},
		{"mysql:8.4", "mysql"},
		{"mongo:7", "mongodb"},
		{"node:22-slim", "nodedotjs"},
		{"python:3.13-alpine", "python"},
		{"golang:1.24", "go"},
		{"caddy:2-alpine", "caddy"},
		{"traefik:v3.2", "traefikproxy"},
		{"rabbitmq:4-management", "rabbitmq"},
		{"grafana/grafana:11", "grafana"},
		{"prom/prometheus:v2.55", "prometheus"},
		{"minio/minio:latest", "minio"},
		{"alpine:3.21", "alpinelinux"},
		{"fedora:41", "fedora"},
		{"archlinux:latest", "archlinux"},
		{"opensuse/tumbleweed:latest", "opensuse"},
		{"nixos/nix:latest", "nixos"},
		{"rancher/k3s:v1.31.2-k3s1", "k3s"},
		{"rancher/rancher:latest", "rancher"},
		{"quay.io/podman/stable:latest", "podman"},
		{"sha256:abcdef123456", ""},
		{"<none>", ""},
		{"mycompany/custom-worker:1.0", ""},
	}
	for _, tc := range cases {
		t.Run(tc.image, func(t *testing.T) {
			if got := dockerImageBrand(tc.image); got != tc.want {
				t.Errorf("dockerImageBrand(%q) = %q, want %q", tc.image, got, tc.want)
			}
			if tc.want != "" {
				_ = mustIcon("brand/" + tc.want)
				_ = mustIcon("logo/" + tc.want)
			}
		})
	}
}
