package inventory

// ColimaState is the local Colima CLI and the VM profiles it reported.
type ColimaState struct {
	Path, Brew string
	Profiles   []ColimaProfile
	Autostart  string // "on", "off", or "" when it could not be checked
	Unreadable bool
}

// ColimaProfile contains only the VM settings needed by the Docker page.
type ColimaProfile struct {
	Name, Status, Runtime string
	Driver, Mount, Socket string
	CPU                   int
	Memory, Disk          int64
}
