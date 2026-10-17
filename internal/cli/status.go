package cli

import (
	"net/url"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"terrancc/internal/client"
	"terrancc/internal/model"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func (a *app) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Today at a glance: habits, timer, today's to-dos, work in progress",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.client()
			if err != nil {
				return err
			}
			day, err := c.Habits("")
			if err != nil {
				return err
			}
			tm, err := c.Timer()
			if err != nil {
				return err
			}
			open := "inbox,backlog,up_next,in_progress,review"
			today, err := c.ListItems(url.Values{"when": {"today"}, "status": {open}})
			if err != nil {
				return err
			}
			doing, err := c.ListItems(url.Values{"status": {"in_progress,review"}})
			if err != nil {
				return err
			}
			out := struct {
				Habits     client.HabitsDay `json:"habits"`
				Timer      map[string]any   `json:"timer"`
				Today      []model.Item     `json:"today"`
				InProgress []model.Item     `json:"in_progress"`
			}{day, tm, today, doing}
			return a.emit(out, func() {
				date, _ := time.Parse("2006-01-02", day.Date)
				a.printf("%s · 🔥 %d-day streak\n", date.Format("Mon 2 Jan"), day.Streak)
				for _, d := range day.Habits {
					a.printf("  %s\n", habitLine(d))
				}
				a.printf("\nTimer: %s\n", a.timerText(tm))
				slugs, _ := a.slugs()
				a.printf("\nToday:\n")
				if len(today) == 0 {
					a.printf("  nothing due\n")
				}
				for _, it := range today {
					a.printf("  %s\n", itemLine(it, slugs))
				}
				if len(doing) > 0 {
					a.printf("\nIn progress / review:\n")
					for _, it := range doing {
						a.printf("  %s\n", itemLine(it, slugs))
						if it.Branch != "" || it.Session != "" {
							a.printf("        🌿 %s  ◆ %s\n", it.Branch, it.Session)
						}
					}
				}
			})
		},
	}
}
