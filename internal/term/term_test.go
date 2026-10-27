package term

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeProc is a terminal process whose output the test writes and whose
// input the test reads.
type fakeProc struct {
	out      *io.PipeReader // what the process prints
	outW     *io.PipeWriter
	mu       sync.Mutex
	input    strings.Builder
	cols     int
	rows     int
	done     chan struct{}
	doneOnce sync.Once
}

func newFake() *fakeProc {
	r, w := io.Pipe()
	return &fakeProc{out: r, outW: w, done: make(chan struct{})}
}

func (f *fakeProc) Read(p []byte) (int, error) { return f.out.Read(p) }
func (f *fakeProc) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.input.Write(p)
}
func (f *fakeProc) Resize(c, r int) error {
	f.mu.Lock()
	f.cols, f.rows = c, r
	f.mu.Unlock()
	return nil
}
func (f *fakeProc) Wait(ctx context.Context) (uint32, error) {
	<-f.done
	return 0, nil
}
func (f *fakeProc) Close() error { f.exit(); return nil }
func (f *fakeProc) exit() {
	f.doneOnce.Do(func() {
		close(f.done)
		f.outW.Close()
	})
}
func (f *fakeProc) typed() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.input.String()
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestSessionReplaysAndBroadcasts(t *testing.T) {
	var procs []*fakeProc
	var resumed []bool
	s := New(func(cols, rows int, resume bool) (Proc, error) {
		p := newFake()
		procs = append(procs, p)
		resumed = append(resumed, resume)
		return p, nil
	})
	defer s.Close()

	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	procs[0].outW.Write([]byte("Welcome to Claude Code\r\n"))
	waitFor(t, "scrollback", func() bool { return strings.Contains(string(s.Scrollback()), "Welcome") })

	// A client joining late gets the scrollback, then live output.
	ch, cancel := s.Subscribe()
	defer cancel()
	procs[0].outW.Write([]byte("> "))
	select {
	case b := <-ch:
		if string(b) != "> " {
			t.Fatalf("live %q", b)
		}
	case <-time.After(time.Second):
		t.Fatal("no live output")
	}

	s.Input([]byte("add fix fence\r"))
	s.Resize(100, 30)
	waitFor(t, "input", func() bool { return procs[0].typed() == "add fix fence\r" })
	if procs[0].cols != 100 || procs[0].rows != 30 {
		t.Fatalf("resize %d %d", procs[0].cols, procs[0].rows)
	}

	// When Claude exits, the next keypress starts it again, resuming.
	procs[0].exit()
	waitFor(t, "exit noticed", func() bool { return !s.Running() })
	if !strings.Contains(string(s.Scrollback()), "exited") {
		t.Fatalf("no exit notice in %q", s.Scrollback())
	}
	s.Input([]byte("\r"))
	waitFor(t, "restart", func() bool { return len(procs) == 2 && s.Running() })
	if resumed[0] || !resumed[1] {
		t.Fatalf("resume flags %v: first start fresh, restarts continue", resumed)
	}
	if procs[1].typed() != "" {
		t.Fatalf("the restart key should not reach the new process: %q", procs[1].typed())
	}
}

func TestScrollbackIsCapped(t *testing.T) {
	s := New(nil)
	s.append([]byte(strings.Repeat("x", maxScrollback+500)))
	if len(s.Scrollback()) != maxScrollback {
		t.Fatalf("scrollback %d", len(s.Scrollback()))
	}
}

func TestSetupDirKeepsEdits(t *testing.T) {
	dir := t.TempDir()
	if err := SetupDir(dir); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if !strings.Contains(string(b), "tcc todo") {
		t.Fatalf("CLAUDE.md %q", b)
	}
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("mine"), 0o644)
	SetupDir(dir)
	if b, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md")); string(b) != "mine" {
		t.Fatal("SetupDir overwrote an edited CLAUDE.md")
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "settings.json")); err != nil {
		t.Fatal(err)
	}
}
