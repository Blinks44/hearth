package pkgmgr

// PlatformLabel returns the display name of the current OS.
func PlatformLabel() string { return "Windows" }

// AvailableManagers returns safe, well-supported package managers for Windows.
func AvailableManagers() []AvailableManager {
	return []AvailableManager{
		{
			ID:          "winget",
			Name:        "WinGet",
			Recommended: true,
			WebsiteURL:  "https://apps.microsoft.com/detail/9nblggh4nns1",
		},
		{
			ID:          "scoop",
			Name:        "Scoop",
			Recommended: true,
			Installable: true,
			WebsiteURL:  "https://scoop.sh",
		},
		{
			ID:         "choco",
			Name:       "Chocolatey",
			WebsiteURL: "https://chocolatey.org/install",
			TermsURL:   "https://docs.chocolatey.org/en-us/information/legal",
		},
	}
}
