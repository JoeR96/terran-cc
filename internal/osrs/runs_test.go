package osrs

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

const fixture = `#RuneLite
rsprofile.rsprofile.AAA.displayName=big chopski
rsprofile.rsprofile.BBB.displayName=Big Chopski
rsprofile.rsprofile.CCC.displayName=someone else
timetracking.rsprofile.AAA.12083.4774=36\:1700000000
timetracking.rsprofile.BBB.birdhouse.1626=24\:%[1]d
timetracking.rsprofile.BBB.birdhouse.1627=24\:%[2]d
timetracking.rsprofile.BBB.birdhouse.1628=0\:%[2]d
timetracking.rsprofile.BBB.10548.4774=33\:%[2]d
timetracking.rsprofile.BBB.11062.4774=38\:%[3]d
timetracking.rsprofile.BBB.4922.4775=38\:%[2]d
timetracking.rsprofile.BBB.contract=267
timetracking.rsprofile.BBB.farmTickOffset=0
timetracking.rsprofile.BBB.farmTickOffsetPrecision=64
`

func fixtureTimers(t *testing.T, now time.Time) timers {
	t.Helper()
	src := strings.NewReplacer("%[1]d", itoa(now.Add(-2*time.Hour).Unix()), "%[2]d", itoa(now.Add(-10*time.Minute).Unix()), "%[3]d", itoa(now.Add(-26*time.Hour).Unix())).Replace(fixture)
	props := readProperties(strings.NewReader(src))
	key := profileKey(props, "big chopski")
	if key != "BBB" {
		t.Fatalf("profile key %q", key)
	}
	return readTimers(props, key)
}

func TestReadPropertiesUnescapes(t *testing.T) {
	p := readProperties(strings.NewReader("a.b=1\\:2\nc\\=d=x\\u0041\n# skip\nlong=one\\\n  two\n"))
	if p["a.b"] != "1:2" || p["c=d"] != "xA" || p["long"] != "onetwo" {
		t.Fatalf("%v", p)
	}
}

func TestRuns(t *testing.T) {
	now := time.Date(2026, 10, 17, 20, 0, 0, 0, time.Local)
	tm := fixtureTimers(t, now)
	runs := map[string]Run{}
	for _, r := range tm.Runs(now) {
		runs[r.Key] = r
	}
	bh := runs["birdhouses"]
	// One seeded 2h ago (full), one seeded 10m ago (40m left), one empty.
	if bh.State != "growing" || bh.Total != 3 || bh.Ready != 2 || !bh.DoneToday || bh.ReadyAt != now.Add(40*time.Minute).Unix() {
		t.Fatalf("bird houses %+v", bh)
	}
	h := runs["herbs"]
	// Ardougne ranarr planted 10 minutes ago is growing; Catherby and the
	// guild have harvestable herbs from yesterday.
	if h.State != "growing" || h.Total != 3 || h.Ready != 2 || !h.DoneToday || h.ReadyAt <= now.Unix() {
		t.Fatalf("herbs %+v", h)
	}
	c := runs["contract"]
	// Seen harvestable 10 minutes ago is not the same as planted today.
	if c.Note != "Dwarf weed" || c.Total != 1 || c.State != "ready" || c.DoneToday {
		t.Fatalf("contract %+v", c)
	}
	if runs["hespori"].State != "unknown" || runs["trees"].Total != 0 {
		t.Fatalf("unseen runs %+v %+v", runs["hespori"], runs["trees"])
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
