//go:build !c5_embed

package main

// embeddedVersion is empty when c5 is not embedded.
var embeddedVersion = ""

// embeddedC5Binary is nil when the binary is built without the c5_embed tag.
var embeddedC5Binary []byte = nil
