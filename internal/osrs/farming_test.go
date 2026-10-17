package osrs

import "testing"

// Expected values below are worked by hand from RuneLite's getTickTime and
// predictPatch.
const t0 = 1200*1_500_000 + 345 // 345s into a 20-minute tick

func TestTickTime(t *testing.T) {
	if got := tickTime(20, 2, t0, offset{}); got != 1_800_002_400 {
		t.Errorf("no offset: %d", got)
	}
	// A 7-minute offset moves the tick boundaries 420s earlier.
	if got := tickTime(20, 0, t0, offset{Mins: 7, Precision: 64, Known: true}); got != 1_799_999_580 {
		t.Errorf("offset: %d", got)
	}
	// An offset measured too coarsely for this rate is ignored.
	if got := tickTime(20, 0, t0, offset{Mins: 7, Precision: 5, Known: true}); got != 1_800_000_000 {
		t.Errorf("coarse offset: %d", got)
	}
}

func TestPredictGrowingHerb(t *testing.T) {
	// Ranarr at stage 1 of 5, seen at t0; 25 minutes later it is at stage 2.
	p, ok := predict("HERB", 33, t0, t0+1500, offset{}, false)
	if !ok || p.State.Produce != PRANARR || p.State.Crop != cropGrowing {
		t.Fatalf("%+v %v", p, ok)
	}
	if p.DoneAt != 1_800_003_600 || p.Stage != 2 || p.Planted != 1_799_998_800 {
		t.Fatalf("done %d stage %d planted %d", p.DoneAt, p.Stage, p.Planted)
	}
}

func TestPredictRegrowingBush(t *testing.T) {
	// Harvestable whiteberries regrow a berry every 20 minutes, 5 lives.
	p, ok := predict("BUSH", 60, t0, t0, offset{}, false)
	if !ok || p.State.Produce != PWHITEBERRIES || p.State.Crop != cropHarvestable || p.DoneAt != 1_800_000_000+3*1200 {
		t.Fatalf("%+v", p)
	}
}

func TestPredictRejects(t *testing.T) {
	if _, ok := predict("NOPE", 1, t0, t0, offset{}, false); ok {
		t.Error("unknown patch type")
	}
	if _, ok := predict("HESPORI", 200, t0, t0, offset{}, false); ok {
		t.Error("value outside the table")
	}
	if _, ok := predict("HERB", 33, 0, t0, offset{}, false); ok {
		t.Error("no timestamp")
	}
	if p, _ := predict("HERB", 0, t0, t0, offset{}, true); p.DoneAt != 0 {
		t.Errorf("autoweed weeds should not tick: %+v", p)
	}
}

func TestTablesLookComplete(t *testing.T) {
	for _, n := range []string{"HERB", "BUSH", "CACTUS", "TREE", "FRUIT_TREE", "HARDWOOD_TREE", "CALQUAT", "HESPORI"} {
		if _, ok := implementations[n]; !ok {
			t.Errorf("missing %s", n)
		}
	}
	if produces[PDWARF_WEED].ItemID != 267 || produces[PPOTATO_CACTUS].Tickrate != 10 || produces[PMAGIC].Stages != 13 {
		t.Errorf("produce table: %+v %+v %+v", produces[PDWARF_WEED], produces[PPOTATO_CACTUS], produces[PMAGIC])
	}
}
