//go:build c5_embed

package main

import _ "embed"

// embeddedVersion is the version of the embedded c5 binary, injected via ldflags.
var embeddedVersion = ""

//go:embed embed/c5/c5
var embeddedC5Binary []byte
