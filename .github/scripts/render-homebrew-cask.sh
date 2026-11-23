#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: $0 <version> <dmg-sha256>" >&2
  exit 1
fi

version="$1"
sha256="$2"

if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "error: invalid version: $version" >&2
  exit 1
fi

if ! [[ "$sha256" =~ ^[0-9a-fA-F]{64}$ ]]; then
  echo "error: invalid sha256: $sha256" >&2
  exit 1
fi

sha256=$(printf '%s' "$sha256" | tr '[:upper:]' '[:lower:]')

cat <<EOF
cask "hearth" do
  version "${version}"
  sha256 "${sha256}"

  url "https://github.com/Blinks44/hearth/releases/download/v#{version}/hearth-#{version}.dmg",
      verified: "github.com/Blinks44/hearth/"
  name "Hearth"
  desc "Local desktop app for developer tools, versions, PATH, Docker, Kubernetes, and AI agents"
  homepage "https://hearth.moduix.dev"

  livecheck do
    url :url
    strategy :github_latest
  end

  depends_on macos: ">= :monterey"

  app "Hearth.app"

  caveats <<~EOS
    Hearth is currently signed ad hoc without Apple notarization.
    If macOS blocks the first launch, open System Settings -> Privacy & Security
    and click "Open Anyway", or run:
      xattr -dr com.apple.quarantine /Applications/Hearth.app
  EOS

  zap trash: [
    "~/Library/Application Support/Hearth",
    "~/Library/Application Support/io.github.blinks44.hearth",
    "~/Library/Caches/Hearth",
    "~/Library/Caches/io.github.blinks44.hearth",
    "~/Library/Logs/Hearth",
    "~/Library/Preferences/io.github.blinks44.hearth.plist",
  ]
end
EOF
