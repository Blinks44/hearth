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

// TrayIconMacOS is the monochrome Hearth flame template icon (32x32 @2x) for the macOS menu bar.
//
//go:embed tray-icon-macos.png
var TrayIconMacOS []byte

// TrayIconColor is the full-color Hearth badge icon (64x64) for Windows and Linux system trays.
//
//go:embed tray-icon-color.png
var TrayIconColor []byte

// Notice is Hearth's license and the licenses of everything it ships:
// fonts, icons and Go modules.
//
//go:embed NOTICE.txt
var Notice string
