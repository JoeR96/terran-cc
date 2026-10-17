package main

import (
	"bytes"
	"encoding/binary"
	"math"
)

const chimeRate = 22050

// chimeWAV builds a short rising three-note chime, played twice, as a 16-bit
// mono WAV file in memory.
func chimeWAV() []byte {
	var pcm []int16
	for range 2 {
		for _, f := range []float64{784, 988, 1319} {
			n := chimeRate * 30 / 100
			for i := range n {
				t := float64(i) / chimeRate
				env := math.Min(1, t/0.01) * math.Exp(-t*6)
				pcm = append(pcm, int16(0.5*math.MaxInt16*env*math.Sin(2*math.Pi*f*t)))
			}
		}
		pcm = append(pcm, make([]int16, chimeRate/4)...)
	}
	var b bytes.Buffer
	size := uint32(len(pcm) * 2)
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, 36+size)
	b.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(chimeRate), uint32(chimeRate * 2), uint16(2), uint16(16)} {
		binary.Write(&b, binary.LittleEndian, v)
	}
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, size)
	binary.Write(&b, binary.LittleEndian, pcm)
	return b.Bytes()
}
