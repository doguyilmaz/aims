package ui

import (
	"io"
	"testing"

	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestStatusKeyDistinguishesUnknownFromZeroAndBlocked(t *testing.T) {
	s := NewStyles(io.Discard)
	for _, tc := range []struct {
		windows []tool.Window
		want    string
	}{
		{[]tool.Window{{Label: "7d", PercentUnknown: true}}, "7d —"},
		{[]tool.Window{{Label: "7d"}}, "7d 0%"},
		{[]tool.Window{{Label: "7d", PercentUnknown: true}, {Label: "5h", Percent: 20}}, "5h 20%"},
		{[]tool.Window{{Label: "7d", PercentUnknown: true, Limited: true}}, "7d limited"},
	} {
		if got := Key(s, ops.ProfileStatus{Usage: tc.windows}, 95); got != tc.want {
			t.Fatalf("usage=%+v: got %q, want %q", tc.windows, got, tc.want)
		}
	}
}
