//go:build !skills_embed

package main

import "io/fs"

// EmbeddedSkillsFS is nil when the binary is built without the skills_embed tag.
// setupSkills() falls back to symlink mode (findC4Root) in this case.
var EmbeddedSkillsFS fs.FS = nil
