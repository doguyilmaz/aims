package claude

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/tool"
)

// Edits go through gjson/sjson: aims adds or replaces only the keys it owns,
// and the user's files keep their key order.

// seedJSON builds a new profile's .claude.json from the hub's: only the keys
// in seedKeys, and per project only seedProjectKeys. Returns nil when there is
// nothing worth seeding.
func seedJSON(src []byte) ([]byte, error) {
	if !gjson.ValidBytes(src) {
		return nil, nil
	}
	doc := gjson.ParseBytes(src)
	out := []byte("{}")
	var err error
	doc.ForEach(func(k, v gjson.Result) bool {
		key := k.String()
		switch {
		case contains(seedKeys, key):
			out, err = sjson.SetRawBytes(out, escapeKey(key), []byte(v.Raw))
		case key == "projects" && v.IsObject():
			v.ForEach(func(pk, pv gjson.Result) bool {
				pv.ForEach(func(ik, iv gjson.Result) bool {
					if contains(seedProjectKeys, ik.String()) {
						out, err = sjson.SetRawBytes(out, "projects."+escapeKey(pk.String())+"."+escapeKey(ik.String()), []byte(iv.Raw))
					}
					return err == nil
				})
				return err == nil
			})
		}
		return err == nil
	})
	if err != nil {
		return nil, err
	}
	if string(out) == "{}" {
		return nil, nil
	}
	return pretty.PrettyOptions(out, &pretty.Options{Indent: "  ", Width: 80, SortKeys: false}), nil
}

// setStatusLine points settings.json's statusLine at command and returns the
// command it replaced (unless that was aims' own).
func setStatusLine(settings []byte, command string) ([]byte, string, error) {
	if len(settings) == 0 {
		settings = []byte("{}")
	}
	if !gjson.ValidBytes(settings) {
		return nil, "", fmt.Errorf("settings.json is not valid JSON")
	}
	prev := gjson.GetBytes(settings, "statusLine.command").String()
	if IsAimsStatusLine(prev) {
		prev = ""
	}
	var out []byte
	var err error
	if gjson.GetBytes(settings, "statusLine").IsObject() {
		// Keep the user's other status line settings (padding and so on).
		out, err = sjson.SetBytes(settings, "statusLine.type", "command")
		if err == nil {
			out, err = sjson.SetBytes(out, "statusLine.command", command)
		}
	} else {
		out, err = sjson.SetBytes(settings, "statusLine", map[string]any{"type": "command", "command": command, "padding": 0})
	}
	if err != nil {
		return nil, "", err
	}
	return pretty.PrettyOptions(out, &pretty.Options{Indent: "  ", Width: 80}), prev, nil
}

// unsetStatusLine restores previous (or removes statusLine) if aims' command
// is still the one set.
func unsetStatusLine(settings []byte, command, previous string) ([]byte, bool, error) {
	if !gjson.ValidBytes(settings) {
		return nil, false, fmt.Errorf("settings.json is not valid JSON")
	}
	cur := gjson.GetBytes(settings, "statusLine.command").String()
	if cur == "" || (cur != command && !IsAimsStatusLine(cur)) {
		return settings, false, nil
	}
	var out []byte
	var err error
	if previous != "" {
		out, err = sjson.SetBytes(settings, "statusLine.command", previous)
	} else {
		out, err = sjson.DeleteBytes(settings, "statusLine")
	}
	if err != nil {
		return nil, false, err
	}
	return pretty.PrettyOptions(out, &pretty.Options{Indent: "  ", Width: 80}), true, nil
}

// escapeKey makes a JSON key safe as one gjson/sjson path component (project
// keys are file paths full of dots).
func escapeKey(k string) string {
	var b []byte
	for i := 0; i < len(k); i++ {
		switch c := k[i]; c {
		case '.', '*', '?', '|', '#', '@', '\\', ':', '!', '=', '<', '>', '%':
			b = append(b, '\\', c)
		default:
			b = append(b, c)
		}
	}
	return string(b)
}

// plainSecrets names the MCP servers in a .claude.json whose env or headers
// hold a credential written out. aims shares these servers with every
// account.
func plainSecrets(doc []byte) []string {
	var out []string
	check := func(where string, servers gjson.Result) {
		servers.ForEach(func(name, srv gjson.Result) bool {
			for _, field := range []string{"env", "headers"} {
				srv.Get(field).ForEach(func(k, v gjson.Result) bool {
					if tool.PlainSecret(k.String(), v.String()) {
						out = append(out, fmt.Sprintf("MCP server %s%s has %s %s written out in .claude.json, and aims shares it with every account; use ${%s} so the value stays in your environment", name.String(), where, field, k.String(), envName(k.String())))
					}
					return true
				})
			}
			return true
		})
	}
	check("", gjson.GetBytes(doc, "mcpServers"))
	gjson.GetBytes(doc, "projects").ForEach(func(p, v gjson.Result) bool {
		check(" ("+fsx.Tildify(p.String())+")", v.Get("mcpServers"))
		return true
	})
	return out
}

// envName suggests an environment variable for a header or env key.
func envName(k string) string {
	return strings.ToUpper(strings.NewReplacer("-", "_", " ", "_").Replace(k))
}
