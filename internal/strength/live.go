package strength

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"
)

// Live sync: 99-Strength tells tccd when something changed, so a water
// logged in the web app shows in Terran CC within a second or two instead
// of at the next refresh.
//
// tccd holds GET /sync/events open (Server-Sent Events). Each "changed"
// event names the areas that moved; habits are read again for "habits", the
// plan for "plan", "workout", "cardio" or "mobility". A 99-Strength without
// the stream (404) is polled at /sync/version instead, and one without that
// either is left to the ordinary refresh in Run.

// liveState is the event stream's bookkeeping, under Service.mu.
type liveState struct {
	pollEvery  time.Duration
	retryMin   time.Duration
	streamIdle time.Duration
	wake       chan struct{} // connect, disconnect or a new token
	cancel     context.CancelFunc

	version  int64            // the last version seen, 0 before any
	areas    map[string]int64 // the last /sync/version areas
	lastID   string           // Last-Event-ID for a reconnect
	verETag  string
	mode     string // "stream", "poll", "off" (neither exists) or "" (not running)
	sseRetry time.Time
}

func newLiveState(o Options) liveState {
	l := liveState{pollEvery: o.PollEvery, retryMin: o.RetryMin, streamIdle: o.StreamIdle, wake: make(chan struct{}, 1)}
	if l.pollEvery <= 0 {
		l.pollEvery = 20 * time.Second
	}
	if l.retryMin <= 0 {
		l.retryMin = time.Second
	}
	if l.streamIdle <= 0 {
		l.streamIdle = 75 * time.Second // three missed 25 s keepalives
	}
	return l
}

// restartLiveLocked ends the stream or polling now; Live starts again for
// the connection as it now is (or waits, when there is none).
func (s *Service) restartLiveLocked() {
	if s.live.cancel != nil {
		s.live.cancel()
		s.live.cancel = nil
	}
	s.live.version, s.live.areas, s.live.lastID, s.live.verETag, s.live.sseRetry = 0, nil, "", "", time.Time{}
	select {
	case s.live.wake <- struct{}{}:
	default:
	}
}

// LiveMode is how tccd currently hears about changes: "stream", "poll",
// "off" or "" while not connected.
func (s *Service) LiveMode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.live.mode
}

var (
	errNoStream = errors.New("99-Strength has no event stream")
	errNoSync   = errors.New("99-Strength has no sync version")
)

// sseRetryAfter is how long a 99-Strength without the event stream is
// polled before the stream is tried again (it may have been deployed since).
const sseRetryAfter = 10 * time.Minute

// Live keeps tccd up to date with 99-Strength until ctx ends. tccd runs it
// next to Run.
func (s *Service) Live(ctx context.Context) {
	failures := 0
	for ctx.Err() == nil {
		s.mu.Lock()
		ok := s.cfg.Connected && !s.cfg.NeedsToken
		cfg, epoch := s.cfg, s.epoch
		wake := s.live.wake
		var cctx context.Context
		var cancel context.CancelFunc
		if ok {
			cctx, cancel = context.WithCancel(ctx)
			s.live.cancel = cancel
		} else {
			s.live.mode = ""
		}
		usePoll := !s.live.sseRetry.IsZero() && s.now().Before(s.live.sseRetry)
		s.mu.Unlock()
		if !ok {
			select {
			case <-ctx.Done():
			case <-wake:
			}
			continue
		}

		token, err := s.tokens.Get()
		if err != nil {
			cancel()
			s.needToken(epoch, "the stored token is missing")
			continue
		}
		r := &remote{base: cfg.BaseURL, token: token, http: s.http}
		if usePoll {
			if err = s.poll(cctx, r, epoch); err == nil {
				cancel()
				continue // time to try the stream again
			}
		} else {
			var heard bool
			heard, err = s.stream(cctx, r, epoch)
			if heard {
				failures = 0
			}
			if errors.Is(err, errNoStream) {
				s.mu.Lock()
				if s.epoch == epoch {
					s.live.sseRetry = s.now().Add(sseRetryAfter)
				}
				s.mu.Unlock()
				s.logf("strength: no event stream, polling /sync/version every %s", s.live.pollEvery)
				cancel()
				continue
			}
		}
		cancel()
		switch {
		case ctx.Err() != nil:
			return
		case unauthorized(err):
			s.needToken(epoch, "99-Strength no longer accepts the token")
			continue
		case errors.Is(err, errNoSync):
			s.setMode(epoch, "off") // Run's refresh is all there is
			s.wait(ctx, wake, sseRetryAfter)
			s.mu.Lock()
			s.live.sseRetry = time.Time{}
			s.mu.Unlock()
			continue
		}
		if cctx.Err() != nil && ctx.Err() == nil {
			continue // restarted on purpose: go again at once
		}
		failures++
		s.wait(ctx, wake, s.backoff(failures))
	}
}

// backoff is RetryMin doubled per failure, capped at a minute, with ±50%
// jitter so many clients do not reconnect in step.
func (s *Service) backoff(failures int) time.Duration {
	d := s.live.retryMin << min(failures-1, 6)
	d = min(d, time.Minute)
	return d/2 + time.Duration(rand.Int64N(int64(d)))
}

