package claude

import (
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"
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

// mergeMCPServers adds the hub's user MCP servers that dst lacks.
func mergeMCPServers(src, dst []byte) ([]byte, []string, error) {
	if !gjson.ValidBytes(src) || !gjson.ValidBytes(dst) {
		return nil, nil, fmt.Errorf("a .claude.json is not valid JSON")
	}
	have := gjson.GetBytes(dst, "mcpServers")
	var added []string
	out := dst
	var err error
	gjson.GetBytes(src, "mcpServers").ForEach(func(k, v gjson.Result) bool {
		name := k.String()
		if have.Get(escapeKey(name)).Exists() {
			return true
		}
		out, err = sjson.SetRawBytes(out, "mcpServers."+escapeKey(name), []byte(v.Raw))
		added = append(added, name)
		return err == nil
	})
	if err != nil || len(added) == 0 {
		return out, added, err
	}
	return pretty.PrettyOptions(out, &pretty.Options{Indent: "  ", Width: 80}), added, nil
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
	out, err := sjson.SetRawBytes(settings, "statusLine", []byte(fmt.Sprintf(`{"type":"command","command":%q,"padding":0}`, command)))
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
