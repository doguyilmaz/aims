package ui

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"
)

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spin animates label on one line of w until fn returns, then clears the line.
// It writes plain escape codes rather than running a Bubble Tea program, so
// nothing else touches the terminal while a wizard is on screen.
func spin(w io.Writer, s *Styles, prefix, label string, fn func()) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	// Ctrl+C reaches aims as a signal here (the terminal is not in raw mode):
	// put the cursor back before leaving.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)

	tick := time.NewTicker(80 * time.Millisecond)
	defer tick.Stop()
	fmt.Fprint(w, "\x1b[?25l")
	for i := 0; ; i++ {
		fmt.Fprintf(w, "\r\x1b[2K%s%s %s", prefix, s.Accent.Render(frames[i%len(frames)]), s.Dim.Render(label))
		select {
		case <-done:
			fmt.Fprint(w, "\r\x1b[2K\x1b[?25h")
			return
		case <-sig:
			fmt.Fprint(w, "\r\x1b[2K\x1b[?25h")
			os.Exit(130)
		case <-tick.C:
		}
	}
}
