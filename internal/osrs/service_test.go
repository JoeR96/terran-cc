package osrs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func writeRuneLite(t *testing.T, dir string, seeded time.Time, prayer4 int) {
	t.Helper()
	os.MkdirAll(filepath.Join(dir, "profiles2"), 0o755)
	os.MkdirAll(filepath.Join(dir, "Data Exports"), 0o755)
	props := "rsprofile.rsprofile.K.displayName=big chopski\n" +
		"timetracking.rsprofile.K.birdhouse.1626=24\\:" + strconv.FormatInt(seeded.Unix(), 10) + "\n"
	os.WriteFile(filepath.Join(dir, "profiles2", "$rsprofile--1.properties"), []byte(props), 0o644)
	bank := `{"id":2434,"quantity":` + strconv.Itoa(prayer4) + `,"name":"Prayer potion(4)"}` + "\n"
	os.WriteFile(filepath.Join(dir, "Data Exports", "container_bank.json"), []byte(bank), 0o644)
}

func TestServiceWatchesFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 17, 20, 0, 0, 0, time.Local)
	writeRuneLite(t, dir, now.Add(-10*time.Minute), 100)
	cfg := DefaultConfig()
	cfg.RuneLite = dir
	s := NewService(cfg)
	s.Now = func() time.Time { return now }
	var ready []Run
	s.OnReady = func(r Run) { ready = append(ready, r) }

	if !s.refresh() {
		t.Fatal("first read should change")
	}
	s.checkReady()
	st := s.Status()
	if st.Runs[0].Key != "birdhouses" || st.Runs[0].State != "growing" || st.Updated.Bank == nil {
		t.Fatalf("%+v", st)
	}
	var prayer Stock
	for _, x := range st.Stock {
		if x.Key == "prayer" {
			prayer = x
		}
	}
	if prayer.Have != 400 || prayer.Status != "ok" {
		t.Fatalf("prayer %+v", prayer)
	}
	if s.refresh() {
		t.Fatal("nothing changed on disk")
	}

	// 41 minutes later the bird house is full: one ready event.
	now = now.Add(41 * time.Minute)
	if !s.checkReady() || len(ready) != 1 || ready[0].Key != "birdhouses" {
		t.Fatalf("ready %+v", ready)
	}
	if s.checkReady() || len(ready) != 1 {
		t.Fatal("ready fires once")
	}

	// A new bank export is picked up.
	time.Sleep(20 * time.Millisecond)
	writeRuneLite(t, dir, now, 10)
	os.Chtimes(filepath.Join(dir, "Data Exports", "container_bank.json"), now, now.Add(time.Second))
	if !s.refresh() {
		t.Fatal("bank change missed")
	}
	for _, x := range s.Status().Stock {
		if x.Key == "prayer" && (x.Have != 40 || x.Status != "low") {
			t.Fatalf("after restock %+v", x)
		}
	}
}

func TestServiceSkillsSaveStart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"skills":[{"name":"Prayer","xp":3846172},{"name":"Construction","xp":13034431}]}`))
	}))
	defer srv.Close()
	s := NewService(Config{RuneLite: t.TempDir()})
	s.HiscoresURL = srv.URL + "/?player="
	var saved []Config
	s.Save = func(c Config) error { saved = append(saved, c); return nil }
	s.readSkills(context.Background())
	if len(saved) != 1 || len(saved[0].Targets) != 2 || saved[0].Targets[0].StartXP != 3846172 {
		t.Fatalf("%+v", saved)
	}
	st := s.Status()
	if len(st.Skills) != 2 || !st.Skills[1].Done || len(st.Warnings) != 2 {
		t.Fatalf("%+v", st)
	}
}

func TestOldConfigGetsDefaultRecipes(t *testing.T) {
	s := NewService(Config{Stock: []StockItem{{Key: "prayer", Names: []string{"Prayer potion"}, Doses: true, Target: 1200}}})
	if len(s.Config().Stock[0].Recipes) == 0 {
		t.Fatal("prayer should have picked up its recipe")
	}
}

func TestBuiltInItemsKeepTheirOwnRecipes(t *testing.T) {
	// A saved config where recipes were attached to the wrong items.
	s := NewService(Config{Stock: []StockItem{
		{Key: "divine-ranging", Names: []string{"Divine ranging potion"}, Doses: true, Recipes: superCombatRecipes()},
		{Key: "anti-venom", Names: []string{"Anti-venom"}, Doses: true, Recipes: rangingRecipes()},
		{Key: "my-own", Names: []string{"Shark"}, Recipes: dartRecipes()},
	}})
	st := s.Config().Stock
	if st[0].Recipes[0].Makes != doseUnit("Divine ranging potion") || len(st[1].Recipes) != 0 || !st[1].Best || st[2].Recipes[0].Makes != "Amethyst dart" {
		t.Fatalf("%+v", st)
	}
}
