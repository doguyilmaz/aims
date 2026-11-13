package proc

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNPMShimUsesBundledRuntimeAndLiteralArguments(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "node_modules", "adapter", "bin", "agent.js")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	node := filepath.Join(dir, "node.exe")
	if err := os.WriteFile(node, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(dir, "adapter.cmd")
	text := "@ECHO off\r\nGOTO start\r\n:find_dp0\r\nSET dp0=%~dp0\r\nEXIT /b\r\n:start\r\nSETLOCAL\r\nCALL :find_dp0\r\nIF EXIST \"%dp0%\\node.exe\" (\r\n SET \"_prog=%dp0%\\node.exe\"\r\n) ELSE (\r\n SET \"_prog=node\"\r\n SET PATHEXT=%PATHEXT:;.JS;=;%\r\n)\r\nendLocal & goto #_undefined_# 2>NUL || title %COMSPEC% & \"%_prog%\"  \"%dp0%\\node_modules\\adapter\\bin\\agent.js\" %*\r\n"
	if err := os.WriteFile(shim, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{`a&b`, `%SECRET%`, `{"literal":"quotes"}`, "line\nbreak"}
	bin, got, err := npmShimCommand(shim, args, nil)
	want := append([]string{script}, args...)
	if err != nil || bin != node || !reflect.DeepEqual(got, want) {
		t.Fatalf("shim did not preserve runtime/arguments: %s %q %v", bin, got, err)
	}
	if err := os.WriteFile(shim, []byte("echo do-something-else\n"+text), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := npmShimCommand(shim, args, nil); err == nil {
		t.Fatal("silently skipped custom batch behavior")
	}
}