func (s *Service) wait(ctx context.Context, wake chan struct{}, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-wake:
	case <-t.C:
	}
}

func (s *Service) setMode(epoch int, mode string) {
	s.mu.Lock()
	if s.epoch == epoch {
		s.live.mode = mode
	}
	s.mu.Unlock()
}

// stream holds GET /sync/events open and acts on each event. heard is true
// once the stream answered, so the caller can reset its backoff.
func (s *Service) stream(ctx context.Context, r *remote, epoch int) (heard bool, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", r.base+"/sync/events", nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	s.mu.Lock()
	if s.live.lastID != "" {
		req.Header.Set("Last-Event-ID", s.live.lastID)
	}
	s.mu.Unlock()
	// The usual client has a whole-request timeout; a stream has none, and
	// is dropped instead when it goes quiet for StreamIdle.
	client := &http.Client{Transport: r.http.Transport}
	idle := time.AfterFunc(s.live.streamIdle, cancel)
	defer idle.Stop()
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented:
		return false, errNoStream
	case resp.StatusCode != http.StatusOK:
		return false, &HTTPError{Status: resp.StatusCode}
	case !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream"):
		return false, errNoStream // a proxy's page, say: not a stream
	}
	s.setMode(epoch, "stream")
	heard = true

	var event, data, id string
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 4096), 64<<10)
	for sc.Scan() {
		idle.Reset(s.live.streamIdle)
		line := sc.Text()
		if line == "" { // end of one event
			if id != "" {
				s.mu.Lock()
				if s.epoch == epoch {
					s.live.lastID = id
				}
				s.mu.Unlock()
			}
			if data != "" || event != "" {
				s.onEvent(ctx, epoch, event, data)
			}
			event, data, id = "", "", ""
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue // a keepalive
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			event = value
		case "data":
			if data != "" {
				data += "\n"
			}
			data += value
		case "id":
			id = value
		}
	}
	if err := sc.Err(); err != nil {
		return true, err
	}
	return true, errors.New("99-Strength closed the event stream")
}

func (s *Service) onEvent(ctx context.Context, epoch int, event, data string) {
	var msg struct {
		Version int64    `json:"version"`
		Areas   []string `json:"areas"`
	}
	if json.Unmarshal([]byte(data), &msg) != nil {
		return
	}
	switch event {
	case "hello":
		// Something may have changed while the stream was down: read
		// everything again unless this is the version already seen.
		s.seen(ctx, epoch, msg.Version, nil)
	case "changed":
		s.seen(ctx, epoch, msg.Version, msg.Areas)
	}
}

// seen records a version and reads again what it says changed. areas nil
// means "unknown": everything is read again if the version is new.
func (s *Service) seen(ctx context.Context, epoch int, version int64, areas []string) {
	s.mu.Lock()
	if s.epoch != epoch {
		s.mu.Unlock()
		return
	}
	known := s.live.version
	if version > s.live.version {
		s.live.version = version
	}
	s.mu.Unlock()
	if areas == nil {
		if version != 0 && version == known {
			return
		}
		areas = []string{"habits", "plan"}
	}
	s.refreshAreas(ctx, areas)
}

func (s *Service) refreshAreas(ctx context.Context, areas []string) {
	habits, plan := false, false
	for _, a := range areas {
		switch strings.ToLower(a) {
		case "habits":
			habits = true
		case "plan", "workout", "cardio", "mobility":
			plan = true
		}
	}
	if habits {
		s.Sync(ctx) // a conditional read; it notifies when the widget changes
	}
	if plan {
		if err := s.RefreshTraining(ctx); err != nil && unauthorized(err) {
			s.mu.Lock()
			epoch := s.epoch
			s.mu.Unlock()
			s.needToken(epoch, "99-Strength no longer accepts the token")
		}
	}
}

// poll asks /sync/version every PollEvery until it is time to try the
// stream again. It returns errNoSync when there is no such route either.
func (s *Service) poll(ctx context.Context, r *remote, epoch int) error {
	s.setMode(epoch, "poll")
	t := time.NewTicker(s.live.pollEvery)
	defer t.Stop()
	for {
		s.mu.Lock()
		etag, done := s.live.verETag, !s.now().Before(s.live.sseRetry)
		s.mu.Unlock()
		if done {
			return nil
		}
		v, tag, err := r.syncVersion(ctx, etag)
		switch {
		case errors.Is(err, errNotModified):
		case statusOf(err) == http.StatusNotFound:
			return errNoSync
		case unauthorized(err):
			return err
		case err != nil:
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// A blip: try again next tick.
		default:
			s.mu.Lock()
			if s.epoch != epoch {
				s.mu.Unlock()
				return nil
			}
			old := s.live.areas
			s.live.verETag, s.live.areas = tag, v.Areas
			s.mu.Unlock()
			if old == nil {
				s.seen(ctx, epoch, v.Version, nil)
			} else {
				var changed []string
				for a, n := range v.Areas {
					if old[a] != n {
						changed = append(changed, a)
					}
				}
				if len(changed) > 0 {
					s.seen(ctx, epoch, v.Version, changed)
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}
