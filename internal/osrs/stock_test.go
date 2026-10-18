package osrs

import (
	"strings"
	"testing"
)

func TestStocks(t *testing.T) {
	have := map[string]int{}
	readContainer(strings.NewReader(`{"id":2434,"quantity":288,"name":"Prayer potion(4)"}
{"id":139,"quantity":226,"name":"Prayer potion(3)"}
{"id":143,"quantity":5,"name":"Prayer potion(1)"}
{"id":13441,"quantity":78,"name":"Anglerfish"}
{"id":13439,"quantity":3289,"name":"Raw anglerfish"}
{"id":21978,"quantity":2,"name":"Super restore mix(2)"}
not json
{"id":13441,"quantity":2,"name":"Anglerfish"}
`), have)
	items := []StockItem{
		{Key: "prayer", Names: []string{"Prayer potion"}, Doses: true, Sufficient: 60, Target: 1200},
		{Key: "angler", Names: []string{"Anglerfish"}, Raw: []string{"Raw anglerfish"}, Sufficient: 1000, Target: 20000},
		{Key: "restore", Names: []string{"Super restore"}, Doses: true, Sufficient: 60, Target: 1200},
		{Key: "brew", Names: []string{"Saradomin brew"}, Doses: true, Sufficient: 0, Target: 0},
	}
	got := map[string]Stock{}
	for _, s := range Stocks(items, have) {
		got[s.Key] = s
	}
	if p := got["prayer"]; p.Have != 288*4+226*3+5 || p.Status != "full" {
		t.Errorf("prayer %+v", p)
	}
	if a := got["angler"]; a.Have != 80 || a.RawHas != 3289 || a.Status != "low" {
		t.Errorf("anglerfish %+v", a)
	}
	// A mix is not a super restore, so it doesn't count.
	if r := got["restore"]; r.Have != 0 || r.Status != "low" {
		t.Errorf("restore %+v", r)
	}
	if b := got["brew"]; b.Status != "full" {
		t.Errorf("zero target is met: %+v", b)
	}
	// Best tier: Extended anti-venom+ is held, so it's what counts.
	tiers := Stocks([]StockItem{{Key: "av", Names: []string{"Extended anti-venom+", "Anti-venom+", "Anti-venom"}, Doses: true, Best: true, Sufficient: 20, Target: 400}},
		map[string]int{"Extended anti-venom+(4)": 1, "Extended anti-venom+(1)": 2, "Anti-venom(4)": 564})[0]
	if tiers.Tier != "Extended anti-venom+" || tiers.Have != 6 || tiers.Lower != 2256 || tiers.Status != "low" {
		t.Errorf("best tier %+v", tiers)
	}
	for _, d := range DefaultStock() {
		if d.Sufficient != d.Target/20 || d.Icon == "" || len(d.Names) == 0 {
			t.Errorf("default %+v", d)
		}
	}
}
