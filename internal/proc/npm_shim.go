package proc

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var npmNodeTail = regexp.MustCompile(`(?im)"%_prog%"[ \t]+([^"\r\n]*?)"%dp0%[\\/]([^"\r\n]+)"[ \t]+%\*[ \t]*$`)
var npmNativeTail = regexp.MustCompile(`(?im)^"%dp0%[\\/]([^"\r\n]+)"[ \t]+%\*[ \t]*$`)

// npmShimCommand runs the runtime and entrypoint from npm's generated Windows
// shim directly. Protocol arguments never pass through cmd.exe expansion.
// Unknown batch programs are rejected instead of guessing their semantics.
func npmShimCommand(bin string, args, env []string) (string, []string, error) {
	f, err := os.Open(bin)
	if err != nil {
		return "", nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil {
		return "", nil, err
	}
	if len(b) > 64<<10 {
		return "", nil, fmt.Errorf("batch adapter is too large to identify as an npm shim")
	}
	text := strings.ReplaceAll(string(b), "\r", "")
	node := npmNodeTail.FindStringSubmatch(text)
	native := npmNativeTail.FindStringSubmatch(text)
	unsupported := func() (string, []string, error) {
		return "", nil, fmt.Errorf("%s is not a supported npm command shim; use a native executable or the adapter's Node entrypoint", bin)
	}
	if node == nil && native == nil {
		return unsupported()
	}
	if len(npmNodeTail.FindAllStringIndex(text, -1))+len(npmNativeTail.FindAllStringIndex(text, -1)) != 1 {
		return unsupported()
	}
	if node != nil && !strings.Contains(strings.ToUpper(text), `SET "_PROG=NODE"`) {
		return unsupported()
	}
	// Only the generated prologue/runtime-selection commands are accepted.
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		upper := strings.ToUpper(line)
		if at := npmNodeTail.FindStringIndex(line); at != nil {
			prefix := strings.ToUpper(strings.TrimSpace(line[:at[0]]))
			if prefix != "" && prefix != "ENDLOCAL & GOTO #_UNDEFINED_# 2>NUL || TITLE %COMSPEC% &" && prefix != "ENDLOCAL & GOTO #_UNDEFINED_# 2>NUL || TITLE %COMSPEC% & SET PATHEXT=%PATHEXT:;.JS;=;% &" {
				return unsupported()
			}
			continue
		}
		if line == "" || npmNativeTail.MatchString(line) {
			continue
		}
		switch upper {
		case "@ECHO OFF", "GOTO START", ":FIND_DP0", "SET DP0=%~DP0", "EXIT /B", ":START", "SETLOCAL", "CALL :FIND_DP0", `IF EXIST "%DP0%\NODE.EXE" (`, `SET "_PROG=%DP0%\NODE.EXE"`, ") ELSE (", `SET "_PROG=NODE"`, "SET PATHEXT=%PATHEXT:;.JS;=;%", ")":
		default:
			return unsupported()
		}
	}
	var relative string
	var prefix []string
	if node != nil {
		relative = node[2]
		if strings.ContainsAny(node[1], `%&|<>^"`) {
			return unsupported()
		}
		prefix = strings.Fields(node[1])
	} else {
		relative = native[1]
	}
	if strings.ContainsAny(relative, "%\x00\r\n") {
		return unsupported()
	}
	target := filepath.Clean(filepath.Join(filepath.Dir(bin), filepath.FromSlash(strings.ReplaceAll(relative, `\`, "/"))))
	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("npm adapter entrypoint is missing: %s", target)
	}
	if node == nil {
		ext := strings.ToLower(filepath.Ext(target))
		if ext != ".exe" && ext != ".com" {
			return unsupported()
		}
		return target, args, nil
	}
	runtime := filepath.Join(filepath.Dir(bin), "node.exe")
	if _, err := os.Stat(runtime); err != nil {
		runtime, err = LookPathEnv("node", env)
		if err != nil {
			return "", nil, err
		}
	}
	return runtime, append(append(prefix, target), args...), nil
}
