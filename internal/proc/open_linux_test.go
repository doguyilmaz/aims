package proc

import "testing"

func TestIncompleteInspectionRetriesWithoutAssumingIdle(t *testing.T) {
	type result struct{ held, known bool }
	for _, tc := range []struct {
		name    string
		results []result
		want    result
	}{
		{"persistent-unknown", []result{{false, false}}, result{false, false}},
		{"becomes-inspectable", []result{{false, false}, {false, true}}, result{false, true}},
		{"finds-live-holder", []result{{false, false}, {true, true}}, result{true, true}},
		{"known-holder", []result{{true, true}}, result{true, true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			held, known := retryInspection(func() (bool, bool) {
				i := min(calls, len(tc.results)-1)
				calls++
				r := tc.results[i]
				return r.held, r.known
			})
			if (result{held, known}) != tc.want {
				t.Fatalf("inspection changed certainty: held=%v known=%v", held, known)
			}
			if calls > 3 || tc.name == "known-holder" && calls != 1 {
				t.Fatalf("inspection did not respect its retry bound: %d calls", calls)
			}
		})
	}
}
