// Command tcc is the Terran CC command line. It talks to tccd, starting it
// if it isn't running.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"terrancc/internal/cli"
	"terrancc/internal/client"
	"terrancc/internal/config"
)

// tccdPath prefers the tccd next to this executable, then one on PATH.
func tccdPath() string {
	name := "tccd"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return name
}

func main() {
	start := func() (*client.Client, error) {
		home, err := config.Home()
		if err != nil {
			return nil, err
		}
		return client.EnsureDaemon(home, tccdPath(), 5*time.Second)
	}
	stop := func() error {
		home, err := config.Home()
		if err != nil {
			return err
		}
		d, err := config.ReadDaemon(home)
		if err != nil {
			return nil // nothing running
		}
		c := client.New(d.Addr, d.Token)
		if _, err := c.Health(); err != nil {
			config.RemoveDaemon(home) // stale file
			return nil
		}
		return client.StopDaemon(home, c, 5*time.Second)
	}
	root := cli.NewRoot(start, os.Stdout, os.Stderr)
	root.AddCommand(cli.ServiceCmd(stop, start, os.Stdout))
	os.Exit(cli.Execute(root, os.Args[1:]))
}
