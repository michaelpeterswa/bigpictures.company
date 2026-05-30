package tile

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPercentRe(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"vips temp-2: 14% complete", 14, true},
		{"vips temp-2: 100% complete", 100, true},
		{"vips temp-7: 7%complete", 7, true},
		{"vips temp-3: 67 % complete", 67, true},
		{"vips temp-2: done in 0.0163s", 0, false},
		{"unrelated noise", 0, false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			m := percentRe.FindStringSubmatch(c.in)
			if c.ok {
				if m == nil {
					t.Fatalf("no match")
				}
				got := atoiOr(m[1], -1)
				if got != c.want {
					t.Errorf("got %d, want %d", got, c.want)
				}
				return
			}
			if m != nil {
				t.Fatalf("unexpected match: %v", m)
			}
		})
	}
}

func TestStreamProgress_CRSplit(t *testing.T) {
	// vips writes progress with CRs; ensure we see them as separate events.
	raw := "vips temp-2: 25% complete\rvips temp-2: 50% complete\rvips temp-2: 100% complete\rvips temp-2: done in 0.01s\n"
	pr, pw := io.Pipe()
	var (
		mu   sync.Mutex
		seen []int
		done bool
	)
	fn := func(p Progress) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, p.Percent)
		if p.Done {
			done = true
		}
	}
	streamProgress(pr, fn)
	_, _ = pw.Write([]byte(raw))
	_ = pw.Close()

	// streamProgress runs in a goroutine; poll briefly.
	for i := 0; i < 100; i++ {
		mu.Lock()
		got := len(seen)
		isDone := done
		mu.Unlock()
		if got >= 4 && isDone {
			break
		}
		// tiny sleep without importing time helpers
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	wantPrefix := []int{25, 50, 100}
	for i, w := range wantPrefix {
		if i >= len(seen) || seen[i] != w {
			t.Fatalf("seen = %v, want prefix %v", seen, wantPrefix)
		}
	}
	if !done {
		t.Fatalf("expected Done=true event; seen = %v", seen)
	}
}

func TestStreamProgress_TailKept(t *testing.T) {
	pr, pw := io.Pipe()
	tail := streamProgress(pr, nil)
	lines := []string{"line-a", "line-b", "line-c"}
	_, _ = pw.Write([]byte(strings.Join(lines, "\n") + "\n"))
	_ = pw.Close()
	for i := 0; i < 100; i++ {
		if len(tail.lines()) >= 3 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	got := tail.lines()
	if len(got) != 3 {
		t.Fatalf("tail.lines() = %v, want 3 entries", got)
	}
}

func atoiOr(s string, fallback int) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return fallback
		}
		n = n*10 + int(c-'0')
	}
	return n
}
