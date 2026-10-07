package launch

import (
	"os"
	"time"
)

// ReadStdin reads piped input. It returns nil for a terminal, and gives up
// when nothing arrives within wait, so an inherited pipe that never closes
// cannot hang aims. With untilEOF, input that has started is read to the end,
// however slowly it comes; otherwise reading also stops wait after the last
// chunk.
func ReadStdin(wait time.Duration, untilEOF bool) (data []byte, timedOut bool) {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return nil, false
	}
	type chunk struct {
		b   []byte
		err error
	}
	ch := make(chan chunk, 16)
	go func() {
		buf := make([]byte, 64<<10)
		for {
			n, err := os.Stdin.Read(buf)
			ch <- chunk{append([]byte(nil), buf[:n]...), err}
			if err != nil {
				return
			}
		}
	}()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	started := false
	for {
		select {
		case c := <-ch:
			data = append(data, c.b...)
			if c.err != nil {
				return data, false // EOF, or a read error: use what arrived
			}
			if len(c.b) > 0 {
				started = true
				if untilEOF {
					timer.Stop()
				} else {
					timer.Reset(wait)
				}
			}
		case <-timer.C:
			if !started {
				return nil, true
			}
			if !untilEOF {
				return data, false
			}
		}
	}
}
