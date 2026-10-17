package osrs

import (
	"fmt"
	"slices"
	"strconv"
	"time"
)

// Run is one of Joe's daily runs as the widget shows it.
type Run struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`            // OSRS Wiki file name
	State     string `json:"state"`           // ready | growing | unknown
	ReadyAt   int64  `json:"ready_at"`        // unix; when the last growing patch finishes
	DoneToday bool   `json:"done_today"`      // planted or seeded since local midnight
	Ready     int    `json:"ready"`           // patches ready (or empty, waiting to be planted)
	Total     int    `json:"total"`           // patches RuneLite has seen
	Note      string `json:"note,omitempty"`  // e.g. the contract crop
	Issue     string `json:"issue,omitempty"` // e.g. "1 diseased"
}

// birdHouseMinutes is how long a seeded bird house takes to fill.
const birdHouseMinutes = 50

var farmRuns = []struct {
	key, name, icon string
	impls           []string
}{
	{"herbs", "Herb run", "Ranarr_weed.png", []string{"HERB"}},
	{"whiteberries", "Whiteberries", "White_berries.png", []string{"BUSH"}},
	{"cacti", "Potato cacti", "Potato_cactus.png", []string{"CACTUS"}},
	{"hespori", "Hespori", "Hespori.png", []string{"HESPORI"}},
	{"trees", "Tree run", "Magic_logs.png", []string{"TREE", "FRUIT_TREE", "HARDWOOD_TREE", "CALQUAT"}},
}

// farmingGuild is the region the contract patches live in.
const farmingGuild = 4922

type seenPatch struct {
	region region
	patch  patch
	pred   prediction
}

func (t timers) seen(now int64, keep func(region, patch) bool) []seenPatch {
	var out []seenPatch
	for _, r := range regions {
		for _, p := range r.Patches {
			if !keep(r, p) {
				continue
			}
			s, ok := t.Patches[strconv.Itoa(r.ID)+"."+strconv.Itoa(p.Varbit)]
			if !ok {
				continue
			}
			if pr, ok := predict(p.Impl, s.Value, s.Stamp, now, t.Offset, t.Autoweed); ok {
				out = append(out, seenPatch{r, p, pr})
			}
		}
	}
	return out
}

// summarize folds patches into one run.
func summarize(run Run, ps []seenPatch, now, midnight int64) Run {
	run.Total = len(ps)
	diseased, dead := 0, 0
	for _, s := range ps {
		growing := s.pred.State.Crop == cropGrowing && s.pred.State.Produce != PWEEDS && s.pred.DoneAt > now
		switch {
		case growing:
			run.ReadyAt = max(run.ReadyAt, s.pred.DoneAt)
		case s.pred.State.Crop == cropDiseased:
			diseased++
		case s.pred.State.Crop == cropDead:
			dead++
		default:
			run.Ready++
		}
		// Only a growing crop tells us when it was planted; a harvestable
		// one may just have been looked at today.
		if s.pred.State.Crop == cropGrowing && s.pred.State.Produce != PWEEDS && s.pred.Planted >= midnight {
			run.DoneToday = true
		}
	}
	switch {
	case run.Total == 0:
		run.State = "unknown"
	case run.ReadyAt > now:
		run.State = "growing"
	default:
		run.State = "ready"
	}
	if diseased > 0 {
		run.Issue = fmt.Sprintf("%d diseased", diseased)
	}
	if dead > 0 {
		run.Issue = joinIssue(run.Issue, fmt.Sprintf("%d dead", dead))
	}
	return run
}

func joinIssue(a, b string) string {
	if a == "" {
		return b
	}
	return a + ", " + b
}

// Runs works out every daily run at time now.
func (t timers) Runs(now time.Time) []Run {
	n := now.Unix()
	y, m, d := now.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, now.Location()).Unix()
	out := []Run{t.birdHouses(n, midnight)}
	for _, fr := range farmRuns {
		ps := t.seen(n, func(r region, p patch) bool { return slices.Contains(fr.impls, p.Impl) })
		out = append(out, summarize(Run{Key: fr.key, Name: fr.name, Icon: fr.icon}, ps, n, midnight))
	}
	out = append(out, t.contract(n, midnight))
	return out
}

func (t timers) birdHouses(now, midnight int64) Run {
	run := Run{Key: "birdhouses", Name: "Bird houses", Icon: "Bird_house.png", State: "unknown"}
	for _, h := range t.BirdHouses {
		if h.Stamp == 0 {
			continue
		}
		run.Total++
		seeded := h.Value > 0 && h.Value%3 == 0
		if !seeded {
			run.Ready++ // empty or built without seeds: needs doing
			continue
		}
		done := h.Stamp + birdHouseMinutes*60
		if done > now {
			run.ReadyAt = max(run.ReadyAt, done)
		} else {
			run.Ready++
		}
		if h.Stamp >= midnight {
			run.DoneToday = true
		}
	}
	switch {
	case run.Total == 0:
	case run.ReadyAt > now:
		run.State = "growing"
	default:
		run.State = "ready"
	}
	return run
}

// contract follows the Farming Guild patch of the contracted crop's type.
func (t timers) contract(now, midnight int64) Run {
	run := Run{Key: "contract", Name: "Farming contract", Icon: "Seed_pack.png", State: "unknown"}
	var crop *produceInfo
	for i := range produces {
		if produces[i].ItemID == t.Contract && t.Contract != 0 && produces[i].Contract > 0 {
			crop = &produces[i]
		}
	}
	if crop == nil {
		run.Note = "no contract"
		return run
	}
	run.Note = crop.Name
	ps := t.seen(now, func(r region, p patch) bool { return r.ID == farmingGuild && p.Impl == crop.Impl })
	return summarize(run, ps, now, midnight)
}
