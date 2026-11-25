package pkgmgr

// PlatformLabel returns the display name of the current OS.
func PlatformLabel() string { return "Linux" }

// AvailableManagers returns safe, well-supported package managers for Linux.
func AvailableManagers() []AvailableManager {
	return []AvailableManager{
		{
			ID:          "flatpak",
			Name:        "Flatpak",
			Recommended: true,
			Installable: true,
			WebsiteURL:  "https://flatpak.org/setup/",
		},
		{
			ID:         "brew",
			Name:       "Homebrew",
			WebsiteURL: "https://brew.sh",
		},
		{
			ID:         "nix",
			Name:       "Nix",
			WebsiteURL: "https://nixos.org/download/",
		},
	}
}
