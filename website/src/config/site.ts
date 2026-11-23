export const site = {
  name: "Hearth",
  origin: "https://hearth.moduix.dev",
  repoOwner: "Blinks44",
  repoName: "hearth",
  repository: "https://github.com/Blinks44/hearth",
  releases: "https://github.com/Blinks44/hearth/releases",
  latestRelease: "https://github.com/Blinks44/hearth/releases/latest",
  discussions: "https://github.com/Blinks44/hearth/discussions",
  issues: "https://github.com/Blinks44/hearth/issues",
  docs: "https://github.com/Blinks44/hearth#readme",
  security: "https://github.com/Blinks44/hearth/blob/main/SECURITY.md",
  changelog: "https://github.com/Blinks44/hearth/blob/main/CHANGELOG.md",
  contributing: "https://github.com/Blinks44/hearth/blob/main/CONTRIBUTING.md",
  license: "https://github.com/Blinks44/hearth/blob/main/LICENSE",
  brewCommand: "brew install --cask blinks44/tap/hearth",
  linuxInstallCommand:
    "curl -fsSL https://github.com/Blinks44/hearth/releases/latest/download/install.sh | sh",
  windowsInstallCommand:
    "irm https://github.com/Blinks44/hearth/releases/latest/download/install.ps1 | iex",
  defaultVersion: "1.0.0",
  author: {
    name: "Sergey V.",
    x: "https://x.com/TheBlinks182",
    xHandle: "@TheBlinks182",
  },
  suite: {
    monny: "https://monny.moduix.dev",
    rebind: "https://rebind.moduix.dev",
  },
  indexingEnabled: true,
} as const;
