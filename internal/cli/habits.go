package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"terrancc/internal/core"
	"terrancc/internal/model"
)

func (a *app) printHabits() error {
	c, err := a.client()
	if err != nil {
		return err
	}
	day, err := c.Habits("")
	if err != nil {
		return err
	}
	return a.emit(day, func() {
		a.printf("%s · 🔥 %d-day streak\n", day.Date, day.Streak)
		for _, d := range day.Habits {
			a.printf("  %s\n", habitLine(d))
		}
	})
}

// parseLogOp reads "+250", "-100", "=1800", "toggle" or a bare number (added).
func parseLogOp(s string) (core.LogOp, error) {
	if s == "toggle" {
		return core.LogOp{Toggle: true}, nil
	}
	set := strings.HasPrefix(s, "=")
	v, err := strconv.ParseFloat(strings.TrimPrefix(strings.TrimPrefix(s, "="), "+"), 64)
	if err != nil {
		return core.LogOp{}, fmt.Errorf("%q is not an amount: use +250, -100, =1800 or toggle", s)
	}
	if set {
		return core.LogOp{Set: &v}, nil
	}
	return core.LogOp{Delta: &v}, nil
}

func (a *app) habitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "habit",
		Short: "Show and log today's habits",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return a.printHabits() },
	}
	ls := &cobra.Command{
		Use:   "ls",
		Short: "Show today's habits and the streak",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return a.printHabits() },
	}
	log := &cobra.Command{
		Use:   "log <habit> [+N|-N|=N|toggle]",
		Short: "Log a habit for today (no amount = one step)",
		Long:  "Log a habit for today. The habit can be its id, name or a unique start of its name.\nWith no amount it adds one step (or ticks a check habit).",
		// Flags are parsed by hand so "-100" isn't mistaken for a flag.
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var rest []string
			for _, arg := range args {
				switch arg {
				case "--json":
					a.json = true
				case "-h", "--help":
					return cmd.Help()
				default:
					rest = append(rest, arg)
				}
			}
			if len(rest) < 1 || len(rest) > 2 {
				return fmt.Errorf("usage: tcc habit log <habit> [+N|-N|=N|toggle]")
			}
			var op core.LogOp
			if len(rest) == 2 {
				var err error
				if op, err = parseLogOp(rest[1]); err != nil {
					return err
				}
			}
			c, err := a.client()
			if err != nil {
				return err
			}
			d, err := c.LogHabit(rest[0], op)
			if err != nil {
				return err
			}
			return a.emit(d, func() { a.printf("%s\n", habitLine(d)) })
		},
	}

	var h model.Habit
	var typ string
	add := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a daily habit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			h.Name, h.Type = args[0], model.HabitType(typ)
			c, err := a.client()
			if err != nil {
				return err
			}
			out, err := c.AddHabit(h)
			if err != nil {
				return err
			}
			return a.emit(out, func() { a.printf("added %s %s\n", out.Icon, out.Name) })
		},
	}
	add.Flags().StringVar(&typ, "type", "check", "count|amount|check")
	add.Flags().Float64Var(&h.Target, "target", 1, "daily target")
	add.Flags().Float64Var(&h.Step, "step", 1, "amount one log adds")
	add.Flags().StringVar(&h.Unit, "unit", "", "e.g. ml, g")
	add.Flags().StringVar(&h.Icon, "icon", "", "an emoji")

	edit := &cobra.Command{
		Use:   "edit <habit>",
		Short: "Change a habit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			patch := map[string]any{}
			for _, f := range []string{"name", "unit", "icon"} {
				if cmd.Flags().Changed(f) {
					v, _ := cmd.Flags().GetString(f)
					patch[f] = v
				}
			}
			for _, f := range []string{"target", "step"} {
				if cmd.Flags().Changed(f) {
					v, _ := cmd.Flags().GetFloat64(f)
					patch[f] = v
				}
			}
			if cmd.Flags().Changed("order") {
				v, _ := cmd.Flags().GetInt("order")
				patch["order"] = v
			}
			if cmd.Flags().Changed("archive") {
				v, _ := cmd.Flags().GetBool("archive")
				patch["archived"] = v
			}
			c, err := a.client()
			if err != nil {
				return err
			}
			out, err := c.EditHabit(args[0], patch)
			if err != nil {
				return err
			}
			return a.emit(out, func() { a.printf("updated %s %s\n", out.Icon, out.Name) })
		},
	}
	for _, f := range []string{"name", "unit", "icon"} {
		edit.Flags().String(f, "", "")
	}
	edit.Flags().Float64("target", 0, "daily target")
	edit.Flags().Float64("step", 0, "amount one log adds")
	edit.Flags().Int("order", 0, "display order")
	edit.Flags().Bool("archive", false, "stop tracking (or --archive=false to restore)")

	cmd.AddCommand(ls, log, add, edit)
	return cmd
}
