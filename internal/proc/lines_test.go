package proc

import (
	"bytes"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
)

func TestPumpBoundsAnalysisAndPreservesRawOutput(t *testing.T) {
	large := strings.Repeat("x", maxAnalysisLineBytes+1)
	input := "before\r\n" + large + "fake event suffix\nafter\n" + large
	for _, fragmented := range []bool{false, true} {
		t.Run(map[bool]string{false: "chunks", true: "fragmented"}[fragmented], func(t *testing.T) {
			var r io.Reader = strings.NewReader(input)
			if fragmented {
				r = iotest.HalfReader(r)
			}
			var raw bytes.Buffer
			var lines []string
			skipped := pump(r, func(b []byte) { raw.Write(b) }, func(s string) { lines = append(lines, s) })
			if !skipped || !slices.Equal(lines, []string{"before", "after"}) {
				t.Fatalf("skipped=%v, reported line lengths=%v", skipped, lineLengths(lines))
			}
			if raw.String() != input {
				t.Fatalf("raw output changed: got %d bytes, want %d", raw.Len(), len(input))
			}
		})
	}
}

func TestPumpKeepsAnalysisAtTheLimit(t *testing.T) {
	line := strings.Repeat("x", maxAnalysisLineBytes)
	for _, ending := range []string{"", "\n", "\r\n", "\r"} {
		for _, fragmented := range []bool{false, true} {
			var r io.Reader = strings.NewReader(line + ending)
			if fragmented {
				// Put the content, CR and LF in independent reads.
				cut := min(1, len(ending))
				r = io.MultiReader(strings.NewReader(line), strings.NewReader(ending[:cut]), strings.NewReader(ending[cut:]))
			}
			var lines []string
			skipped := pump(r, func([]byte) {}, func(s string) { lines = append(lines, s) })
			if ending == "\r" {
				// Without LF, the CR is an extra content byte.
				if !skipped || len(lines) != 0 {
					t.Fatalf("oversized final line ending in CR accepted: fragmented=%v", fragmented)
				}
				continue
			}
			if skipped {
				t.Fatalf("line at limit skipped: ending=%q fragmented=%v", ending, fragmented)
			}
			if !slices.Equal(lines, []string{line}) {
				t.Fatalf("ending=%q fragmented=%v: wrong line lengths %v", ending, fragmented, lineLengths(lines))
			}
		}
	}
}

func lineLengths(lines []string) []int {
	lengths := make([]int, len(lines))
	for i, line := range lines {
		lengths[i] = len(line)
	}
	return lengths
}
