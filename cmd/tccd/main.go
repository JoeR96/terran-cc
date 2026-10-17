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
	"syscall"
	"time"

	"terrancc/internal/api"
	"terrancc/internal/config"
	"terrancc/internal/core"
	"terrancc/internal/events"
	"terrancc/internal/store/sqlite"
	"terrancc/internal/timer"
)

const defaultAddr = "127.0.0.1:47821"

func main() {
	if err := run(); err != nil {
		log.Printf("tccd: %v", err)
		fmt.Fprintln(os.Stderr, "tccd:", err)
		os.Exit(1)
	}
}

func run() error {
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

	token, err := config.Token(home)
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		if alreadyRunning(*addr) {
			log.Printf("another tccd is already serving %s", *addr)
			return nil
		}
		return err
	}

	db, err := sqlite.Open(filepath.Join(home, "tcc.db"))
	if err != nil {
		return err
	}
	defer db.Close()

	svc := &core.Service{Store: db, Clock: core.SystemClock{}}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
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
	srv := &http.Server{Handler: api.New(svc, tm, bus, token, home), ReadHeaderTimeout: 10 * time.Second}

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

func alreadyRunning(addr string) bool {
	c := http.Client{Timeout: time.Second}
	resp, err := c.Get("http://" + addr + "/api/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
