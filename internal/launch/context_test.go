package launch

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tools"
)

func TestCanceledHeadlessRequestDoesNotStartTool(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain-tool-fallback", true: "configured-profile"}[configured], func(t *testing.T) {
			var o Options
			if configured {
				o = setup(t)
				o.Profile = "a"
			} else {
				testutil.Sandbox(t)
				o.Tool = tools.Get("codex")
			}
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv(o.Tool.Layout().BinEnv, self)
			o.Headless, o.NoStdin = true, true
			o.Stdout, o.Stderr = io.Discard, io.Discard
			// This test binary would fail parsing these args if it were run.
			o.Args = []string{"--aims-must-not-start-canceled-request"}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if _, err := Run(ctx, o); !errors.Is(err, context.Canceled) {
				t.Fatalf("headless path lost request context: %v", err)
			}
		})
	}
}
