// Package resources embeds the files the app ships with: fonts, icons and
// the licenses notice.
// Licenses of third-party files sit next to them.
package resources

import "embed"

// Fonts holds Inter and JetBrains Mono (both SIL OFL 1.1), as fonts/*.ttf.
//
//go:embed fonts/*.ttf
var Fonts embed.FS

// Icons holds Lucide icons as icons/lucide/<name>.svg, tool logos in one
// color as icons/brand/<slug>.svg and in their own colors as
// icons/logo/<slug>.svg; sources and licenses are in icons/LICENSES.md.
//
//go:embed icons/lucide/*.svg icons/brand/*.svg icons/logo/*.svg
var Icons embed.FS

// Logo is the app icon's tile without its margin, small, for the sidebar.
//
//go:embed logo.png
var Logo []byte

// Notice is Hearth's license and the licenses of everything it ships:
// fonts, icons and Go modules.
//
//go:embed NOTICE.txt
var Notice string
