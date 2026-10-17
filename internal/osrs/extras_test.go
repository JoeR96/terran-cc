package osrs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtras(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/mapping"):
			w.Write([]byte(`[{"id":27684,"name":"Voidwaker blade"},{"id":13231,"name":"Primordial crystal"}]`))
		case strings.HasPrefix(r.URL.Path, "/temple"):
			w.Write([]byte(`{"data":{"items":[{"id":27684,"count":1},{"id":99999,"count":1}]}}`))
		case strings.HasPrefix(r.URL.Path, "/sync/"):
			w.Write([]byte(`{"achievement_diaries":{"Falador":{"Elite":{"complete":false,"tasks":[false,true,true]}},"Karamja":{"Elite":{"complete":true,"tasks":[true]}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	defer func(a, b, c string) { templeURL, mappingURL, wikisyncURL = a, b, c }(templeURL, mappingURL, wikisyncURL)
	templeURL, mappingURL, wikisyncURL = srv.URL+"/temple?player=", srv.URL+"/mapping", srv.URL+"/sync/%s"

	s := NewService(Config{RuneLite: t.TempDir()})
	s.readExtras(context.Background())
	f := s.Facts()
	if !f.Obtained["voidwaker blade"] || f.Obtained["primordial crystal"] || len(f.Obtained) != 1 {
		t.Fatalf("obtained %v", f.Obtained)
	}
	if f.Diaries["Falador"] != (Diary{Done: 2, Total: 3}) || !f.Diaries["Karamja"].Complete {
		t.Fatalf("diaries %v", f.Diaries)
	}
	if s.Status().Updated.Log == nil {
		t.Fatal("log time not set")
	}
	b := bonesIn(map[string]int{"Dragon bones": 10, "Big bones": 2, "Rune": 5})
	if b.Count != 12 || b.XP != int((10*72+2*15)*3.5) {
		t.Fatalf("bones %+v", b)
	}
}
