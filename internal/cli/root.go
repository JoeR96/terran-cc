// Package cli implements the tcc command. Every command is a thin call to
// tccd through internal/client; output is human text, or the API JSON with --json.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"terrancc/internal/client"
)

type app struct {
	getClient func() (*client.Client, error)
	c         *client.Client
	out       io.Writer
	errOut    io.Writer
	json      bool
}

// client connects lazily, so --help never starts the service.
func (a *app) client() (*client.Client, error) {
	if a.c == nil {
		c, err := a.getClient()
		if err != nil {
			return nil, err
		}
		a.c = c
	}
	return a.c, nil
}

// emit prints v as JSON with --json, otherwise calls human.
func (a *app) emit(v any, human func()) error {
	if a.json {
		enc := json.NewEncoder(a.out)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	human()
	return nil
}

func (a *app) printf(format string, args ...any) { fmt.Fprintf(a.out, format, args...) }

// NewRoot builds the tcc command tree. getClient is called on first use.
func NewRoot(getClient func() (*client.Client, error), out, errOut io.Writer) *cobra.Command {
	a := &app{getClient: getClient, out: out, errOut: errOut}
	root := &cobra.Command{
		Use:           "tcc",
		Short:         "Terran CC: tickets, to-dos, habits and a Pomodoro timer",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().BoolVar(&a.json, "json", false, "print JSON (for scripts and Claude)")
	root.SetOut(out)
	root.SetErr(errOut)
	root.AddCommand(a.statusCmd())
	a.addItemCmds(root)
	root.AddCommand(a.projectCmd(), a.habitCmd(), a.timerCmd())
	root.CompletionOptions.DisableDefaultCmd = true
	return root
}

// Execute runs root with args and returns the process exit code:
// 0 ok, 1 error, 2 not found.
func Execute(root *cobra.Command, args []string) int {
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return 0
	}
	asJSON := false
	for _, arg := range args {
		if arg == "--json" {
			asJSON = true
		}
	}
	if asJSON {
		b, _ := json.Marshal(map[string]string{"error": err.Error()})
		fmt.Fprintln(root.OutOrStdout(), string(b))
	} else {
		fmt.Fprintln(root.ErrOrStderr(), "tcc:", err)
	}
	var ae *client.APIError
	if errors.As(err, &ae) && ae.Status == 404 {
		return 2
	}
	return 1
}

func parseID(s string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimPrefix(strings.TrimSpace(s), "#"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%q is not an item id (use 12 or #12)", s)
	}
	return id, nil
}
