package main

import "testing"

var screens = []screen{
	{ID: "\\\\.\\DISPLAY1", Work: rect{0, 0, 1920, 1040}, Primary: true},
	{ID: "\\\\.\\DISPLAY2", Work: rect{1920, 0, 2560, 1400}},
}

func TestPlaceKeepsBoundsOnKnownMonitor(t *testing.T) {
	r, mon := placeOnScreen(rect{2000, 100, 800, 600}, "\\\\.\\DISPLAY2", screens)
	if r != (rect{2000, 100, 800, 600}) || mon != "\\\\.\\DISPLAY2" {
		t.Fatalf("%+v %s", r, mon)
	}
}

func TestPlaceMissingMonitorGoesToPrimary(t *testing.T) {
	r, mon := placeOnScreen(rect{6000, 100, 800, 600}, "\\\\.\\DISPLAY6", screens)
	if mon != "\\\\.\\DISPLAY1" || r.W != 800 || r.H != 600 {
		t.Fatalf("%+v %s", r, mon)
	}
	// Centred on the primary and fully visible.
	if r.X != 560 || r.Y != 220 {
		t.Fatalf("not centred: %+v", r)
	}
}

func TestPlaceShrinksToFit(t *testing.T) {
	r, _ := placeOnScreen(rect{0, 0, 4000, 3000}, "nope", screens)
	if r.W != 1920 || r.H != 1040 || r.X != 0 || r.Y != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestPlaceOffscreenOnKnownMonitorIsPulledBack(t *testing.T) {
	// Monitor exists but the window was left almost entirely off it.
	r, mon := placeOnScreen(rect{1900, 1030, 400, 300}, "\\\\.\\DISPLAY1", screens)
	if mon != "\\\\.\\DISPLAY1" || r.X+r.W > 1920 || r.Y+r.H > 1040 {
		t.Fatalf("%+v %s", r, mon)
	}
}

func TestPlaceNoScreens(t *testing.T) {
	r, mon := placeOnScreen(rect{10, 10, 300, 300}, "x", nil)
	if r != (rect{10, 10, 300, 300}) || mon != "x" {
		t.Fatalf("%+v %s", r, mon)
	}
}
