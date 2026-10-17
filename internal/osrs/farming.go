// Package osrs reads what RuneLite already writes to disk (farming and bird
// house timers, the bank export) plus the hiscores, and turns them into
// Joe's daily runs, stock levels and skill targets.
package osrs

//go:generate python -I gen/convert.py farming_gen.go
//go:generate gofmt -w farming_gen.go

// The farming timer rules below are a port of RuneLite's FarmingTracker
// (BSD-2, see farming_gen.go for the notice); the tables themselves are
// generated.

type produce int

type produceInfo struct {
	Name          string
	Contract      int    // FARMGUILD_CONTRACT_TYPE value, -1 if none
	Impl          string // patch type it grows in
	ItemID        int    // what the farming contract key stores
	Tickrate      int    // minutes per growth stage
	Stages        int
	Regrow        int // minutes per stage while regrowing (bushes, fruit trees)
	HarvestStages int
}

type cropState int

const (
	cropHarvestable cropState = iota
	cropGrowing
	cropDiseased
	cropDead
	cropEmpty
	cropFilling
)

type patchState struct {
	Produce produce
	Crop    cropState
	Stage   int
}

type impl struct {
	health bool // needs a health check before harvesting (trees, bushes)
	state  func(value int) *patchState
}

type patch struct {
	Name   string
	Varbit int
	Impl   string
}

type region struct {
	Name    string
	ID      int
	Patches []patch
}

// stages and tickrate follow RuneLite's PatchState: harvestable and filling
// count lives, and only growing and harvestable states tick.
func (s patchState) stages() int {
	p := produces[s.Produce]
	if s.Crop == cropHarvestable || s.Crop == cropFilling {
		return p.HarvestStages
	}
	return p.Stages
}

func (s patchState) tickrate() int {
	p := produces[s.Produce]
	switch s.Crop {
	case cropHarvestable:
		return p.Regrow
	case cropGrowing:
		return p.Tickrate
	}
	return 0
}

// offset is the farm-tick offset RuneLite measured for this account.
type offset struct {
	Mins, Precision int
	Known           bool
}

// tickTime is RuneLite's getTickTime: the moment `ticks` growth ticks after
// the tick containing t, for a crop growing every rate minutes.
func tickTime(rate, ticks int, t int64, off offset) int64 {
	var shift int64
	if off.Known && (off.Precision >= rate || off.Precision >= 40) {
		shift = int64(off.Mins%rate) * 60
	}
	now := t + shift
	period := int64(rate) * 60
	current := now - now%period
	return current + int64(ticks)*period - shift
}

// prediction is what a patch will be: its state, when it finishes growing
// (0 when it isn't growing) and the stage it has reached by now.
type prediction struct {
	State   patchState
	DoneAt  int64
	Stage   int
	Planted int64 // estimated planting time, for "done today"
}

// predict ports RuneLite's predictPatch for a stored value:timestamp pair.
func predict(implName string, value int, stamp, now int64, off offset, autoweed bool) (prediction, bool) {
	im, ok := implementations[implName]
	if !ok || stamp <= 0 {
		return prediction{}, false
	}
	st := im.state(value)
	if st == nil {
		return prediction{}, false
	}
	stage, stages, rate := st.Stage, st.stages(), st.tickrate()
	if autoweed && st.Produce == PWEEDS {
		stage, stages, rate = 0, 1, 0
	}
	p := prediction{State: *st, Stage: stage, Planted: stamp}
	if rate > 0 {
		tNow := tickTime(rate, 0, now, off)
		tStamp := tickTime(rate, 0, stamp, off)
		p.DoneAt = tickTime(rate, stages-1-stage, tStamp, off)
		p.Planted = tStamp - int64(st.Stage)*int64(rate)*60
		p.Stage = min(stage+int((tNow-tStamp)/int64(rate*60)), stages-1)
	}
	return p, true
}
