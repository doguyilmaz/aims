package proc

import (
	"path/filepath"
	"strings"
)

func attachedCommand(bin string, args, env []string) (string, []string, error) {
	ext := strings.ToLower(filepath.Ext(bin))
	if ext != ".cmd" && ext != ".bat" {
		return bin, args, nil
	}
	return npmShimCommand(bin, args, env)
}
