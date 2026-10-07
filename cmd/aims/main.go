// Command aims runs several Claude Code and Codex accounts side by side.
package main

import (
	"os"

	"github.com/doguyilmaz/aims/internal/cli"
)

// version is set at build time (-ldflags "-X main.version=...").
var version = "dev"

func main() { os.Exit(cli.Execute(version)) }
