package osrs

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"terrancc/internal/model"
)

// Facts is what the game data says, for ticking OSRS to-dos off by itself.
type Facts struct {
	Obtained map[string]bool // collection log item names, lower case
	Have     map[string]int  // bank + inventory + equipment, by name
	XP       map[string]int  // skill XP, from the hiscores
	Diaries  map[string]Diary
}

// Diary is one area's elite tier progress.
type Diary struct {
	Done, Total int
	Complete    bool
}

// Change is one edit the facts call for.
type Change struct {
	ID    int64
	Done  bool
	Title string // new title, "" to keep
	Why   string // note explaining the change
}

var (
	// "91 Smithing (rune crossbow)": a level requirement.
	levelReq = regexp.MustCompile(`^(\d{1,2}) ([A-Z][a-z]+)\b`)
	// "716 adamantite nails": a material to gather.
	material = regexp.MustCompile(`^(\d[\d,]*) ([^(]+?)\s*(\(.*\))?$`)
	// "Firemaking 75 to 90 (4.09M XP)": a Base 90s line.
	baseLine = regexp.MustCompile(`^([A-Z][a-z]+) \d+ to (\d+) \(`)
	// "Ardougne elite (1/8)": a diary heading.
	diaryLine = regexp.MustCompile(`^(.+) elite \((\d+)/(\d+)\)$`)
)

func isSkill(s string) bool {
	for _, k := range skillOrder {
		if k == s {
			return true
		}
	}
	return false
}

// bankCount finds a material in the bank, trying the plural and singular
// forms the to-do and the bank might each use.
func bankCount(have map[string]int, name string) (int, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	cands := []string{name, strings.TrimSuffix(name, "s"), strings.TrimSuffix(name, "es"), name + "s"}
	if strings.HasPrefix(name, "bolts of ") {
		cands = append(cands, "bolt of "+strings.TrimPrefix(name, "bolts of "))
	}
	for item, q := range have {
		l := strings.ToLower(item)
		for _, c := range cands {
			if l == c {
				return q, true
			}
		}
	}
	return 0, false
}

// baseName is a title without its bracketed note: "Hilt (Artio)" → "Hilt".
func baseName(title string) string {
	if i := strings.Index(title, " ("); i > 0 {
		title = title[:i]
	}
	return strings.ToLower(strings.TrimSpace(title))
}

// Plan works out which open OSRS to-dos the facts finish or retitle.
func Plan(items []model.Item, f Facts) []Change {
	var out []Change
	for _, it := range items {
		if it.Context != model.ContextOSRS || it.ProjectID != nil || it.Status == model.StatusDone || it.Status == model.StatusSomeday {
			continue
		}
		t := strings.TrimSpace(it.Title)
		switch {
		case f.Obtained[baseName(t)]:
			out = append(out, Change{ID: it.ID, Done: true, Why: "In the collection log."})
		case diaryLine.MatchString(t) && f.Diaries != nil:
			m := diaryLine.FindStringSubmatch(t)
			d, ok := f.Diaries[m[1]]
			if !ok {
				continue
			}
			if d.Complete {
				out = append(out, Change{ID: it.ID, Done: true, Title: fmt.Sprintf("%s elite (%d/%d)", m[1], d.Total, d.Total), Why: "Diary complete (WikiSync)."})
			} else if want := fmt.Sprintf("%s elite (%d/%d)", m[1], d.Done, d.Total); want != t {
				out = append(out, Change{ID: it.ID, Title: want, Why: "Progress from WikiSync."})
			}
		case baseLine.MatchString(t) && f.XP != nil:
			m := baseLine.FindStringSubmatch(t)
			xp, ok := f.XP[m[1]]
			goal, _ := strconv.Atoi(m[2])
			if !ok {
				continue
			}
			lv := levelFor(xp)
			if lv >= goal {
				out = append(out, Change{ID: it.ID, Done: true, Why: fmt.Sprintf("%s is %d.", m[1], lv)})
			} else if want := fmt.Sprintf("%s %d to %d (%.2fM XP)", m[1], lv, goal, float64(xpFor(goal)-xp)/1e6); want != t {
				out = append(out, Change{ID: it.ID, Title: want})
			}
		case levelReq.MatchString(t) && isSkill(levelReq.FindStringSubmatch(t)[2]):
			m := levelReq.FindStringSubmatch(t)
			need, _ := strconv.Atoi(m[1])
			if xp, ok := f.XP[m[2]]; ok && levelFor(xp) >= need {
				lv := levelFor(xp)
				out = append(out, Change{ID: it.ID, Done: true, Why: fmt.Sprintf("%s is %d.", m[2], lv)})
			}
		case it.ParentID != nil && material.MatchString(t) && f.Have != nil:
			m := material.FindStringSubmatch(t)
			need, _ := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
			if q, ok := bankCount(f.Have, m[2]); ok && q >= need {
				out = append(out, Change{ID: it.ID, Done: true, Why: fmt.Sprintf("%d in the bank.", q)})
			}
		}
	}
	return out
}
