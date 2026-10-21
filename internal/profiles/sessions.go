package profiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/tool"
)

// Mode says whether aims moves a session between accounts.
type Mode string

const (
	// Watched: aims moves the session to the next account at a limit and
	// back once the first works again.
	Watched Mode = "watched"
	// Pinned: started as tool@profile, the session stays on that account.
	Pinned Mode = "pinned"
	// Unwatched: automatic moves are off, or aims cannot follow this kind
	// of session; it stays on its account.
	Unwatched Mode = "unwatched"
)

// OpenSession is an interactive session aims started and has not seen end.
type OpenSession struct {
	PID     int       `json:"pid"`
	Tool    tool.ID   `json:"tool"`
	Profile string    `json:"profile"`
	Mode    Mode      `json:"mode"`
	Dir     string    `json:"dir,omitempty"`
	Started time.Time `json:"started"`
}

func sessionsDir() string { return filepath.Join(config.Home(), "sessions") }

func sessionFile() string {
	return filepath.Join(sessionsDir(), strconv.Itoa(os.Getpid())+".json")
}

// Track records the session this aims runs, again after each move.
func Track(s OpenSession) {
	s.PID = os.Getpid()
	if b, err := json.Marshal(s); err == nil {
		_ = fsx.WriteFile(sessionFile(), b, 0o600)
	}
}

// Untrack forgets it once it ends.
func Untrack() { _ = os.Remove(sessionFile()) }

// OpenSessions lists the sessions still running, oldest first, and forgets
// those whose aims died without saying so.
func OpenSessions() []OpenSession {
	entries, _ := os.ReadDir(sessionsDir())
	var out []OpenSession
	for _, e := range entries {
		path := filepath.Join(sessionsDir(), e.Name())
		pid, err := strconv.Atoi(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			continue
		}
		var s OpenSession
		b, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(b, &s) != nil || s.PID != pid || !proc.Alive(pid) {
			_ = os.Remove(path)
			continue
		}
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b OpenSession) int { return a.Started.Compare(b.Started) })
	return out
}
