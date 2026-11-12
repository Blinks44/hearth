// Package resources embeds the files the app ships with: fonts, icons and
// the licenses notice.
// Licenses of third-party files sit next to them.
package resources

import "embed"

// Fonts holds Inter and JetBrains Mono (both SIL OFL 1.1), as fonts/*.ttf.
//
//go:embed fonts/*.ttf
var Fonts embed.FS

// Icons holds Lucide icons as icons/lucide/<name>.svg and tool logos as
// icons/brand/<slug>.svg; sources and licenses are in icons/LICENSES.md.
//
//go:embed icons/lucide/*.svg icons/brand/*.svg
var Icons embed.FS

// Notice is Hearth's license and the licenses of everything it ships:
// fonts, icons and Go modules.
//
//go:embed NOTICE.txt
var Notice string
