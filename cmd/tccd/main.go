// Command tccd is the Terran CC service. It owns the database, the timer and
// the event stream, and serves the HTTP API on 127.0.0.1.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"terrancc/internal/api"
	"terrancc/internal/assist"
	"terrancc/internal/config"
	"terrancc/internal/core"
	"terrancc/internal/events"
	"terrancc/internal/osrs"
	"terrancc/internal/store/sqlite"
	"terrancc/internal/timer"
)

const defaultAddr = "127.0.0.1:47821"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tccd:", err)
		os.Exit(1)
	}
}

func run() (err error) {
	addr := flag.String("addr", envOr("TCC_ADDR", defaultAddr), "listen address")
	homeFlag := flag.String("home", "", "data directory (default $TCC_HOME or the user config dir)")
	flag.Parse()

	if *homeFlag != "" {
		os.Setenv("TCC_HOME", *homeFlag)
	}
	home, err := config.Home()
	if err != nil {
		return err
	}
	logf, err := os.OpenFile(filepath.Join(home, "tccd.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logf.Close()
	log.SetOutput(logf)
	defer func() { // runs before logf closes, so clients can read why we exited
		if err != nil {
			log.Printf("tccd: %v", err)
		}
	}()

	token, err := config.Token(home)
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return portTaken(*addr, home, token, err)
	}

	db, err := sqlite.Open(filepath.Join(home, "tcc.db"))
	if err != nil {
		return err
	}
	defer db.Close()

	svc := &core.Service{Store: db, Clock: core.SystemClock{}}
	sigCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	// POST /api/shutdown cancels this too: a detached tccd gets no Ctrl+C.
	ctx, stop := context.WithCancel(sigCtx)
	defer stop()
	if err := svc.SeedHabits(ctx); err != nil {
		return err
	}

	cfg := timer.DefaultConfig()
	if v, ok, _ := db.GetSetting(ctx, api.TimerConfigKey); ok {
		if err := json.Unmarshal([]byte(v), &cfg); err != nil {
			log.Printf("ignoring bad timer config: %v", err)
			cfg = timer.DefaultConfig()
		}
	}
	tm := timer.New(cfg, time.Now)
	bus := events.NewBus()
	// Decode into an empty config: decoding over the defaults would reuse
	// their list entries by position and leak fields between items.
	var ocfg osrs.Config
	if v, ok, _ := db.GetSetting(ctx, api.OSRSConfigKey); ok {
		if err := json.Unmarshal([]byte(v), &ocfg); err != nil {
			log.Printf("ignoring bad OSRS config: %v", err)
			ocfg = osrs.Config{}
		}
	}
	tracker := osrs.NewService(ocfg)
	tracker.Version = api.Version
	srv := &http.Server{Handler: api.New(svc, tm, bus, token, home, api.WithShutdown(stop), api.WithAssistant(assistant()), api.WithOSRS(tracker, ""), api.WithVault(vaultDir())), ReadHeaderTimeout: 10 * time.Second}
	go tracker.Run(ctx)

	go api.RunTicker(ctx, tm, bus)
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(shut)
	}()

	if err := config.WriteDaemon(home, config.Daemon{Addr: ln.Addr().String(), Token: token, PID: os.Getpid()}); err != nil {
		return err
	}
	defer config.RemoveDaemon(home)
	log.Printf("tccd %s listening on %s (home %s)", api.Version, ln.Addr(), home)

	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Printf("tccd stopped")
	return nil
}

// portTaken handles a failed bind. If our own tccd (same home) holds the port
// but daemon.json is missing, it rewrites daemon.json and exits cleanly.
// Otherwise it explains who holds the port.
func portTaken(addr, home, token string, bindErr error) error {
	c := http.Client{Timeout: time.Second}
	resp, err := c.Get("http://" + addr + "/api/health")
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", addr, bindErr)
	}
	defer resp.Body.Close()
	var h struct {
		Home string `json:"home"`
		PID  int    `json:"pid"`
	}
	json.NewDecoder(resp.Body).Decode(&h)
	if samePath(h.Home, home) {
		log.Printf("tccd (pid %d) is already serving %s; restoring daemon.json", h.PID, addr)
		return config.WriteDaemon(home, config.Daemon{Addr: addr, Token: token, PID: h.PID})
	}
	return fmt.Errorf("another tccd (pid %d, home %s) is serving %s: stop it or set TCC_ADDR to another port", h.PID, h.Home, addr)
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// assistant talks to OpenCode Go. The key is read on each message, from
// TCC_OPENCODE_KEY or opencode's own auth.json, so connecting OpenCode Go
// later needs no restart. TCC_ASSIST_MODEL and TCC_ASSIST_URL override the
// defaults.
func assistant() *assist.Client {
	c := &assist.Client{URL: assist.DefaultURL, Model: assist.DefaultModel, Key: assist.OpencodeKey(assist.AuthPath()), Version: api.Version}
	if v := os.Getenv("TCC_ASSIST_URL"); v != "" {
		c.URL = v
	}
	if v := os.Getenv("TCC_ASSIST_MODEL"); v != "" {
		c.Model = v
	}
	if k := os.Getenv("TCC_OPENCODE_KEY"); k != "" {
		c.Key = func() (string, error) { return k, nil }
	}
	return c
}

// vaultDir is Joe's Obsidian vault, read by the assistant: TCC_VAULT, or
// Documents\Obsidian Vault when it exists.
func vaultDir() string {
	if v := os.Getenv("TCC_VAULT"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	d := filepath.Join(home, "Documents", "Obsidian Vault")
	if fi, err := os.Stat(d); err == nil && fi.IsDir() {
		return d
	}
	return ""
}
