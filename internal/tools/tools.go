// Package tools is the list of supported CLIs. Adding one means writing an
// adapter package that implements tool.Adapter and listing it here.
package tools

import (
	"strings"

	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tool/claude"
	"github.com/doguyilmaz/aims/internal/tool/codex"
)

var all = []tool.Adapter{claude.New(), codex.New()}

// All returns the adapters in display order.
func All() []tool.Adapter { return all }

// IDs returns the tool ids in display order.
func IDs() []tool.ID {
	ids := make([]tool.ID, len(all))
	for i, a := range all {
		ids[i] = a.ID()
	}
	return ids
}

// Get returns the adapter for id, or nil.
func Get(id tool.ID) tool.Adapter {
	for _, a := range all {
		if a.ID() == id {
			return a
		}
	}
	return nil
}

// Parse looks a tool up by name.
func Parse(name string) (tool.Adapter, bool) {
	a := Get(tool.ID(strings.ToLower(name)))
	return a, a != nil
}

// Names lists the tool ids for messages and completion.
func Names() []string {
	out := make([]string, len(all))
	for i, a := range all {
		out[i] = string(a.ID())
	}
	return out
}
