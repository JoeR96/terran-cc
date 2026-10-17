package client

import (
	"context"
	"errors"
	"net"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"terrancc/internal/api"
	"terrancc/internal/config"
	"terrancc/internal/core"
	"terrancc/internal/events"
	"terrancc/internal/store/sqlite"
	"terrancc/internal/timer"
)

func testServer(t *testing.T) *Client {
	t.Helper()
	home := t.TempDir()
	db, err := sqlite.Open(filepath.Join(home, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := &core.Service{Store: db, Clock: core.SystemClock{}}
	srv := httptest.NewServer(api.New(svc, timer.New(timer.DefaultConfig(), time.Now), events.NewBus(), "tok", home))
	t.Cleanup(srv.Close)
	return New(srv.Listener.Addr().String(), "tok")
}

func TestClientRoundTrip(t *testing.T) {
	c := testServer(t)
	p, err := c.AddProject(map[string]any{"name": "BDTV Bonanza"})
	if err != nil || p.Slug != "bdtv-bonanza" {
		t.Fatalf("%+v %v", p, err)
	}
	it, err := c.AddItem(core.NewItem{Title: "Fix melee", ProjectID: &p.ID})
	if err != nil || it.ID == 0 {
		t.Fatalf("%+v %v", it, err)
	}
	items, err := c.ListItems(url.Values{"project": {"bdtv-bonanza"}})
	if err != nil || len(items) != 1 || items[0].Title != "Fix melee" {
		t.Fatalf("%+v %v", items, err)
	}
	it, err = c.Action(it.ID, "start", core.StartInfo{Branch: "b"})
	if err != nil || it.Branch != "b" {
		t.Fatalf("%+v %v", it, err)
	}
	snap, err := c.TimerAction("toggle", nil)
	if err != nil || snap["state"] != "running" {
		t.Fatalf("%v %v", snap, err)
	}
}

func TestAPIError(t *testing.T) {
	c := testServer(t)
	_, err := c.GetItem(404)
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 404 || ae.Message == "" {
		t.Fatalf("%#v", err)
	}
}

func buildTccd(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "tccd")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	out, err := exec.Command("go", "build", "-o", exe, "terrancc/cmd/tccd").CombinedOutput()
	if err != nil {
		t.Fatalf("build tccd: %v\n%s", err, out)
	}
	return exe
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

func stopDaemon(t *testing.T, home string) {
	d, err := config.ReadDaemon(home)
	if err != nil {
		return
	}
	if p, err := os.FindProcess(d.PID); err == nil {
		p.Kill()
		p.Wait()
	}
	time.Sleep(200 * time.Millisecond) // let Windows release the db file before TempDir cleanup
}

func TestEnsureStartsDaemon(t *testing.T) {
	exe := buildTccd(t)
	home := t.TempDir()
	t.Setenv("TCC_HOME", home)
	t.Setenv("TCC_ADDR", freeAddr(t))
	t.Cleanup(func() { stopDaemon(t, home) })

	start := time.Now()
	c, err := EnsureDaemon(home, exe, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("too slow")
	}
	day, err := c.Habits("")
	if err != nil || len(day.Habits) != 4 {
		t.Fatalf("seeded habits: %+v %v", day, err)
	}
	// A second call reuses the running daemon.
	if _, err := EnsureDaemon(home, exe, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	_ = context.Background()
}

func TestStaleDaemonFile(t *testing.T) {
	exe := buildTccd(t)
	home := t.TempDir()
	t.Setenv("TCC_HOME", home)
	t.Setenv("TCC_ADDR", freeAddr(t))
	t.Cleanup(func() { stopDaemon(t, home) })
	config.WriteDaemon(home, config.Daemon{Addr: freeAddr(t), Token: "old", PID: 1})
	c, err := EnsureDaemon(home, exe, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Health(); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureTimesOut(t *testing.T) {
	home := t.TempDir()
	_, err := EnsureDaemon(home, filepath.Join(home, "missing-tccd"), 300*time.Millisecond)
	if err == nil {
		t.Fatal("expected an error")
	}
}
