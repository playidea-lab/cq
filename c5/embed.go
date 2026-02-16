// Package c5 provides embedded static content for the C5 Hub server.
package c5

import "embed"

//go:embed llms.txt
var LLMSTxt string

//go:embed docs/*.md
var DocsFS embed.FS
