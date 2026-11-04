// Package resources embeds the files the app ships with: fonts now, icons
// later. Licenses of third-party files sit next to them.
package resources

import "embed"

// Fonts holds Inter and JetBrains Mono (both SIL OFL 1.1), as fonts/*.ttf.
//
//go:embed fonts/*.ttf
var Fonts embed.FS
