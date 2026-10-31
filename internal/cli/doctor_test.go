package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestDoctorDuplicateAccounts(t *testing.T) {
	evaluations := []profiles.Evaluation{
		{Name: "personal", Account: tool.Account{Email: "me@example.com", Tenant: "personal"}},
		{Name: "work", Account: tool.Account{Email: "me@example.com", Tenant: "company"}},
		{Name: "unknown", Account: tool.Account{Email: "me@example.com"}},
		{Name: "unknown-again", Account: tool.Account{Email: "me@example.com"}},
		{Name: "stale", Account: tool.Account{Email: "me@example.com", Tenant: "personal", Stale: true}},
	}
	if got := duplicateAccounts(evaluations); len(got) != 0 {
		t.Fatalf("distinct or uncertain identities reported as duplicates: %v", got)
	}
	evaluations = append(evaluations,
		profiles.Evaluation{Name: "personal-copy", Account: tool.Account{Email: "ME@example.com", Tenant: "personal"}},
		profiles.Evaluation{Name: "work-copy", Account: tool.Account{Email: "me@example.com", Tenant: "company"}},
	)
	want := [][]string{{"personal", "personal-copy"}, {"work", "work-copy"}}
	if got := duplicateAccounts(evaluations); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDoctorMixedSharing(t *testing.T) {
	cfg := &config.Config{Tools: map[tool.ID]*config.Tool{"claude": {Profiles: map[string]*config.Profile{
		"personal": {}, "work": {}, "client": {Share: config.ShareNone},
	}}}}
	evs := []profiles.Evaluation{
		{Name: "personal", Account: tool.Account{Plan: "max"}},
		{Name: "work", Account: tool.Account{Plan: "team"}},
		{Name: "client", Account: tool.Account{Plan: "enterprise"}},
	}
	work, personal, what := mixedSharing(cfg, "claude", evs)
	if work != "work" || personal != "personal" || !strings.HasPrefix(what, "conversations") {
		t.Fatalf("got %q %q %q", work, personal, what)
	}
	cfg.Tools["claude"].Profiles["work"].Share = config.ShareSettings
	if _, _, what := mixedSharing(cfg, "claude", evs); strings.Contains(what, "conversations") {
		t.Errorf("work keeps its own conversations, got %q", what)
	}
	cfg.Tools["claude"].Profiles["work"].Share = config.ShareNone
	if work, _, _ := mixedSharing(cfg, "claude", evs); work != "" {
		t.Errorf("isolated accounts reported as sharing: %q", work)
	}
}
