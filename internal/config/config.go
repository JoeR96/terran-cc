// Package config locates Terran CC's home directory and the files in it.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Home returns $TCC_HOME, or <user config dir>/terran-cc, creating it if needed.
func Home() (string, error) {
	dir := os.Getenv("TCC_HOME")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "terran-cc")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// Daemon is what tccd writes to daemon.json so clients can find it.
type Daemon struct {
	Addr  string `json:"addr"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

func daemonPath(home string) string { return filepath.Join(home, "daemon.json") }

func WriteDaemon(home string, d Daemon) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return os.WriteFile(daemonPath(home), b, 0o600)
}

func ReadDaemon(home string) (Daemon, error) {
	var d Daemon
	b, err := os.ReadFile(daemonPath(home))
	if err != nil {
		return d, err
	}
	return d, json.Unmarshal(b, &d)
}

func RemoveDaemon(home string) error {
	err := os.Remove(daemonPath(home))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Token returns the API token, creating a random one on first use.
func Token(home string) (string, error) {
	p := filepath.Join(home, "token")
	if b, err := os.ReadFile(p); err == nil {
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	t := hex.EncodeToString(buf)
	return t, os.WriteFile(p, []byte(t), 0o600)
}
