//go:build skills_embed

package main

import (
	"embed"
	"io/fs"
)

//go:embed all:skills_src
var _embeddedSkills embed.FS

// EmbeddedSkillsFS is a non-nil fs.FS when the binary is built with skills_embed tag.
var EmbeddedSkillsFS fs.FS = _embeddedSkills
