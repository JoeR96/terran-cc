package osrs

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Config is what Joe can change: whose data to read and what to track.
type Config struct {
	Account  string        `json:"account"`
	RuneLite string        `json:"runelite_dir,omitempty"` // default ~/.runelite
	Stock    []StockItem   `json:"stock"`
	AFK      []string      `json:"afk"`
	Targets  []SkillTarget `json:"targets"` // empty until the first hiscores read fills in Base 90s
}

func DefaultConfig() Config {
	return Config{Account: "big chopski", Stock: DefaultStock(), AFK: append([]string(nil), DefaultAFK...)}
}

// Status is everything the widget shows.
type Status struct {
	Account  string          `json:"account"`
	Updated  Updated         `json:"updated"`
	Runs     []Run           `json:"runs"`
	Stock    []Stock         `json:"stock"`
	AFK      []string        `json:"afk"`
	Skills   []SkillProgress `json:"skills"`
	Bones    Bones           `json:"bones"`
	Warnings []string        `json:"warnings"`
}

type Updated struct {
	Timers *time.Time `json:"timers"` // newest change RuneLite recorded
	Bank   *time.Time `json:"bank"`   // when the bank export was written
	Skills *time.Time `json:"skills"` // when the hiscores were read
	Log    *time.Time `json:"log"`    // collection log and diaries
}

// Service keeps the latest view of the RuneLite files and hiscores.
type Service struct {
	Save        func(Config) error
	OnChange    func()    // something the widget shows changed
	OnReady     func(Run) // a run went from growing to ready
	Now         func() time.Time
	HTTP        *http.Client
	HiscoresURL string
	Version     string

	mu       sync.Mutex
	cfg      Config
	timers   timers
	found    bool // the account was found in RuneLite's profile
	have     map[string]int
	bankAt   time.Time
	skills   Skills
	skillsAt time.Time
	mods     map[string]time.Time
	states   map[string]string
	obtained map[string]bool
	names    map[int]string
	diaries  map[string]Diary
	extrasAt time.Time
}

func NewService(cfg Config) *Service {
	if cfg.Account == "" {
		cfg.Account = DefaultConfig().Account
	}
	if cfg.Stock == nil {
		cfg.Stock = DefaultStock()
	}
	if cfg.AFK == nil {
		cfg.AFK = append([]string(nil), DefaultAFK...)
	}
	return &Service{cfg: cfg, Now: time.Now, HTTP: &http.Client{Timeout: 20 * time.Second}, mods: map[string]time.Time{}, states: map[string]string{}}
}

func (s *Service) dir() string {
	if s.cfg.RuneLite != "" {
		return s.cfg.RuneLite
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".runelite")
}

func (s *Service) propsPath() string {
	return filepath.Join(s.dir(), "profiles2", "$rsprofile--1.properties")
}

// containers are the Data Export files whose items count as stock.
func (s *Service) containers() []string {
	d := filepath.Join(s.dir(), "Data Exports")
	return []string{filepath.Join(d, "container_bank.json"), filepath.Join(d, "container_inventory.json"), filepath.Join(d, "container_equipment.json")}
}

func (s *Service) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// SetConfig replaces the config, saves it and rereads everything.
func (s *Service) SetConfig(c Config) error {
	s.mu.Lock()
	if c.Account == "" {
		c.Account = s.cfg.Account
	}
	s.cfg = c
	s.mods = map[string]time.Time{}
	s.mu.Unlock()
	if s.Save != nil {
		if err := s.Save(c); err != nil {
			return err
		}
	}
	s.refresh()
	s.changed()
	return nil
}

func (s *Service) changed() {
	if s.OnChange != nil {
		s.OnChange()
	}
}

// modified reports whether path changed since the last look.
func (s *Service) modified(path string) (time.Time, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}, false
	}
	if s.mods[path].Equal(fi.ModTime()) {
		return fi.ModTime(), false
	}
	s.mods[path] = fi.ModTime()
	return fi.ModTime(), true
}

// refresh rereads any file that changed and reports whether one did.
func (s *Service) refresh() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	any := false
	if _, ok := s.modified(s.propsPath()); ok {
		if f, err := os.Open(s.propsPath()); err == nil {
			props := readProperties(f)
			f.Close()
			key := profileKey(props, s.cfg.Account)
			s.found = key != ""
			s.timers = readTimers(props, key)
			any = true
		}
	}
	bankChanged := false
	for _, p := range s.containers() {
		if _, ok := s.modified(p); ok {
			bankChanged = true
		}
	}
	if bankChanged {
		have := map[string]int{}
		for i, p := range s.containers() {
			if f, err := os.Open(p); err == nil {
				readContainer(f, have)
				f.Close()
				if i == 0 {
					if fi, err := os.Stat(p); err == nil {
						s.bankAt = fi.ModTime()
					}
				}
			}
		}
		s.have = have
		any = true
	}
	return any
}

