//go:build !c5_embed

package main

import (
	"fmt"
	"io/fs"
)

// embeddedC5Version is empty when built without c5_embed tag.
var embeddedC5Version string

// EmbeddedC5FS is nil when the binary is built without the c5_embed tag.
// HubComponent falls back to PATH lookup only in this case.
var EmbeddedC5FS fs.FS = nil

// ExtractEmbeddedC5 is unreachable in non-c5_embed builds because EmbeddedC5FS
// is nil and the caller guards on EmbeddedC5FS != nil before invoking this.
func ExtractEmbeddedC5() (string, error) {
	return "", fmt.Errorf("c5 binary not embedded (build without c5_embed tag)")
}
