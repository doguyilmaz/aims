package ops

import (
	"testing"

	"github.com/doguyilmaz/aims/internal/testutil"
)

func TestMain(m *testing.M) { testutil.Main(m) }

func TestShellJoin(t *testing.T) {
	cases := map[string][]string{
		"/usr/local/bin/aims statusline":                 {"/usr/local/bin/aims", "statusline"},
		`'/opt/my apps/aims' statusline`:                 {"/opt/my apps/aims", "statusline"},
		`'/opt/{a,b}/aims' mcp`:                          {"/opt/{a,b}/aims", "mcp"},
		`'/home/o'\''neil/[x]/aims#1' statusline`:        {"/home/o'neil/[x]/aims#1", "statusline"},
		`'C:\Users\me\aims.exe' statusline`:              {`C:\Users\me\aims.exe`, "statusline"},
		`'~/bin/aims' ''`:                                {"~/bin/aims", ""},
		"/Users/me/.local/bin/aims-2.0_x+y@z statusline": {"/Users/me/.local/bin/aims-2.0_x+y@z", "statusline"},
	}
	for want, parts := range cases {
		if got := shellJoin(parts); got != want {
			t.Errorf("shellJoin(%q) = %s, want %s", parts, got, want)
		}
	}
}
