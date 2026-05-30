package tile

import (
	"bufio"
	"io"
	"regexp"
	"strconv"
	"sync"
)

// Progress is one parsed percentage event from `vips --vips-progress`.
type Progress struct {
	// Percent is in the range [0, 100].
	Percent int
	// Done is true on the final "done in Xs" line.
	Done bool
}

// percentRe matches "12% complete" / "12%complete" / "12 % complete". vips
// itself emits "NN% complete" without a separating space; the looser pattern
// here costs us nothing and survives across vips versions.
var percentRe = regexp.MustCompile(`(\d{1,3})\s*%\s*complete`)

// doneRe matches the closing "done in Xs" message dzsave emits when the
// pyramid is fully written.
var doneRe = regexp.MustCompile(`\bdone in\b`)

// stderrTail buffers the last N lines of dzsave's stderr so callers can attach
// them to a tile/dzsave-failed Problem for diagnosis without dragging the full
// log around. The done channel closes when the underlying read goroutine exits.
type stderrTail struct {
	mu   sync.Mutex
	buf  []string
	cap  int
	done chan struct{}
}

func newStderrTail(cap int) *stderrTail { return &stderrTail{cap: cap} }

func (t *stderrTail) add(line string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, line)
	if len(t.buf) > t.cap {
		t.buf = t.buf[len(t.buf)-t.cap:]
	}
}

func (t *stderrTail) lines() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, len(t.buf))
	copy(out, t.buf)
	return out
}

// Wait blocks until the background reader has drained stderr.
func (t *stderrTail) Wait() {
	if t.done != nil {
		<-t.done
	}
}

// streamProgress reads dzsave's stderr in a background goroutine, calls
// progressFn for every parsed percent or done event, and stores the trailing
// 50 lines in the returned stderrTail. Caller must call Wait on the returned
// tail before reading lines() or calling cmd.Wait, otherwise the reader and
// the process exit can race (the os/exec docs require draining StderrPipe
// before Wait).
func streamProgress(r io.Reader, progressFn func(Progress)) *stderrTail {
	tail := newStderrTail(50)
	tail.done = make(chan struct{})

	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 0, 4096), 1<<20)
	scan.Split(splitCRLF)

	go func() {
		defer close(tail.done)
		for scan.Scan() {
			line := scan.Text()
			tail.add(line)
			if match := percentRe.FindStringSubmatch(line); match != nil {
				if n, err := strconv.Atoi(match[1]); err == nil && progressFn != nil {
					progressFn(Progress{Percent: clampPercent(n)})
				}
			}
			if progressFn != nil && doneRe.MatchString(line) {
				progressFn(Progress{Percent: 100, Done: true})
			}
		}
	}()
	return tail
}

// splitCRLF treats '\n' and '\r' as line boundaries so dzsave's CR-only
// progress writes are surfaced in real time.
func splitCRLF(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func clampPercent(n int) int {
	switch {
	case n < 0:
		return 0
	case n > 100:
		return 100
	default:
		return n
	}
}
