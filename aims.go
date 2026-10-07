// Package aims holds the files embedded into the binary.
package aims

import _ "embed"

// Skill is the agent skill installed by `aims setup` into Claude Code and Codex.
//
//go:embed skills/aims/SKILL.md
var Skill string
