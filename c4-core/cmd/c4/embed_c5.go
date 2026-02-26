//go:build c5_embed

package main

import (
	"embed"
	"io/fs"
)

// embeddedC5Version is set at build time via -ldflags.
// It is used for fast-path cache invalidation: if the version on disk
// matches embeddedC5Version, extraction is skipped.
var embeddedC5Version string

//go:embed embed/c5/c5*
var embeddedC5FS embed.FS

// EmbeddedC5FS is a non-nil fs.FS when the binary is built with c5_embed tag.
var EmbeddedC5FS fs.FS = embeddedC5FS

// ExtractEmbeddedC5 extracts the embedded c5 binary to ~/.c4/bin/c5
// and returns the full path. See extractC5 for details.
func ExtractEmbeddedC5() (string, error) {
	return extractC5(EmbeddedC5FS, embeddedC5Version)
}
