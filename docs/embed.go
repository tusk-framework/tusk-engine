package docs

import "embed"

// UserGuide contains the canonical user documentation shipped with the Engine.
//
//go:embed user-guide/*.md
var UserGuide embed.FS
