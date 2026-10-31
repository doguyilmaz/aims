package claude

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/tidwall/gjson"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"
)

// The keys Seed copies are kept the same in every home's .claude.json with a
// three-way merge: each home's edits since the last run go onto what was
// merged then, and of two homes that changed one key the newer file wins.
// Nothing else in the file is read or written.

// flat is the shared part of a .claude.json: a path of components joined by
// sep, mapped to its compact raw JSON.
type flat map[string]string

const sep = "\x00"

var (
	// listKeys are sets: their members merge one by one.
	listKeys = []string{"allowedTools", "enabledMcpjsonServers", "disabledMcpjsonServers"}
	// onceKeys only ever turn true.
	onceKeys = []string{"hasTrustDialogAccepted", "hasCompletedProjectOnboarding"}
)

// configLockWait is how long a write waits for Claude Code's own lock.
var configLockWait = time.Second

type sharedBase struct {
	Merged flat            `json:"merged"`
	Homes  map[string]flat `json:"homes"`
}

type home struct {
	path string
	doc  []byte
	keys flat
	mod  time.Time
	// base is what the home held after the last run; nil when aims has not
	// shared it before, so it only adds.
	base flat
}

func (*Adapter) ShareSettings(homes []tool.Home, base string) ([]string, error) {
	unlock, ok, err := fsx.Lock(base, 2*time.Second)
	if err != nil || !ok {
		return nil, err
	}
	defer unlock()
	var prev sharedBase
	_ = fsx.ReadJSONLenient(base, &prev)
	var hs []*home
	for _, h := range homes {
		path := fsx.Real(claudeJSON(h))
		if slices.ContainsFunc(hs, func(o *home) bool { return o.path == path }) {
			continue
		}
		doc, err := os.ReadFile(path)
		fi := fsx.Stat(path)
		if err != nil || fi == nil || !gjson.ValidBytes(doc) {
			continue // not logged in yet, or being edited by hand
		}
		hs = append(hs, &home{path: path, doc: doc, keys: flatten(doc), mod: fi.ModTime(), base: prev.Homes[path]})
	}
	if len(hs) < 2 {
		return nil, nil
	}
	merged := merge(prev.Merged, hs)
	next := sharedBase{Merged: merged, Homes: map[string]flat{}}
	changed := map[string]bool{}
	var errs []error
	for _, h := range hs {
		wrote, err := write(h, merged)
		if err != nil {
			errs = append(errs, err)
		}
		if !wrote {
			// Its edits are in merged; what it still lacks it never had.
			next.Homes[h.path] = h.keys
			continue
		}
		next.Homes[h.path] = merged
		for k := range diff(h.keys, merged) {
			changed[label(k)] = true
		}
	}
	if err := fsx.WriteJSON(base, next, 0o600); err != nil {
		errs = append(errs, err)
	}
	return slices.Sorted(maps.Keys(changed)), errors.Join(errs...)
}

// write brings h's file to want under Claude Code's lock. It leaves the file
// alone when Claude Code changed the shared keys since h was read: the next
// run merges those edits.
func write(h *home, want flat) (bool, error) {
	if len(diff(h.keys, want)) == 0 {
		return true, nil
	}
	unlock, ok := lockConfig(h.path)
	if !ok {
		return false, nil
	}
	defer unlock()
	doc, err := os.ReadFile(h.path)
	if err != nil {
		return false, err
	}
	if !bytes.Equal(doc, h.doc) && len(diff(flatten(doc), h.keys)) > 0 {
		return false, nil
	}
	out, err := apply(doc, flatten(doc), want)
	if err != nil {
		return false, err
	}
	return true, fsx.WriteFile(h.path, out, 0o600)
}

