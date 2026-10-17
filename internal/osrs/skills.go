package osrs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"
)

// xpFor is the experience needed for a level (1-126), the standard OSRS
// curve.
func xpFor(level int) int {
	total := 0.0
	for l := 1; l < level; l++ {
		total += math.Floor(float64(l) + 300*math.Pow(2, float64(l)/7))
	}
	return int(math.Floor(total / 4))
}

// levelFor is the level an amount of experience gives, capped at 99.
func levelFor(xp int) int {
	l := 1
	for l < 99 && xpFor(l+1) <= xp {
		l++
	}
	return l
}

// SkillTarget is a level Joe is working towards. StartXP is recorded the
// first time the target is seen, so the bar can show progress since then.
type SkillTarget struct {
	Skill   string `json:"skill"`
	Level   int    `json:"level"`
	StartXP int    `json:"start_xp"`
}

// SkillProgress is a target with the hiscores' current XP.
type SkillProgress struct {
	SkillTarget
	XP       int    `json:"xp"`
	Current  int    `json:"current_level"`
	GoalXP   int    `json:"goal_xp"`
	Icon     string `json:"icon"`
	Done     bool   `json:"done"`
	ToGo     int    `json:"to_go"`
	Progress int    `json:"progress"` // XP gained since the start
}

// Skills is a hiscores lookup: skill name to XP.
type Skills map[string]int

const hiscoresURL = "https://secure.runescape.com/m=hiscore_oldschool/index_lite.json?player="

// FetchSkills reads a player's XP from the official hiscores.
func FetchSkills(ctx context.Context, hc *http.Client, base, player, version string) (Skills, error) {
	if base == "" {
		base = hiscoresURL
	}
	req, err := http.NewRequestWithContext(ctx, "GET", base+url.QueryEscape(player), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "terran-cc/"+version)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("hiscores %d for %q", resp.StatusCode, player)
	}
	var body struct {
		Skills []struct {
			Name string `json:"name"`
			XP   int    `json:"xp"`
		} `json:"skills"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("hiscores: %w", err)
	}
	out := Skills{}
	for _, s := range body.Skills {
		if s.Name != "Overall" {
			out[s.Name] = max(s.XP, 0)
		}
	}
	return out, nil
}

// DefaultTargets is Base 90s (every skill below 90 aims at 90) plus 99
// Construction.
func DefaultTargets(sk Skills) []SkillTarget {
	var out []SkillTarget
	for _, name := range skillOrder {
		if xp, ok := sk[name]; ok && xp < xpFor(90) {
			out = append(out, SkillTarget{Skill: name, Level: 90})
		}
	}
	return append(out, SkillTarget{Skill: "Construction", Level: 99})
}

var skillOrder = []string{"Attack", "Defence", "Strength", "Hitpoints", "Ranged", "Prayer", "Magic", "Cooking",
	"Woodcutting", "Fletching", "Fishing", "Firemaking", "Crafting", "Smithing", "Mining", "Herblore", "Agility",
	"Thieving", "Slayer", "Farming", "Runecraft", "Hunter", "Construction", "Sailing"}

// Progress measures targets against current XP. Targets without a start get
// the current XP as their start; the bool reports whether any did, so the
// caller can save them.
func Progress(targets []SkillTarget, sk Skills) ([]SkillProgress, bool) {
	changed := false
	out := make([]SkillProgress, 0, len(targets))
	for i, t := range targets {
		xp, ok := sk[t.Skill]
		if !ok {
			continue
		}
		if t.StartXP == 0 {
			targets[i].StartXP, t.StartXP, changed = xp, xp, true
		}
		goal := xpFor(t.Level)
		out = append(out, SkillProgress{SkillTarget: t, XP: xp, Current: levelFor(xp), GoalXP: goal, Icon: t.Skill + "_icon.png",
			Done: xp >= goal, ToGo: max(goal-xp, 0), Progress: max(xp-t.StartXP, 0)})
	}
	return out, changed
}

// skillsEvery is how often the hiscores are read; they only change when
// Joe logs out or hops worlds.
const skillsEvery = 10 * time.Minute
