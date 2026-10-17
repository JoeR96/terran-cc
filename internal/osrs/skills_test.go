package osrs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestXPCurve(t *testing.T) {
	for _, c := range []struct{ level, xp int }{{1, 0}, {2, 83}, {85, 3258594}, {90, 5346332}, {99, 13034431}} {
		if got := xpFor(c.level); got != c.xp {
			t.Errorf("xpFor(%d) = %d, want %d", c.level, got, c.xp)
		}
	}
	if levelFor(5346331) != 89 || levelFor(5346332) != 90 || levelFor(200_000_000) != 99 {
		t.Error("levelFor")
	}
}

func TestFetchSkillsAndProgress(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		w.Write([]byte(`{"skills":[{"name":"Overall","xp":1},{"name":"Prayer","xp":3846172},{"name":"Ranged","xp":50759397},{"name":"Construction","xp":4312979}]}`))
	}))
	defer srv.Close()
	sk, err := FetchSkills(context.Background(), srv.Client(), srv.URL+"/?player=", "big chopski", "1.0")
	if err != nil || sk["Prayer"] != 3846172 || ua != "terran-cc/1.0" {
		t.Fatalf("%v %v %q", sk, err, ua)
	}
	targets := DefaultTargets(sk)
	// Prayer and Construction are under 90; Ranged isn't; plus 99 Construction.
	if len(targets) != 3 || targets[0].Skill != "Prayer" || targets[2].Level != 99 {
		t.Fatalf("%+v", targets)
	}
	prog, changed := Progress(targets, sk)
	if !changed || targets[0].StartXP != 3846172 {
		t.Fatalf("start not recorded: %+v", targets)
	}
	p := prog[0]
	if p.Current != 86 || p.GoalXP != 5346332 || p.ToGo != 5346332-3846172 || p.Progress != 0 || p.Icon != "Prayer_icon.png" {
		t.Fatalf("%+v", p)
	}
	sk["Prayer"] += 1000
	prog, changed = Progress(targets, sk)
	if changed || prog[0].Progress != 1000 {
		t.Fatalf("second pass: %+v %v", prog[0], changed)
	}
}
