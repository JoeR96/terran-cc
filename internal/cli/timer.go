package cli

import (
	"github.com/spf13/cobra"
)

// timerText adds the focus item's title to a snapshot line when there is one.
func (a *app) timerText(s map[string]any) string {
	line := timerLine(s)
	if id, ok := s["item_id"].(float64); ok {
		if c, err := a.client(); err == nil {
			if it, err := c.GetItem(int64(id)); err == nil {
				line += " · #" + itoa(it.ID) + " " + it.Title
			}
		}
	}
	return line
}

func (a *app) timerCmd() *cobra.Command {
	show := func(s map[string]any, err error) error {
		if err != nil {
			return err
		}
		return a.emit(s, func() { a.printf("%s\n", a.timerText(s)) })
	}
	cmd := &cobra.Command{
		Use:   "timer",
		Short: "Pomodoro timer (no subcommand shows it)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.client()
			if err != nil {
				return err
			}
			return show(c.Timer())
		},
	}
	var item string
	var minutes float64
	var task string
	start := &cobra.Command{
		Use:   "start",
		Short: "Start the current phase, optionally focused on an item, or a one-off countdown with --min",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if minutes > 0 {
				body["minutes"] = minutes
				body["label"] = task
			}
			if item != "" {
				id, err := parseID(item)
				if err != nil {
					return err
				}
				body["item_id"] = id
			}
			c, err := a.client()
			if err != nil {
				return err
			}
			return show(c.TimerAction("start", body))
		},
	}
	start.Flags().StringVar(&item, "item", "", "item id to focus on")
	start.Flags().Float64Var(&minutes, "min", 0, "run a one-off countdown of this many minutes (no break after)")
	start.Flags().StringVar(&task, "task", "", "with --min: lock in on this task (the widget shows only the time and the task)")
	cmd.AddCommand(start)

	for name, short := range map[string]string{
		"status": "Show the timer", "pause": "Pause", "resume": "Resume",
		"toggle": "Start, pause or resume (for a Stream Deck button)", "skip": "End this phase now", "stop": "Reset to a fresh focus session",
	} {
		cmd.AddCommand(&cobra.Command{
			Use:   name,
			Short: short,
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := a.client()
				if err != nil {
					return err
				}
				if name == "status" {
					return show(c.Timer())
				}
				return show(c.TimerAction(name, nil))
			},
		})
	}

	config := &cobra.Command{
		Use:   "config",
		Short: "Show or change the timer lengths (minutes)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.client()
			if err != nil {
				return err
			}
			cur, err := c.Timer()
			if err != nil {
				return err
			}
			cfg, _ := cur["config"].(map[string]any)
			changed := false
			for flag, key := range map[string]string{"focus": "focus_min", "short": "short_min", "long": "long_min"} {
				if cmd.Flags().Changed(flag) {
					v, _ := cmd.Flags().GetFloat64(flag)
					cfg[key], changed = v, true
				}
			}
			if cmd.Flags().Changed("every") {
				v, _ := cmd.Flags().GetInt("every")
				cfg["long_every"], changed = v, true
			}
			if cmd.Flags().Changed("auto-start") {
				v, _ := cmd.Flags().GetBool("auto-start")
				cfg["auto_start"], changed = v, true
			}
			if changed {
				if cur, err = c.TimerConfig(cfg); err != nil {
					return err
				}
			}
			return a.emit(cur["config"], func() { a.printf("%s\n", configLine(cur)) })
		},
	}
	config.Flags().Float64("focus", 25, "focus minutes")
	config.Flags().Float64("short", 5, "short break minutes")
	config.Flags().Float64("long", 15, "long break minutes")
	config.Flags().Int("every", 4, "long break after this many focus sessions")
	config.Flags().Bool("auto-start", false, "start the next phase automatically")
	cmd.AddCommand(config)
	return cmd
}
