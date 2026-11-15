// A test-only CLI/ACP stand-in. Never contacts a provider or reads a login.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("native-provider 1.2.3")
		return
	}
	switch os.Getenv("BRIDGE_NATIVE_MODE") {
	case "spawn":
		cmd := exec.Command("codex", os.Args[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		for _, kv := range os.Environ() {
			if !strings.HasPrefix(kv, "BRIDGE_NATIVE_MODE=") {
				cmd.Env = append(cmd.Env, kv)
			}
		}
		if err := cmd.Run(); err != nil {
			os.Exit(2)
		}
		return
	case "echo":
		fmt.Fprintln(os.Stderr, "native-stderr")
		if _, err := io.Copy(os.Stdout, os.Stdin); err != nil {
			os.Exit(2)
		}
		return
	case "signal":
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		fmt.Println("ready")
		<-ch
		os.Exit(23)
	}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 4096), 4<<20)
	for sc.Scan() {
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		if json.Unmarshal(sc.Bytes(), &req) != nil {
			os.Exit(2)
		}
		result := map[string]any{
			"pid": os.Getpid(), "args": os.Args[1:],
			"profile": os.Getenv("AIMS_SESSION_PROFILE"),
			"mode":    os.Getenv("AIMS_SESSION_MODE"), "run": os.Getenv("AIMS_RUN"),
			"home": os.Getenv("AIMS_HOME"), "codexHome": os.Getenv("CODEX_HOME"),
			"sqliteHome": os.Getenv("CODEX_SQLITE_HOME"), "claudeHome": os.Getenv("CLAUDE_CONFIG_DIR"),
			"codexPath": os.Getenv("CODEX_PATH"), "claudeExecutable": os.Getenv("CLAUDE_CODE_EXECUTABLE"),
			"hasOpenAIKey": os.Getenv("OPENAI_API_KEY") != "", "parentThread": os.Getenv("CODEX_THREAD_ID"),
		}
		if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"id": req.ID, "result": result}); err != nil {
			os.Exit(2)
		}
	}
	if sc.Err() != nil {
		os.Exit(2)
	}
}