// readSkills reads the hiscores, fills in default targets and records the
// start of any new target.
func (s *Service) readSkills(ctx context.Context) {
	s.mu.Lock()
	account := s.cfg.Account
	s.mu.Unlock()
	sk, err := FetchSkills(ctx, s.HTTP, s.HiscoresURL, account, s.Version)
	if err != nil {
		log.Printf("osrs: %v", err)
		return
	}
	s.mu.Lock()
	s.skills, s.skillsAt = sk, s.Now()
	if len(s.cfg.Targets) == 0 {
		s.cfg.Targets = DefaultTargets(sk)
	}
	_, changed := Progress(s.cfg.Targets, sk)
	cfg := s.cfg
	s.mu.Unlock()
	if changed && s.Save != nil {
		if err := s.Save(cfg); err != nil {
			log.Printf("osrs: save targets: %v", err)
		}
	}
	s.changed()
}

// readExtras reads the collection log and diary progress.
func (s *Service) readExtras(ctx context.Context) {
	s.mu.Lock()
	account, names := s.cfg.Account, s.names
	s.mu.Unlock()
	got, names, err := fetchObtained(ctx, s.HTTP, account, s.Version, names)
	if err != nil {
		log.Printf("osrs: collection log: %v", err)
	}
	diaries, derr := fetchDiaries(ctx, s.HTTP, account, s.Version)
	if derr != nil {
		log.Printf("osrs: diaries: %v", derr)
	}
	s.mu.Lock()
	s.names = names
	if err == nil {
		s.obtained = got
	}
	if derr == nil {
		s.diaries = diaries
	}
	if err == nil || derr == nil {
		s.extrasAt = s.Now()
	}
	s.mu.Unlock()
	s.changed()
}

// Facts is what the game data says, for Plan.
func (s *Service) Facts() Facts {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Facts{Obtained: s.obtained, Have: s.have, XP: s.skills, Diaries: s.diaries}
}

// Status is the current view.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Account: s.cfg.Account, AFK: s.cfg.AFK, Runs: s.timers.Runs(s.Now()), Warnings: []string{}}
	if !s.found {
		st.Warnings = append(st.Warnings, "RuneLite has no time-tracking data for "+s.cfg.Account+" yet")
	}
	if !s.timers.Newest.IsZero() {
		t := s.timers.Newest
		st.Updated.Timers = &t
	}
	if !s.extrasAt.IsZero() {
		t := s.extrasAt
		st.Updated.Log = &t
	}
	st.Bones = bonesIn(s.have)
	if s.have != nil {
		st.Stock = Stocks(s.cfg.Stock, s.have)
		t := s.bankAt
		st.Updated.Bank = &t
	} else {
		st.Stock = Stocks(s.cfg.Stock, map[string]int{})
		st.Warnings = append(st.Warnings, "no bank export yet: turn on RuneLite's Data Export plugin and open your bank")
	}
	if s.skills != nil {
		targets := append([]SkillTarget(nil), s.cfg.Targets...)
		st.Skills, _ = Progress(targets, s.skills)
		t := s.skillsAt
		st.Updated.Skills = &t
	} else {
		st.Skills = []SkillProgress{}
	}
	return st
}

// checkReady notices runs that finished growing since the last check.
func (s *Service) checkReady() bool {
	runs := s.Status().Runs
	s.mu.Lock()
	var ready []Run
	moved := false
	for _, r := range runs {
		prev, seen := s.states[r.Key]
		if seen && prev != r.State {
			moved = true
			if prev == "growing" && r.State == "ready" {
				ready = append(ready, r)
			}
		}
		s.states[r.Key] = r.State
	}
	s.mu.Unlock()
	if s.OnReady != nil {
		for _, r := range ready {
			s.OnReady(r)
		}
	}
	return moved
}

// Poll rereads changed files now, as the watch loop does every 2 seconds.
func (s *Service) Poll() {
	if s.refresh() {
		s.checkReady()
		s.changed()
	}
}

// Run watches the files every 2 seconds, rechecks ready times every 30 and
// reads the hiscores every 10 minutes, until ctx is done.
func (s *Service) Run(ctx context.Context) {
	s.refresh()
	s.checkReady()
	go s.readSkills(ctx)
	go s.readExtras(ctx)
	extras := time.NewTicker(extrasEvery)
	defer extras.Stop()
	files := time.NewTicker(2 * time.Second)
	clock := time.NewTicker(30 * time.Second)
	skills := time.NewTicker(skillsEvery)
	defer files.Stop()
	defer clock.Stop()
	defer skills.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-files.C:
			if s.refresh() {
				s.checkReady()
				s.changed()
			}
		case <-clock.C:
			if s.checkReady() {
				s.changed()
			}
		case <-skills.C:
			go s.readSkills(ctx)
		case <-extras.C:
			go s.readExtras(ctx)
		}
	}
}
