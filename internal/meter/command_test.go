package meter

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
)

func TestMeterOutputWorker(t *testing.T) {
	switch os.Getenv("AIMS_TEST_METER_OUTPUT") {
	case "success":
		fmt.Print(`{"reported":true}`)
		os.Exit(0)
	case "failure":
		fmt.Print(`{"reported":true}`)
		os.Exit(1)
	}
}

func TestMeterRejectsNonzeroExitWithPlausibleJSON(t *testing.T) {
	t.Setenv(config.HomeVar, t.TempDir())
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"failure", "success"} {
		t.Setenv("AIMS_TEST_METER_OUTPUT", mode)
		out, err := run(t.Context(), self, "-test.run=^TestMeterOutputWorker$")
		if mode == "failure" {
			if err == nil || len(out) != 0 {
				t.Fatalf("failed command became a successful reading: %q %v", out, err)
			}
		} else if err != nil || !bytes.Equal(out, []byte(`{"reported":true}`)) {
			t.Fatalf("successful output lost: %q %v", out, err)
		}
	}
}
