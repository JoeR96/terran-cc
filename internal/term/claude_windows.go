package term

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/UserExistsError/conpty"
)

// ClaudeStarter runs Claude Code in dir under a Windows pseudo-console,
// continuing the last conversation there when there is one.
func ClaudeStarter(dir string) Starter {
	return func(cols, rows int, again bool) (Proc, error) {
		exe, err := exec.LookPath("claude")
		if err != nil {
			home, _ := os.UserHomeDir()
			exe = filepath.Join(home, ".local", "bin", "claude.exe")
			if _, serr := os.Stat(exe); serr != nil {
				return nil, fmt.Errorf("can't find the claude CLI: %v", err)
			}
		}
		cmd := fmt.Sprintf("%q", exe)
		if again || hasConversation(dir) {
			cmd += " --continue"
		}
		return conpty.Start(cmd, conpty.ConPtyDimensions(cols, rows), conpty.ConPtyWorkDir(dir), conpty.ConPtyEnv(cleanEnv(os.Environ())))
	}
}
