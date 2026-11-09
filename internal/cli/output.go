package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"terrancc/internal/client"
	"terrancc/internal/model"
	"terrancc/internal/strength"
)

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// habitLine renders "💧 Water 250/3000 ml" or "💊 Creatine + supplements ✓".
func habitLine(d model.HabitDay) string {
	if d.Kind != "" {
		return remoteHabitLine(d)
	}
	label := strings.TrimSpace(d.Icon + " " + d.Name)
	if d.Type == model.HabitCheck {
		mark := "·"
		if d.Hit {
			mark = "✓"
		}
		return label + " " + mark
	}
	s := fmt.Sprintf("%s %s/%s", label, num(d.Value), num(d.Target))
	if d.Unit != "" {
		s += " " + d.Unit
	}
	if d.Hit {
		s += " ✓"
	}
	return s
}

// remoteHabitLine renders a 99-Strength habit: "💧 Water (532ml bottle) 4/6
// bottles (min 4) ✓ min", "⚖️ Morning weigh-in 94.6 kg ✓" or "💊 Creatine ·".
// "⟳" marks a change that has not reached 99-Strength yet.
func remoteHabitLine(d model.HabitDay) string {
	s := strings.TrimSpace(d.Icon + " " + d.Name)
	unit := ""
	if d.Unit != "" {
		unit = " " + d.Unit
	}
	switch d.Type {
	case model.HabitCheck:
		if d.Hit {
			s += " ✓"
		} else {
			s += " ·"
		}
	case model.HabitNumeric:
		if d.Value > 0 {
			s += " " + num(d.Value) + unit + " ✓"
		} else {
			s += " not logged"
		}
	default:
		s += fmt.Sprintf(" %s/%s%s", num(d.Value), num(d.Target), unit)
		if d.Minimum > 0 && d.Minimum < d.Target {
			s += " (min " + num(d.Minimum) + ")"
		}
		switch {
		case d.Hit:
			s += " ✓"
		case d.MinHit:
			s += " ✓ min"
		}
	}
	if d.Pending > 0 {
		s += " ⟳"
	}
	return s
}

// syncTime shows a sync time as a clock time, with the day when it was not today.
func syncTime(t *time.Time) string {
	if t == nil {
		return "never"
	}
	l := t.Local()
	if now := time.Now(); l.YearDay() == now.YearDay() && l.Year() == now.Year() {
		return l.Format("15:04")
	}
	return l.Format("Mon 2 Jan 15:04")
}

func waiting(n int) string {
	if n == 1 {
		return "1 change waiting to sync"
	}
	return fmt.Sprintf("%d changes waiting to sync", n)
}

// syncNote is the line under the habits when 99-Strength needs attention,
// or "" when all is well.
func syncNote(sy *strength.Sync) string {
	if sy == nil {
		return ""
	}
	var s string
	switch {
	case sy.NeedsToken && sy.LastSync == nil:
		s = "⚠ 99-Strength token needs renewing: run `tcc strength connect`"
	case sy.NeedsToken:
		s = "⚠ 99-Strength token needs renewing: run `tcc strength connect`. Showing values from " + syncTime(sy.LastSync)
	case sy.Stale && sy.LastSync == nil:
		s = "⚠ 99-Strength is offline and no habits have been read yet"
	case sy.Stale:
		s = "⚠ 99-Strength is offline. Showing values from " + syncTime(sy.LastSync)
	case sy.Pending > 0:
		return "⟳ " + waiting(sy.Pending)
	case sy.Error != "":
		return "⚠ " + sy.Error
	default:
		return ""
	}
	if sy.Pending > 0 {
		s += ", " + waiting(sy.Pending)
	}
	return s
}

// habitsHeader is the first line of the habit list. The local one is as it
// always was; 99-Strength adds its name.
func habitsHeader(date string, day client.HabitsDay) string {
	s := fmt.Sprintf("%s · 🔥 %d-day streak", date, day.Streak)
	if day.Source == model.SourceStrength {
		s += " · 99-Strength"
	}
	return s
}

// itemLine renders "#12   bug     backlog      Fix bot melee attacks  [bdtv-bonanza]".
func itemLine(it model.Item, slugs map[int64]string) string {
	s := fmt.Sprintf("#%-4d %-7s %-11s %s", it.ID, it.Kind, it.Status, it.Title)
	if it.ProjectID != nil {
		s += "  [" + slugs[*it.ProjectID] + "]"
	} else if it.When != model.WhenAnytime {
		s += "  (" + string(it.When) + ")"
	}
	return s
}

func clock(seconds float64) string {
	n := int(seconds)
	return fmt.Sprintf("%02d:%02d", n/60, n%60)
}

// timerLine renders a timer snapshot, e.g. "focus 24:13 running · 2 done".
func timerLine(s map[string]any) string {
	cycle, _ := s["cycle"].(float64)
	rem, _ := s["remaining_s"].(float64)
	return fmt.Sprintf("%s %s %s · %d done", s["phase"], clock(rem), s["state"], int(cycle))
}

func configLine(s map[string]any) string {
	c, _ := s["config"].(map[string]any)
	return fmt.Sprintf("focus %vm · short %vm · long %vm every %v · auto-start %v",
		c["focus_min"], c["short_min"], c["long_min"], c["long_every"], c["auto_start"])
}