// lockConfig takes the lock Claude Code takes to write a config file
// (proper-lockfile: a directory beside it). It never breaks a lock it did not
// take.
func lockConfig(path string) (unlock func(), ok bool) {
	lock := path + ".lock"
	deadline := time.Now().Add(configLockWait)
	for {
		err := os.Mkdir(lock, 0o755)
		if err == nil {
			return func() { os.Remove(lock) }, true
		}
		if !os.IsExist(err) || time.Now().After(deadline) {
			return nil, false
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func merge(prev flat, hs []*home) flat {
	all := maps.Clone(prev)
	if all == nil {
		all = flat{}
	}
	for _, h := range hs {
		maps.Copy(all, h.keys)
		maps.Copy(all, h.base)
	}
	out := flat{}
	for k := range all {
		pv, pok := prev[k]
		v, ok := pv, pok
		var newest time.Time
		for _, h := range hs {
			hv, has := h.keys[k]
			switch bv, had := h.base[k]; {
			case h.base == nil && (!has || pok && hv == pv):
				continue
			case h.base != nil && has == had && hv == bv:
				continue
			case !has && once(k):
				continue
			case !newest.IsZero() && !h.mod.After(newest):
				continue
			}
			newest, v, ok = h.mod, hv, has
		}
		if ok {
			out[k] = v
		}
	}
	return out
}

func flatten(doc []byte) flat {
	out := flat{}
	gjson.ParseBytes(doc).ForEach(func(k, v gjson.Result) bool {
		switch key := k.String(); {
		case key == "mcpServers":
			v.ForEach(func(n, s gjson.Result) bool {
				out[join(key, n.String())] = compact(s.Raw)
				return true
			})
		case key == "projects":
			v.ForEach(func(p, pv gjson.Result) bool {
				flattenProject(out, p.String(), pv)
				return true
			})
		case contains(seedKeys, key):
			out[key] = compact(v.Raw)
		}
		return true
	})
	return out
}

func flattenProject(out flat, p string, v gjson.Result) {
	v.ForEach(func(k, iv gjson.Result) bool {
		switch key := k.String(); {
		case !contains(seedProjectKeys, key):
		case key == "mcpServers":
			iv.ForEach(func(n, s gjson.Result) bool {
				out[join("projects", p, key, n.String())] = compact(s.Raw)
				return true
			})
		case contains(listKeys, key):
			iv.ForEach(func(_, e gjson.Result) bool {
				out[join("projects", p, key, compact(e.Raw))] = "true"
				return true
			})
		case contains(onceKeys, key):
			if iv.Type == gjson.True {
				out[join("projects", p, key)] = "true"
			}
		}
		return true
	})
}

// apply edits doc's shared keys from have to want, leaving the rest of the
// file and its key order as they are.
func apply(doc []byte, have, want flat) ([]byte, error) {
	lists := map[string][]string{}
	var err error
	for _, k := range slices.Sorted(maps.Keys(diff(have, want))) {
		c := strings.Split(k, sep)
		if len(c) == 4 && c[0] == "projects" && contains(listKeys, c[2]) {
			lists[path(c[:3])] = c[:3]
			continue
		}
		if v, ok := want[k]; ok {
			doc, err = sjson.SetRawBytes(doc, path(c), []byte(v))
		} else {
			doc, err = sjson.DeleteBytes(doc, path(c))
		}
		if err != nil {
			return nil, err
		}
	}
	for p, c := range lists {
		prefix := join(c...) + sep
		var members []string
		gjson.GetBytes(doc, p).ForEach(func(_, e gjson.Result) bool {
			if _, ok := want[prefix+compact(e.Raw)]; ok {
				members = append(members, compact(e.Raw))
			}
			return true
		})
		for _, k := range slices.Sorted(maps.Keys(want)) {
			if m, ok := strings.CutPrefix(k, prefix); ok && !slices.Contains(members, m) {
				members = append(members, m)
			}
		}
		if doc, err = sjson.SetRawBytes(doc, p, []byte("["+strings.Join(members, ",")+"]")); err != nil {
			return nil, err
		}
	}
	return pretty.PrettyOptions(doc, &pretty.Options{Indent: "  ", Width: 80}), nil
}

// diff lists the keys whose value differs between a and b.
func diff(a, b flat) flat {
	out := flat{}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			out[k] = v
		}
	}
	for k, v := range b {
		if _, ok := a[k]; !ok {
			out[k] = v
		}
	}
	return out
}

func once(k string) bool {
	c := strings.Split(k, sep)
	return contains(onceKeys, c[len(c)-1])
}

// label names a shared key for people: "MCP server coolify", "~/app: trust".
func label(k string) string {
	c := strings.Split(k, sep)
	switch {
	case c[0] == "mcpServers":
		return "MCP server " + c[1]
	case c[0] != "projects" || len(c) < 3:
		return c[0]
	}
	what := map[string]string{
		"hasTrustDialogAccepted":        "trust",
		"hasCompletedProjectOnboarding": "onboarding",
		"allowedTools":                  "allowed tools",
		"mcpServers":                    "MCP servers",
		"enabledMcpjsonServers":         ".mcp.json approvals",
		"disabledMcpjsonServers":        ".mcp.json approvals",
	}[c[2]]
	return fsx.Tildify(c[1]) + ": " + what
}

func join(c ...string) string { return strings.Join(c, sep) }

func path(c []string) string {
	esc := make([]string, len(c))
	for i, s := range c {
		esc[i] = escapeKey(s)
	}
	return strings.Join(esc, ".")
}

func compact(raw string) string { return string(pretty.Ugly([]byte(raw))) }
