package main

import (
	"encoding/binary"
	"testing"
)

func TestChimeWAV(t *testing.T) {
	b := chimeWAV()
	if string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" || string(b[36:40]) != "data" {
		t.Fatalf("bad header % x", b[:44])
	}
	if got := binary.LittleEndian.Uint32(b[4:8]); int(got) != len(b)-8 {
		t.Fatalf("riff size %d, file %d", got, len(b))
	}
	n := binary.LittleEndian.Uint32(b[40:44])
	if int(n) != len(b)-44 {
		t.Fatalf("data size %d, file %d", n, len(b))
	}
	// Long enough to notice (over a second) and not silent.
	if secs := float64(n) / 2 / chimeRate; secs < 1 || secs > 4 {
		t.Fatalf("chime lasts %.2fs", secs)
	}
	var peak int16
	for i := 44; i+1 < len(b); i += 2 {
		peak = max(peak, int16(binary.LittleEndian.Uint16(b[i:])))
	}
	if peak < 8000 {
		t.Fatalf("too quiet: peak %d", peak)
	}
}
