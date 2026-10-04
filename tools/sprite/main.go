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
	out := shrink(src, cutOutBackground(src), height)
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

// shrink crops to the character and scales it to the target height by averaging.
func shrink(img *image.NRGBA, background []bool, height int) *image.NRGBA {
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
			var r, g, b, solid, total int
			for y := y0; y < max(y1, y0+1); y++ {
				for x := x0; x < max(x1, x0+1); x++ {
					total++
					if background[y*w+x] {
						continue
					}
					c := img.NRGBAAt(x, y)
					r, g, b, solid = r+int(c.R), g+int(c.G), b+int(c.B), solid+1
				}
			}
			// A cell is drawn only when the character covers at least half of it.
			if solid*2 >= total && solid > 0 {
				out.SetNRGBA(ox, oy, color.NRGBA{uint8(r / solid), uint8(g / solid), uint8(b / solid), 255})
			}
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
