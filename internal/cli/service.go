package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"terrancc/internal/client"
)

// ServiceCmd builds "tcc service". stop shuts the running tccd down (a no-op
// when none runs); start ensures one is running and returns a client for it.
func ServiceCmd(stop func() error, start func() (*client.Client, error), out io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: "service", Short: "Check, stop or restart the background service (tccd)"}
	status := func() error {
		c, err := start()
		if err != nil {
			return err
		}
		h, err := c.Health()
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "tccd version %v · pid %v · home %v\n", h["version"], h["pid"], h["home"])
		return nil
	}
	cmd.AddCommand(&cobra.Command{
		Use: "status", Short: "Start the service if needed and show it", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return status() },
	}, &cobra.Command{
		Use: "stop", Short: "Stop the service (the next tcc command starts it again)", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if err := stop(); err != nil {
				return err
			}
			fmt.Fprintln(out, "tccd stopped")
			return nil
		},
	}, &cobra.Command{
		Use: "restart", Short: "Stop the service and start it again", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if err := stop(); err != nil {
				return err
			}
			return status()
		},
	})
	return cmd
}
