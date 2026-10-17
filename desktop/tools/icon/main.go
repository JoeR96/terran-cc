// Command icon draws the Terran CC tray icon: an accent ring with a dot,
// like a timer face. Run from desktop/: go run ./tools/icon
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func main() {
	const n = 64
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	accent := color.NRGBA{0xf0, 0x8a, 0x5d, 0xff}
	c := float64(n-1) / 2
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			d := math.Hypot(float64(x)-c, float64(y)-c)
			a := 0.0
			switch {
			case d >= 22 && d <= 30: // ring
				a = math.Min(1, math.Min(d-21, 31-d))
			case d <= 9: // centre dot
				a = math.Min(1, 10-d)
			}
			// a hand from the centre up to the ring
			if math.Abs(float64(x)-c) <= 2.5 && float64(y) < c && float64(y) > 10 {
				a = 1
			}
			if a > 0 {
				col := accent
				col.A = uint8(255 * a)
				img.SetNRGBA(x, y, col)
			}
		}
	}
	f, _ := os.Create("tray.png")
	defer f.Close()
	png.Encode(f, img)
}
