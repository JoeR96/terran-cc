package strength

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	"terrancc/internal/core"
	"terrancc/internal/model"
	"terrancc/internal/store"
)

// 99-Strength's habit types, lower-cased. They become HabitDay.Kind.
const (
	kindCounter  = "counter"
	kindNumeric  = "numeric"
	kindCheckbox = "checkbox"
)

// Body weight limits, from BodyWeightEntry.MinKg / MaxKg in 99-Strength.
const (
	minKg = 30
	maxKg = 300
)

// icons turns the icon keys the phone app draws (habitIcons.ts) into the
// emoji Terran CC shows. An icon that is already an emoji passes through.
var icons = map[string]string{
	"droplets":           "💧",
	"footprints":         "🚶",
	"scale":              "⚖️",
	"pill":               "💊",
	"stretch-horizontal": "🧘",
	"dumbbell":           "🏋️",
	"moon":               "🌙",
	"book-open":          "📖",
	"apple":              "🍎",
	"sun":                "☀️",
}

func iconFor(icon *string) string {
	if icon == nil {
		return ""
	}
	if e, ok := icons[*icon]; ok {
		return e
	}
	for _, r := range *icon {
		if r > unicode.MaxASCII {
			return *icon // an emoji, e.g. from the Terran CC import
		}
	}
	return "" // an icon key this build does not know
}

func isWeighIn(h RemoteHabit) bool {
	return strings.EqualFold(h.Type, kindNumeric) && h.Unit != nil && strings.EqualFold(*h.Unit, "kg")
}

// toHabitDay maps a 99-Strength habit onto Terran CC's habit shape:
// Counter is a count with step 1, Checkbox a check, Numeric the new
// "numeric" type. Hit means the full target; MinHit the bad-day minimum.
func toHabitDay(h RemoteHabit, id int64, date string, value float64, minHit, hit bool, pending int) model.HabitDay {
	d := model.HabitDay{
		Habit: model.Habit{ID: id, Name: h.Name, Icon: iconFor(h.Icon), Target: h.Target, Order: h.Order},
		Date:  date, Value: value, Hit: hit,
		Source: model.SourceStrength, Kind: strings.ToLower(h.Type),
		Minimum: h.Minimum, MinHit: minHit, RemoteID: h.ID, Pending: pending,
	}
	if h.Unit != nil {
		d.Unit = *h.Unit
	}
	switch d.Kind {
	case kindCheckbox:
		d.Type, d.Step = model.HabitCheck, 1
	case kindNumeric:
		d.Type = model.HabitNumeric
	default:
		d.Type, d.Step = model.HabitCount, 1
	}
	return d
}

// words lower-cases s and turns every run of punctuation or space into one
// space, so "weigh-in" and "Weigh in" compare equal.
func words(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		} else {
			space = true
		}
	}
	return b.String()
}

// findLocked resolves a habit the way local habits resolve: a number (the
// id), the exact name, or a unique start of the name, any case. 99-Strength
// names are longer ("Morning weigh-in"), so a unique start of any word in
// the name works too: "weigh-in", "bottle".
func (s *Service) findLocked(ref string) (RemoteHabit, error) {
	ref = strings.TrimSpace(ref)
	hs := s.cache.Today.Habits
	none := fmt.Errorf("%w: no habit matches %q", store.ErrNotFound, ref)
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		for _, h := range hs {
			if s.ids.IDs[h.ID] == id {
				return h, nil
			}
		}
		return RemoteHabit{}, none
	}
	low := strings.ToLower(ref)
	var starts, inside []RemoteHabit
	for _, h := range hs {
		name := strings.ToLower(h.Name)
		if name == low || strings.EqualFold(h.ID, ref) {
			return h, nil
		}
		if strings.HasPrefix(name, low) {
			starts = append(starts, h)
		}
		if w := words(ref); w != "" && strings.Contains(" "+words(h.Name), " "+w) {
			inside = append(inside, h)
		}
	}
	matches := starts
	if len(matches) == 0 {
		matches = inside
	}
	switch len(matches) {
	case 0:
		return RemoteHabit{}, none
	case 1:
		return matches[0], nil
	}
	names := make([]string, len(matches))
	for i, h := range matches {
		names[i] = h.Name
	}
	return RemoteHabit{}, invalid("ambiguous habit %q: %s", ref, strings.Join(names, ", "))
}

func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// planned is the write a log turns into.
type planned struct {
	kind   string
	delta  int
	value  float64 // the value the habit should end up with
	weight bool
	noop   bool // nothing would change, so nothing is sent
}

// plan turns a Terran CC log (empty, +N, -N, =N or toggle) into a write.
// cur is the value on screen: the cached value plus anything still queued.
//
// A single +1 or -1 on a counter is sent as 99-Strength's increment when the
// API is reachable, so a tap on the phone a moment earlier is added to, not
// overwritten. Everything else is "set to value", which is safe to repeat.
func plan(h RemoteHabit, cur float64, lop core.LogOp, live bool) (planned, error) {
	switch strings.ToLower(h.Type) {
	case kindCheckbox:
		v := 1 - math.Min(cur, 1)
		switch {
		case lop.Set != nil:
			v = *lop.Set
		case lop.Delta != nil:
			v = cur + *lop.Delta
		}
		if v >= 1 {
			v = 1
		} else {
			v = 0
		}
		return planned{kind: opSet, value: v, noop: v == cur}, nil

	case kindNumeric:
		var v float64
		switch {
		case lop.Toggle:
			return planned{}, invalid("%s takes a value, not a toggle", h.Name)
		case lop.Set != nil:
			v = *lop.Set
		case lop.Delta != nil: // a bare number: "tcc habit log weigh-in 94.6"
			v = *lop.Delta
		default:
			return planned{}, invalid("%s needs a value, for example: tcc habit log %q 94.6", h.Name, strings.ToLower(h.Name))
		}
		if isWeighIn(h) {
			v = round(v, 1)
			if v < minKg || v > maxKg {
				return planned{}, invalid("a weigh-in must be between %d and %d kg", minKg, maxKg)
			}
			return planned{kind: opSet, value: v, weight: true}, nil
		}
		if v < 0 {
			return planned{}, invalid("%s cannot be negative", h.Name)
		}
		return planned{kind: opSet, value: round(v, 2)}, nil
	}

	// A counter.
	if lop.Toggle {
		return planned{}, invalid("%s is a counter: use +1, -1 or =N", h.Name)
	}
	if lop.Set != nil {
		v := round(math.Max(*lop.Set, 0), 2)
		return planned{kind: opSet, value: v, noop: v == cur}, nil
	}
	d := 1.0
	if lop.Delta != nil {
		d = *lop.Delta
	}
	v := round(math.Max(cur+d, 0), 2)
	// 99-Strength counts in whole things (water is bottles, not ml). An old
	// habit like "water +250" would wreck the day, so refuse the absurd.
	if d > 0 && v > math.Max(h.Target*5, h.Target+20) {
		unit := ""
		if h.Unit != nil {
			unit = " " + *h.Unit
		}
		return planned{}, invalid("%s has a target of %s%s, so +%s looks wrong (use =%s if you mean it)",
			h.Name, num(h.Target), unit, num(d), num(v))
	}
	if v == cur {
		return planned{noop: true, value: v}, nil
	}
	if live && (d == 1 || d == -1) && cur+d >= 0 {
		return planned{kind: opInc, delta: int(d), value: v}, nil
	}
	return planned{kind: opSet, value: v}, nil
}
