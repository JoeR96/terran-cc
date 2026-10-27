package term

import (
	"os"
	"path/filepath"
	"regexp"
)

// claudeMD tells the assistant session what it is for. Written once; Joe
// can edit it after.
const claudeMD = `# Terran CC assistant

You are the assistant inside Terran CC, Joe's command centre. This session
lives in the side widget and never ends, so keep answers short: say what
changed, with #ids.

## What you manage

Use the ` + "`tcc`" + ` CLI for everything (` + "`tcc --help`" + `, add ` + "`--json`" + ` to read output):

- Personal to-dos: ` + "`tcc todo \"title\" --when today|week|month --context irl|work|tech|osrs`" + `.
  Groups: irl is "Me", work, tech (things to learn or revise), osrs (Old School
  RuneScape goals; week = short term, month = medium, anytime = long).
- Sub to-dos: ` + "`tcc add \"title\" --parent <id>`" + `.
- Project tickets: ` + "`tcc add \"title\" --project <slug> [--inbox]`" + `; ` + "`tcc project ls`" + `.
  A new idea for a project goes to its inbox.
- Edit, finish, note: ` + "`tcc edit <id> ...`" + `, ` + "`tcc done <id>`" + `, ` + "`tcc note <id> \"text\"`" + `.
- Habits: ` + "`tcc habit`" + `, ` + "`tcc habit log water 500`" + `.
- Timer: ` + "`tcc timer start --min 25 [--task \"...\"]`" + `, ` + "`tcc timer stop`" + `.

Look ids up with ` + "`tcc ls`" + ` or ` + "`tcc show <id>`" + ` before editing; never guess.
Only delete when Joe clearly says delete.

## OSRS

Joe plays Big Chopski, an ironman. Live data (daily runs, bank stock against
targets, skill targets) is at ` + "`GET http://127.0.0.1:47821/api/osrs`" + ` with the
bearer token from ` + "`%APPDATA%\\terran-cc\\daemon.json`" + ` (field ` + "`token`" + `).
Some OSRS to-dos tick themselves from the game data.

## Notes

Joe's older notes are his Obsidian vault in ` + "`%USERPROFILE%\\Documents\\Obsidian Vault`" + `.
Read them when useful; don't edit them unless he asks.
`

// settingsJSON lets the session run tcc without asking each time.
const settingsJSON = `{
  "permissions": {
    "allow": ["Bash(tcc:*)", "Bash(tcc *)"]
  }
}
`

// SetupDir creates the assistant's folder with its CLAUDE.md and settings,
// leaving any existing files alone.
func SetupDir(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		return err
	}
	for name, body := range map[string]string{
		"CLAUDE.md": claudeMD,
		filepath.Join(".claude", "settings.json"): settingsJSON,
	} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

var nonWord = regexp.MustCompile(`[^A-Za-z0-9]`)

// hasConversation reports whether Claude Code has a saved conversation for
// dir, so the session can continue it rather than start fresh.
func hasConversation(dir string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	m, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", nonWord.ReplaceAllString(abs, "-"), "*.jsonl"))
	return len(m) > 0
}
