package cli

import (
	"fmt"
	"strconv"
	"strings"

	"terrancc/internal/model"
)

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// habitLine renders "💧 Water 250/3000 ml" or "💊 Creatine + supplements ✓".
func habitLine(d model.HabitDay) string {
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
