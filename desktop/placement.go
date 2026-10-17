package main

type rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type screen struct {
	ID      string
	Work    rect
	Primary bool
}

// placeOnScreen decides where a saved window reopens. It stays put when its
// monitor still exists and enough of it is on that monitor; otherwise it is
// centred on the primary. Either way it is shrunk and moved to fit the work
// area. It returns the bounds and the monitor they are on.
func placeOnScreen(saved rect, monitor string, screens []screen) (rect, string) {
	if len(screens) == 0 {
		return saved, monitor
	}
	for _, s := range screens {
		if s.ID == monitor && overlap(saved, s.Work) >= 80 {
			return clampInto(saved, s.Work), s.ID
		}
	}
	// Monitor IDs come from HMONITOR handles, which change across reboots,
	// so a window still sitting on some monitor stays on that one.
	for _, s := range screens {
		if overlap(saved, s.Work) >= 80 {
			return clampInto(saved, s.Work), s.ID
		}
	}
	p := screens[0]
	for _, s := range screens {
		if s.Primary {
			p = s
			break
		}
	}
	r := rect{W: min(saved.W, p.Work.W), H: min(saved.H, p.Work.H)}
	r.X = p.Work.X + (p.Work.W-r.W)/2
	r.Y = p.Work.Y + (p.Work.H-r.H)/2
	return r, p.ID
}

// overlap is the smaller side of the intersection, so a thin sliver counts
// as off the screen.
func overlap(a, b rect) int {
	w := min(a.X+a.W, b.X+b.W) - max(a.X, b.X)
	h := min(a.Y+a.H, b.Y+b.H) - max(a.Y, b.Y)
	if w <= 0 || h <= 0 {
		return 0
	}
	return min(w, h)
}

func clampInto(r, area rect) rect {
	r.W = min(r.W, area.W)
	r.H = min(r.H, area.H)
	r.X = min(max(r.X, area.X), area.X+area.W-r.W)
	r.Y = min(max(r.Y, area.Y), area.Y+area.H-r.H)
	return r
}
