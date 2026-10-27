// Command icon draws the Terran CC icon, an original sci-fi command centre
// on landing legs, and writes it where the app uses it:
//
//   - appicon.png: the window, taskbar and tray icon, embedded in the binary
//   - rsrc_windows_amd64.syso: the exe icon, linked in by go build
//
// Run from desktop/: go run ./tools/icon
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"

	"github.com/tc-hib/winres"
)

// The drawing is made on a 256 grid at 4x and scaled down, so edges are
// smooth at every size.
const (
	grid  = 256
	scale = 4
	big   = grid * scale
)

var (
	outline = color.NRGBA{0x0c, 0x0c, 0x0c, 0xff}
	steel   = color.NRGBA{0x8c, 0x96, 0xa8, 0xff}
	light   = color.NRGBA{0xbf, 0xc8, 0xd6, 0xff}
	dark    = color.NRGBA{0x55, 0x5e, 0x70, 0xff}
	orange  = color.NRGBA{0xfb, 0x7e, 0x14, 0xff} // Campbell accent, as in the app
	glow    = color.NRGBA{0xff, 0xc0, 0x6a, 0xff}
)

type pt struct{ x, y float64 }

type shape interface{ in(x, y float64) bool }

type poly []pt

func (p poly) in(x, y float64) bool {
	inside := false
	for i, j := 0, len(p)-1; i < len(p); j, i = i, i+1 {
		a, b := p[i], p[j]
		if (a.y > y) != (b.y > y) && x < (b.x-a.x)*(y-a.y)/(b.y-a.y)+a.x {
			inside = !inside
		}
	}
	return inside
}

type circle struct{ cx, cy, r float64 }

func (c circle) in(x, y float64) bool { return math.Hypot(x-c.cx, y-c.cy) <= c.r }

// half is the top half of a circle: a dome.
type half struct{ circle }

func (h half) in(x, y float64) bool { return y <= h.cy && h.circle.in(x, y) }

func rect(x0, y0, x1, y1 float64) poly { return poly{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}} }

func mirror(p poly) poly {
	out := make(poly, len(p))
	for i, q := range p {
		out[len(p)-1-i] = pt{grid - q.x, q.y}
	}
	return out
}

type layer struct {
	s   shape
	col color.NRGBA
}

// command centre, back to front.
func layers() []layer {
	legL := poly{{44, 196}, {74, 196}, {64, 238}, {30, 238}}
	podL := poly{{12, 132}, {40, 120}, {52, 124}, {52, 184}, {40, 190}, {12, 180}}
	base := poly{{30, 150}, {62, 118}, {194, 118}, {226, 150}, {226, 192}, {196, 214}, {60, 214}, {30, 192}}
	tower := poly{{86, 74}, {170, 74}, {184, 124}, {72, 124}}
	return []layer{
		{legL, dark}, {mirror(legL), dark},
		{rect(40, 230, 76, 242), steel}, {mirror(rect(40, 230, 76, 242)), steel}, // feet
		{podL, dark}, {mirror(podL), dark},
		{base, steel},
		{poly{{30, 150}, {62, 118}, {194, 118}, {226, 150}, {196, 150}, {60, 150}}, light}, // lit top deck
		{tower, steel},
		{poly{{86, 74}, {170, 74}, {176, 96}, {80, 96}}, light},
		{half{circle{128, 76, 38}}, light},
		{rect(125, 18, 131, 40), dark}, // antenna
		{circle{128, 16, 8}, orange},
		{rect(52, 164, 204, 182), outline}, // window band
		{rect(58, 168, 88, 178), orange}, {rect(94, 168, 124, 178), orange},
		{rect(132, 168, 162, 178), orange}, {rect(168, 168, 198, 178), orange},
		{rect(108, 100, 148, 112), orange},                      // bridge window
		{circle{26, 156, 7}, glow}, {circle{230, 156, 7}, glow}, // pod lights
	}
}

func render() *image.NRGBA {
	ls := layers()
	mask := make([]bool, big*big)
	col := make([]color.NRGBA, big*big)
	for y := 0; y < big; y++ {
		for x := 0; x < big; x++ {
			gx, gy := (float64(x)+.5)/scale, (float64(y)+.5)/scale
			for _, l := range ls {
				if l.s.in(gx, gy) {
					mask[y*big+x] = true
					col[y*big+x] = l.col
				}
			}
		}
	}
	// A dark outline around the whole silhouette: pixels near the edge.
	const w = 5 * scale
	edge := make([]bool, big*big)
	for y := 0; y < big; y++ {
		for x := 0; x < big; x++ {
			if !mask[y*big+x] {
				continue
			}
			for d := 1; d <= w && !edge[y*big+x]; d++ {
				for _, o := range [][2]int{{d, 0}, {-d, 0}, {0, d}, {0, -d}} {
					nx, ny := x+o[0], y+o[1]
					if nx < 0 || ny < 0 || nx >= big || ny >= big || !mask[ny*big+nx] {
						edge[y*big+x] = true
						break
					}
				}
			}
		}
	}
	img := image.NewNRGBA(image.Rect(0, 0, grid, grid))
	for y := 0; y < grid; y++ {
		for x := 0; x < grid; x++ {
			var r, g, b, a float64
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					i := (y*scale+sy)*big + x*scale + sx
					if !mask[i] {
						continue
					}
					c := col[i]
					if edge[i] {
						c = outline
					}
					r, g, b, a = r+float64(c.R), g+float64(c.G), b+float64(c.B), a+1
				}
			}
			if a > 0 {
				img.SetNRGBA(x, y, color.NRGBA{uint8(r / a), uint8(g / a), uint8(b / a), uint8(255 * a / (scale * scale))})
			}
		}
	}
	return img
}

func main() {
	img := render()
	f, err := os.Create("appicon.png")
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	f.Close()

	icon, err := winres.NewIconFromResizedImage(img, []int{256, 128, 64, 48, 32, 24, 16})
	if err != nil {
		log.Fatal(err)
	}
	var rs winres.ResourceSet
	// Wails loads the window icon as IDI_APPLICATION (32512); Explorer shows
	// the exe's first icon group, which is this one.
	if err := rs.SetIcon(winres.ID(32512), icon); err != nil {
		log.Fatal(err)
	}
	out, err := os.Create("rsrc_windows_amd64.syso")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	if err := rs.WriteObject(out, winres.ArchAMD64); err != nil {
		log.Fatal(err)
	}
}
