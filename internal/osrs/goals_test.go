package osrs

import (
	"testing"

	"terrancc/internal/model"
)

func osrsItem(id int64, title string, parent int64) model.Item {
	it := model.Item{ID: id, Title: title, Context: model.ContextOSRS, Status: model.StatusBacklog}
	if parent != 0 {
		it.ParentID = &parent
	}
	return it
}

func TestPlan(t *testing.T) {
	items := []model.Item{
		osrsItem(1, "Voidwaker (blade done)", 0),
		osrsItem(2, "Voidwaker hilt (Artio or Callisto)", 1),
		osrsItem(3, "Voidwaker gem", 1),
		osrsItem(4, "Great white shark boat", 0),
		osrsItem(5, "716 adamantite nails", 4),
		osrsItem(6, "26 camphor planks", 4),
		osrsItem(7, "10 bolts of canvas", 4),
		osrsItem(8, "91 Smithing (rune crossbow from scratch)", 9),
		osrsItem(10, "85 Firemaking (pyre ship)", 9),
		osrsItem(11, "Firemaking 75 to 90 (4.09M XP)", 12),
		osrsItem(13, "Defence 89 to 90 (0.15M XP)", 12),
		osrsItem(14, "Ardougne elite (1/8)", 15),
		osrsItem(16, "Karamja elite (3/5)", 15),
		osrsItem(17, "1,000 chompy kills (chompy bird hat)", 9),
	}
	done := items[2]
	done.Status = model.StatusDone // already done: left alone
	items[2] = done
	f := Facts{
		Obtained: map[string]bool{"voidwaker hilt": true, "voidwaker gem": true},
		Have:     map[string]int{"Adamantite nails": 900, "Camphor plank": 3, "Bolt of canvas": 10},
		XP:       map[string]int{"Smithing": xpFor(91), "Firemaking": xpFor(76) + 10, "Defence": xpFor(90)},
		Diaries:  map[string]Diary{"Ardougne": {Done: 3, Total: 8}, "Karamja": {Done: 5, Total: 5, Complete: true}},
	}
	got := map[int64]Change{}
	for _, c := range Plan(items, f) {
		got[c.ID] = c
	}
	want := map[int64]Change{
		2:  {ID: 2, Done: true},
		5:  {ID: 5, Done: true},
		7:  {ID: 7, Done: true},
		8:  {ID: 8, Done: true},
		11: {ID: 11, Title: "Firemaking 76 to 90 (4.01M XP)"},
		13: {ID: 13, Done: true},
		14: {ID: 14, Title: "Ardougne elite (3/8)"},
		16: {ID: 16, Done: true, Title: "Karamja elite (5/5)"},
	}
	for id, w := range want {
		g, ok := got[id]
		if !ok || g.Done != w.Done || (w.Title != "" && g.Title != w.Title) {
			t.Errorf("#%d: got %+v, want %+v", id, g, w)
		}
	}
	for id := range got {
		if _, ok := want[id]; !ok {
			t.Errorf("#%d changed unexpectedly: %+v", id, got[id])
		}
	}
}
