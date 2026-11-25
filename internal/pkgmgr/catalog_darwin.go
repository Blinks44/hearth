package pkgmgr

// PlatformLabel returns the display name of the current OS.
func PlatformLabel() string { return "macOS" }

// AvailableManagers returns safe, well-supported package managers for macOS.
func AvailableManagers() []AvailableManager {
	return []AvailableManager{
		{
			ID:          "brew",
			Name:        "Homebrew",
			Recommended: true,
			Installable: true,
			WebsiteURL:  "https://brew.sh",
		},
		{
			ID:         "nix",
			Name:       "Nix",
			WebsiteURL: "https://nixos.org/download/",
		},
		{
			ID:         "port",
			Name:       "MacPorts",
			WebsiteURL: "https://www.macports.org/install.php",
		},
	}
}
