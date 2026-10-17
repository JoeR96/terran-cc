package config

import (
	"os"
	"testing"
)

func TestHomeUsesEnv(t *testing.T) {
	dir := t.TempDir() + "/home"
	t.Setenv("TCC_HOME", dir)
	got, err := Home()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("got %q want %q", got, dir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("home not created: %v", err)
	}
}

func TestTokenStable(t *testing.T) {
	dir := t.TempDir()
	a, err := Token(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Token(dir)
	if a != b || len(a) != 64 {
		t.Fatalf("tokens %q %q", a, b)
	}
}

func TestDaemonRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Daemon{Addr: "127.0.0.1:1", Token: "abc", PID: 42}
	if err := WriteDaemon(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDaemon(dir)
	if err != nil || got != want {
		t.Fatalf("got %+v err %v", got, err)
	}
}
