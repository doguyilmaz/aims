package ops

import (
	"os"
	"testing"

	"github.com/doguyilmaz/aims/internal/schedule"
)

func TestHealKeepAlive(t *testing.T) {
	if err := HealKeepAlive(); err != nil {
		t.Fatal(err)
	}
	if _, on := schedule.Installed(KeepAliveName); on {
		t.Fatal("a keepalive nobody turned on was set up")
	}
	path, err := InstallKeepAlive()
	if err != nil {
		t.Fatal(err)
	}
	// What brew upgrade does through the cask's uninstall stanza.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := HealKeepAlive(); err != nil {
		t.Fatal(err)
	}
	if _, on := schedule.Installed(KeepAliveName); !on {
		t.Fatal("the keepalive Homebrew removed did not come back")
	}
	if _, _, err := UninstallKeepAlive(); err != nil {
		t.Fatal(err)
	}
	if err := HealKeepAlive(); err != nil {
		t.Fatal(err)
	}
	if _, on := schedule.Installed(KeepAliveName); on {
		t.Fatal("a keepalive turned off came back")
	}
}

func TestHealKeepAliveFromAnOlderAims(t *testing.T) {
	path, err := schedule.Install(KeepAliveJob())
	if err != nil {
		t.Fatal(err)
	}
	if err := HealKeepAlive(); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(path)
	if err := HealKeepAlive(); err != nil {
		t.Fatal(err)
	}
	if _, on := schedule.Installed(KeepAliveName); !on {
		t.Fatal("a keepalive an older aims turned on is not known as wanted")
	}
	_, _, _ = UninstallKeepAlive()
}
