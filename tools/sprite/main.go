// Command sprite turns the mascot artwork into the small transparent image the
// terminal banner draws with half-block characters.
//
//	go run ./tools/sprite <source.png> <height-px> <out.png> [preview.png]
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strconv"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: sprite <source.png> <height-px> <out.png> [preview.png]")
		os.Exit(2)
	}
	height, err := strconv.Atoi(os.Args[2])
	check(err)
	src := load(os.Args[1])
	background := cutOutBackground(src)
	palette, index := flatten(src, background)
	out := shrink(src, background, palette, index, height)
	save(os.Args[3], out)
	if len(os.Args) > 4 {
		save(os.Args[4], preview(out, 16))
	}
	fmt.Printf("%dx%d\n", out.Bounds().Dx(), out.Bounds().Dy())
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func load(path string) *image.NRGBA {
	f, err := os.Open(path)
	check(err)
	defer f.Close()
	decoded, err := png.Decode(f)
	check(err)
	b := decoded.Bounds()
	img := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := range b.Dy() {
		for x := range b.Dx() {
			img.Set(x, y, decoded.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return img
}

func save(path string, img image.Image) {
	f, err := os.Create(path)
	check(err)
	defer f.Close()
	check(png.Encode(f, img))
}

// cutOutBackground marks the white backdrop: near-white or transparent pixels
// connected to the image border. White fur inside the outline is kept.
func cutOutBackground(img *image.NRGBA) []bool {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	backdrop := func(x, y int) bool {
		c := img.NRGBAAt(x, y)
		return c.A < 16 || (c.R > 238 && c.G > 238 && c.B > 238)
	}
	background := make([]bool, w*h)
	var stack []int
	push := func(x, y int) {
		if x < 0 || y < 0 || x >= w || y >= h || background[y*w+x] || !backdrop(x, y) {
			return
		}
		background[y*w+x] = true
		stack = append(stack, y*w+x)
	}
	for x := range w {
		push(x, 0)
		push(x, h-1)
	}
	for y := range h {
		push(0, y)
		push(w-1, y)
	}
	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := i%w, i/w
		push(x+1, y)
		push(x-1, y)
		push(x, y+1)
		push(x, y-1)
	}
	return background
}

const paletteSize = 9

// flatten reduces the artwork to a small palette (k-means over the character's
// pixels) and returns the palette and each pixel's index in it. Flat colours keep
// the sprite crisp: neighbouring cells end up exactly equal instead of nearly equal.
func flatten(img *image.NRGBA, background []bool) (palette [][3]float64, index []int) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	var pixels [][3]float64
	var where []int
	for i := range w * h {
		if background[i] {
			continue
		}
		c := img.NRGBAAt(i%w, i/w)
		pixels = append(pixels, [3]float64{float64(c.R), float64(c.G), float64(c.B)})
		where = append(where, i)
	}
	// Start from evenly spaced samples so the result is the same on every run.
	palette = make([][3]float64, paletteSize)
	for k := range palette {
		palette[k] = pixels[(2*k+1)*len(pixels)/(2*paletteSize)]
	}
	// Small features would otherwise be absorbed by the fur colours, so the pink of
	// the cheeks and the dark of the eyes each get a palette slot of their own.
	pinkest, darkest := pixels[0], pixels[0]
	for _, p := range pixels {
		if p[0]-p[2] > pinkest[0]-pinkest[2] {
			pinkest = p
		}
		if p[0]+p[1]+p[2] < darkest[0]+darkest[1]+darkest[2] {
			darkest = p
		}
	}
	palette[0], palette[1] = pinkest, darkest
	assigned := make([]int, len(pixels))
	for range 24 {
		var sum [paletteSize][3]float64
		var count [paletteSize]int
		for i, p := range pixels {
			best, bestDist := 0, 1e18
			for k, c := range palette {
				d := (p[0]-c[0])*(p[0]-c[0]) + (p[1]-c[1])*(p[1]-c[1]) + (p[2]-c[2])*(p[2]-c[2])
				if d < bestDist {
					best, bestDist = k, d
				}
			}
			assigned[i] = best
			count[best]++
			for ch := range 3 {
				sum[best][ch] += p[ch]
			}
		}
		for k := range palette {
			if count[k] > 0 {
				palette[k] = [3]float64{sum[k][0] / float64(count[k]), sum[k][1] / float64(count[k]), sum[k][2] / float64(count[k])}
			}
		}
	}
	index = make([]int, w*h)
	for i, at := range where {
		index[at] = assigned[i]
	}
	return palette, index
}

// shrink crops to the character and scales it to the target height. Each output
// pixel takes the most common palette colour in the area it covers, which keeps
// outlines and eyes sharp where averaging would blur them into grey.
func shrink(img *image.NRGBA, background []bool, palette [][3]float64, index []int, height int) *image.NRGBA {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	minX, minY, maxX, maxY := w, h, 0, 0
	for y := range h {
		for x := range w {
			if !background[y*w+x] {
				minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
			}
		}
	}
	cropW, cropH := maxX-minX+1, maxY-minY+1
	width := max(1, (cropW*height+cropH/2)/cropH)
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	for oy := range height {
		for ox := range width {
			x0, x1 := minX+ox*cropW/width, minX+(ox+1)*cropW/width
			y0, y1 := minY+oy*cropH/height, minY+(oy+1)*cropH/height
			var votes [paletteSize]int
			solid, total := 0, 0
			for y := y0; y < max(y1, y0+1); y++ {
				for x := x0; x < max(x1, x0+1); x++ {
					total++
					if !background[y*w+x] {
						votes[index[y*w+x]]++
						solid++
					}
				}
			}
			// A cell is drawn only when the character covers at least half of it.
			if solid*2 < total || solid == 0 {
				continue
			}
			best := 0
			for k := range votes {
				if votes[k] > votes[best] {
					best = k
				}
			}
			// The cheeks and eyes are small; they win a cell they cover a third of.
			for _, feature := range []int{0, 1} {
				if votes[feature]*3 >= solid {
					best = feature
				}
			}
			c := palette[best]
			out.SetNRGBA(ox, oy, color.NRGBA{uint8(c[0] + 0.5), uint8(c[1] + 0.5), uint8(c[2] + 0.5), 255})
		}
	}
	return out
}

// preview enlarges the sprite on a dark backdrop so it can be checked by eye.
func preview(img *image.NRGBA, scale int) *image.NRGBA {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w*scale, h*scale))
	for y := range h * scale {
		for x := range w * scale {
			c := img.NRGBAAt(x/scale, y/scale)
			if c.A == 0 {
				c = color.NRGBA{24, 24, 27, 255}
			}
			out.SetNRGBA(x, y, c)
		}
	}
	return out
}
