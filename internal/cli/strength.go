package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"terrancc/internal/strength"
)

// readToken gets the token without it appearing on screen or in shell
// history: typed at a hidden prompt, or piped in on stdin.
func (a *app) readToken(cmd *cobra.Command) (string, error) {
	in := cmd.InOrStdin()
	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(a.errOut, "99-Strength token (hidden): ")
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(a.errOut)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	b, err := io.ReadAll(io.LimitReader(in, 4096))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (a *app) printStrength(st strength.Status) {
	if !st.Connected {
		a.printf("99-Strength: not connected. Habits are the local ones.\n")
		a.printf("  connect   tcc strength connect --api https://api-99strength.enjoeneer.dev/api/v1\n")
		return
	}
	a.printf("99-Strength: connected to %s\n", st.BaseURL)
	a.printf("  account   %s (token %q)\n", st.Email, st.TokenName)
	switch st.State {
	case "auth":
		a.printf("  state     the token needs renewing: run `tcc strength connect` and give a new one\n")
	case "stale":
		a.printf("  state     offline, showing saved values\n")
	default:
		a.printf("  state     up to date\n")
	}
	a.printf("  synced    %s\n", syncTime(st.LastSync))
	if st.Pending > 0 {
		a.printf("  queued    %s\n", waiting(st.Pending))
	}
	if st.Error != "" {
		a.printf("  problem   %s\n", st.Error)
	}
	a.printf("  token     kept in %s\n", st.TokenStore)
}

func (a *app) strengthCmd() *cobra.Command {
	status := func(cmd *cobra.Command, args []string) error {
		c, err := a.client()
		if err != nil {
			return err
		}
		st, err := c.Strength()
		if err != nil {
			return err
		}
		return a.emit(st, func() { a.printStrength(st) })
	}
	cmd := &cobra.Command{
		Use:   "strength",
		Short: "Take habits from 99-Strength instead of the local ones",
		Long: "99-Strength (the training app) can own the habits. Once connected, `tcc habit`,\n" +
			"`tcc status` and the habit widget show and log 99-Strength's habits. Changes made\n" +
			"while it is unreachable are kept and sent when it is back.",
		Args: cobra.NoArgs,
		RunE: status,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show the 99-Strength connection",
		Args:  cobra.NoArgs,
		RunE:  status,
	})

	var api, token string
	connect := &cobra.Command{
		Use:   "connect --api <base url> [--token <token>]",
		Short: "Connect with a personal API token (or renew the token)",
		Long: "Checks the token with 99-Strength and, only if it works, stores it in Windows\n" +
			"Credential Manager. Without --token you are asked for it (nothing is shown as\n" +
			"you type), or it is read from stdin when piped:\n\n" +
			"  tcc strength connect --api https://api-99strength.enjoeneer.dev/api/v1\n" +
			"  Get-Clipboard | tcc strength connect --api https://api-99strength.enjoeneer.dev/api/v1\n\n" +
			"--token works too, but leaves the token in your shell history.\n" +
			"To renew a token, run it again; --api can be left out then.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.client()
			if err != nil {
				return err
			}
			if api == "" { // renewing: keep the address already connected
				st, err := c.Strength()
				if err != nil {
					return err
				}
				if api = st.BaseURL; api == "" {
					return errors.New("--api is required, e.g. --api https://api-99strength.enjoeneer.dev/api/v1")
				}
			}
			if token == "" {
				if token, err = a.readToken(cmd); err != nil {
					return err
				}
				if token == "" {
					return errors.New("no token given")
				}
			}
			st, err := c.StrengthConnect(api, token)
			if err != nil {
				return err
			}
			return a.emit(st, func() {
				a.printf("Connected to %s as %s (token %q).\n", st.BaseURL, st.Email, st.TokenName)
				if day, err := c.Habits(""); err == nil {
					a.printf("%d habits now come from 99-Strength. The token is kept in %s.\n", len(day.Habits), st.TokenStore)
				}
				if note := syncNote(&st.Sync); note != "" {
					a.printf("%s\n", note)
				}
			})
		},
	}
	connect.Flags().StringVar(&api, "api", "", "base URL, e.g. https://api-99strength.enjoeneer.dev/api/v1")
	connect.Flags().StringVar(&token, "token", "", "personal API token (nns_pat_...); better left out, see above")

	var force bool
	disconnect := &cobra.Command{
		Use:   "disconnect",
		Short: "Forget the token and go back to local habits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.client()
			if err != nil {
				return err
			}
			if !force {
				st, err := c.Strength()
				if err != nil {
					return err
				}
				if st.Pending > 0 {
					return fmt.Errorf("%s. Run `tcc strength sync` once 99-Strength is reachable, or `tcc strength disconnect --force` to drop them", waiting(st.Pending))
				}
			}
			dropped, err := c.StrengthDisconnect()
			if err != nil {
				return err
			}
			return a.emit(map[string]any{"connected": false, "dropped": dropped}, func() {
				a.printf("Disconnected. The token is deleted and habits are the local ones again.\n")
				if dropped > 0 {
					a.printf("%d changes that had not reached 99-Strength were dropped.\n", dropped)
				}
			})
		},
	}

	disconnect.Flags().BoolVar(&force, "force", false, "disconnect even if changes are still waiting to sync (they are dropped)")

	sync := &cobra.Command{
		Use:   "sync",
		Short: "Send queued changes and read today's habits again, now",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.client()
			if err != nil {
				return err
			}
			st, err := c.StrengthSync()
			if err != nil {
				return err
			}
			return a.emit(st, func() { a.printStrength(st) })
		},
	}

	cmd.AddCommand(connect, disconnect, sync)
	return cmd
}
