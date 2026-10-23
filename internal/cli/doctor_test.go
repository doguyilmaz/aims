package cli

import (
	"reflect"
	"testing"

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
