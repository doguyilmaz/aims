package launch

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/tool"
)

// Keep the analyzer in structured mode while this test binary accepts only
// Go's own test flags. No provider or account credentials are used.
type structuredOutputAdapter struct{ tool.Adapter }

func (a structuredOutputAdapter) NewAnalyzer([]string) tool.Analyzer {
	return a.Adapter.NewAnalyzer([]string{"--json"})
}

const outputFailure = "{\"type\":\"error\",\"message\":\"You've hit your usage limit\"}\n"

func TestOversizedOutputWorker(t *testing.T) {
	mode := os.Getenv("AIMS_TEST_OUTPUT_WORKER")
	if mode == "" {
		return
	}
	if mode == "large" {
		_, _ = io.WriteString(os.Stderr, strings.Repeat("x", 2<<20)+"\n")
	}
	_, _ = io.WriteString(os.Stdout, outputFailure)
	os.Exit(1)
}

func TestHeadlessDoesNotRetryAfterSkippingOutputAnalysis(t *testing.T) {
	for _, large := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete-output-can-retry", true: "oversized-stderr-prevents-retry"}[large], func(t *testing.T) {
			o := setup(t)
			o.Tool = structuredOutputAdapter{o.Tool}
			o.Args = []string{"-test.run=^TestOversizedOutputWorker$"}
			var out bytes.Buffer
			o.Stdout, o.Stderr = &out, io.Discard
			cfg, err := config.Load()
			if err != nil {
				t.Fatal(err)
			}
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			mode := map[bool]string{false: "small", true: "large"}[large]
			env := append(os.Environ(), "AIMS_TEST_OUTPUT_WORKER="+mode)
			res, next, err := attempt(t.Context(), o, cfg, self, env, nil, "a", true, []string{"a"})
			if err != nil || res.Code != 1 || res.Failure != tool.FailLimit {
				t.Fatalf("result=%+v, err=%v", res, err)
			}
			if large {
				if next.Name != "" || out.String() != outputFailure {
					t.Fatalf("incomplete analysis retried or changed raw output: next=%q, output=%q", next.Name, out.String())
				}
			} else if next.Name != "b" || out.Len() != 0 {
				t.Fatalf("complete limit response lost its retry: next=%q, output=%q", next.Name, out.String())
			}
		})
	}
}
