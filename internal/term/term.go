// Package term keeps one long-lived terminal program (Claude Code) running
// for the side widget: it holds the process, its scrollback and every
// connected viewer, so closing a window never ends the session.
package term

import (
	"context"
	"io"
	"sync"
)

// Proc is a program attached to a pseudo-terminal.
type Proc interface {
	io.ReadWriter
	Resize(cols, rows int) error
	Wait(ctx context.Context) (uint32, error)
	Close() error
}

// Starter launches the program. again is true when it is being restarted
// after exiting, so it should pick up where it left off.
type Starter func(cols, rows int, again bool) (Proc, error)

const maxScrollback = 512 << 10

const exitNotice = "\r\n\x1b[33m[Claude exited. Press any key to start it again.]\x1b[0m\r\n"

// Session is the one running program plus what it has printed.
type Session struct {
	start Starter

	mu      sync.Mutex
	proc    Proc
	started bool // has run at least once
	cols    int
	rows    int
	buf     []byte
	subs    map[chan []byte]struct{}
	closed  bool
}

func New(start Starter) *Session {
	return &Session{start: start, cols: 100, rows: 30, subs: map[chan []byte]struct{}{}}
}

// Ensure starts the program if it isn't running.
func (s *Session) Ensure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureLocked()
}

func (s *Session) ensureLocked() error {
	if s.proc != nil || s.closed {
		return nil
	}
	p, err := s.start(s.cols, s.rows, s.started)
	if err != nil {
		return err
	}
	s.proc, s.started = p, true
	go s.pump(p)
	return nil
}

// pump copies the program's output to the scrollback and the viewers, and
// notices when it exits.
func (s *Session) pump(p Proc) {
	b := make([]byte, 32<<10)
	for {
		n, err := p.Read(b)
		if n > 0 {
			s.append(append([]byte(nil), b[:n]...))
		}
		if err != nil {
			break
		}
	}
	p.Wait(context.Background())
	s.mu.Lock()
	if s.proc == p {
		s.proc = nil
	}
	s.mu.Unlock()
	s.append([]byte(exitNotice))
}

func (s *Session) append(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = append(s.buf, b...)
	if over := len(s.buf) - maxScrollback; over > 0 {
		s.buf = append([]byte(nil), s.buf[over:]...)
	}
	for ch := range s.subs {
		select {
		case ch <- b:
		default: // a stalled viewer misses output rather than stalling Claude
		}
	}
}

// Running reports whether the program is up.
func (s *Session) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.proc != nil
}

// Scrollback is everything printed so far, up to the cap.
func (s *Session) Scrollback() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.buf...)
}

// Subscribe returns live output and a func to stop it.
func (s *Session) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 256)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}
}

// Input types into the program. With the program stopped, a keypress
// starts it again instead.
func (s *Session) Input(b []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proc == nil {
		return s.ensureLocked()
	}
	_, err := s.proc.Write(b)
	return err
}

// Resize sets the terminal size; the most recent viewer's size wins.
func (s *Session) Resize(cols, rows int) {
	if cols < 20 || rows < 5 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cols, s.rows = cols, rows
	if s.proc != nil {
		s.proc.Resize(cols, rows)
	}
}

// Close ends the program for good (tccd shutting down).
func (s *Session) Close() {
	s.mu.Lock()
	s.closed = true
	p := s.proc
	s.mu.Unlock()
	if p != nil {
		p.Close()
	}
}
