#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 3 ]; then
  echo "usage: $0 <version> <arm64-dmg-sha256> <amd64-dmg-sha256>" >&2
  exit 1
fi

version="$1"
sha256_arm64="$2"
sha256_amd64="$3"

if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "error: invalid version: $version" >&2
  exit 1
fi

if ! [[ "$sha256_arm64" =~ ^[0-9a-fA-F]{64}$ ]]; then
  echo "error: invalid arm64 sha256: $sha256_arm64" >&2
  exit 1
fi

if ! [[ "$sha256_amd64" =~ ^[0-9a-fA-F]{64}$ ]]; then
  echo "error: invalid amd64 sha256: $sha256_amd64" >&2
  exit 1
fi

sha256_arm64=$(printf '%s' "$sha256_arm64" | tr '[:upper:]' '[:lower:]')
sha256_amd64=$(printf '%s' "$sha256_amd64" | tr '[:upper:]' '[:lower:]')

cat <<EOF
cask "hearth" do
  arch arm: "arm64", intel: "amd64"

  version "${version}"
  sha256 arm:   "${sha256_arm64}",
         intel: "${sha256_amd64}"

  url "https://github.com/Blinks44/hearth/releases/download/v#{version}/hearth-#{version}-#{arch}.dmg"
  name "Hearth"
  desc "Local desktop app for developer tools, versions, PATH, Docker, Kubernetes, and AI agents"
  homepage "https://hearth.moduix.dev"

  livecheck do
    url :url
    strategy :github_latest
  end

  auto_updates true
  depends_on macos: :monterey

  app "Hearth.app"

  postflight_steps do
    run "/usr/bin/xattr",
        args: ["-dr", "com.apple.quarantine", "{{appdir}}/Hearth.app"]
  end

  zap trash: [
    "~/Library/Application Support/Hearth",
    "~/Library/Application Support/io.github.blinks44.hearth",
    "~/Library/Caches/Hearth",
    "~/Library/Caches/io.github.blinks44.hearth",
    "~/Library/Logs/Hearth",
    "~/Library/Preferences/io.github.blinks44.hearth.plist",
  ]

  caveats <<~EOS
    Hearth is currently signed ad hoc without Apple notarization.
    This cask removes the macOS quarantine attribute during installation so Hearth launches without Gatekeeper prompts.
  EOS
end
EOF
