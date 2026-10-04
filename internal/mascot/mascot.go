// Package mascot draws and animates the Accred fox.
//
// The artwork is pixel art drawn with half-block characters, so each terminal
// row carries two rows of pixels.
package mascot

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/color"
	"image/png"
	"math/rand"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The sprites are generated from assets/mascot.png by tools/sprite.
var (
	//go:embed fox16.png
	roamPNG []byte
	//go:embed fox28.png
	bannerPNG []byte
)

type State int

const (
	Idle State = iota
	Thinking
	Happy
	Sad
	Sleeping
)

const (
	// Height is the number of terminal rows the roaming fox occupies: the
	// sprite plus one row of headroom for hopping.
	Height = 9

	happyTicks = 18
	sadTicks   = 28
	// Ticks without user activity before the fox falls asleep (about a minute).
	sleepAfter = 500
)

var (
	hop          = []int{0, 1, 2, 2, 1, 0}
	overlayStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1D4ED8", Dark: "#60A5FA"})
)

type sprite struct {
	w, h int
	px   []color.NRGBA // alpha 0 is transparent
}

func decode(raw []byte) sprite {
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		panic("mascot: embedded sprite is unreadable: " + err.Error())
	}
	b := decoded.Bounds()
	s := sprite{w: b.Dx(), h: b.Dy(), px: make([]color.NRGBA, b.Dx()*b.Dy())}
	for y := range s.h {
		for x := range s.w {
			s.px[y*s.w+x] = color.NRGBAModel.Convert(decoded.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
		}
	}
	return s
}

func (s sprite) at(x, y int) (color.NRGBA, bool) {
	if x < 0 || y < 0 || x >= s.w || y >= s.h {
		return color.NRGBA{}, false
	}
	c := s.px[y*s.w+x]
	return c, c.A != 0
}

func (s sprite) flipped() sprite {
	out := sprite{w: s.w, h: s.h, px: make([]color.NRGBA, len(s.px))}
	for y := range s.h {
		for x := range s.w {
			out.px[y*s.w+x] = s.px[y*s.w+(s.w-1-x)]
		}
	}
	return out
}

func dark(c color.NRGBA) bool {
	return c.A != 0 && (299*int(c.R)+587*int(c.G)+114*int(c.B))/1000 < 150
}

// eyesClosed lowers the eyelids: the upper part of each eye takes the fur colour above it.
func (s sprite) eyesClosed() sprite {
	out := sprite{w: s.w, h: s.h, px: append([]color.NRGBA(nil), s.px...)}
	for y := 1; y < s.h/2; y++ {
		for x := range s.w {
			here, _ := s.at(x, y)
			below, _ := s.at(x, y+1)
			if dark(here) && dark(below) {
				out.px[y*s.w+x] = s.px[(y-1)*s.w+x]
			}
		}
	}
	return out
}

func hex(c color.NRGBA) lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B))
}

// render draws the sprite into rows of half-blocks, raised lift pixels off the floor.
func (s sprite) render(rows, lift int) []string {
	top := rows*2 - s.h - lift
	lines := make([]string, rows)
	for row := range rows {
		var line strings.Builder
		for x := range s.w {
			upper, hasUpper := s.at(x, row*2-top)
			lower, hasLower := s.at(x, row*2+1-top)
			switch {
			case hasUpper && hasLower && upper == lower:
				// A solid cell is painted as background, which leaves no seam between rows
				// in terminals that draw block characters slightly short of the cell edge.
				line.WriteString(lipgloss.NewStyle().Background(hex(upper)).Render(" "))
			case hasUpper && hasLower:
				line.WriteString(lipgloss.NewStyle().Foreground(hex(upper)).Background(hex(lower)).Render("▀"))
			case hasUpper:
				line.WriteString(lipgloss.NewStyle().Foreground(hex(upper)).Render("▀"))
			case hasLower:
				line.WriteString(lipgloss.NewStyle().Foreground(hex(lower)).Render("▄"))
			default:
				line.WriteByte(' ')
			}
		}
		lines[row] = line.String()
	}
	return lines
}

// Banner returns the large fox for the welcome screen, and its width in columns.
func Banner() (art string, width int) {
	s := decode(bannerPNG)
	return strings.Join(s.render((s.h+1)/2, 0), "\n"), s.w
}

type pose struct {
	faceLeft, eyesClosed bool
	lift                 int
}

type Mascot struct {
	x, dir int
	state  State
	tick   int // ticks since the state began
	quiet  int // ticks since the user last did something
	pause  int // ticks left standing still
	blink  int // ticks left with eyes closed
	rng    *rand.Rand

	art    sprite
	frames map[pose][]string
}

func New(seed int64) *Mascot {
	return &Mascot{dir: 1, rng: rand.New(rand.NewSource(seed)), art: decode(roamPNG), frames: map[pose][]string{}}
}

func (m *Mascot) State() State { return m.state }

// Set switches the animation. Happy and Sad return to Idle on their own.
func (m *Mascot) Set(s State) {
	if m.state != s {
		m.state, m.tick = s, 0
	}
}

// Wake records user activity and ends a nap.
func (m *Mascot) Wake() {
	m.quiet = 0
	if m.state == Sleeping {
		m.Set(Idle)
	}
}

// Tick advances the animation by one frame within a lane of the given width.
func (m *Mascot) Tick(width int) {
	m.tick++
	m.quiet++
	switch m.state {
	case Happy:
		if m.tick > happyTicks {
			m.Set(Idle)
		}
	case Sad:
		if m.tick > sadTicks {
			m.Set(Idle)
		}
	case Thinking:
		m.step(width)
	case Idle:
		if m.quiet > sleepAfter {
			m.Set(Sleeping)
			return
		}
		m.wander(width)
	}
	m.x = max(0, min(m.x, width-m.art.w))
}

func (m *Mascot) wander(width int) {
	if m.blink > 0 {
		m.blink--
	} else if m.rng.Intn(40) == 0 {
		m.blink = 2
	}
	if m.pause > 0 {
		m.pause--
		return
	}
	switch {
	case m.rng.Intn(60) == 0:
		m.pause = 8 + m.rng.Intn(20)
	case m.rng.Intn(90) == 0:
		m.dir = -m.dir
	case m.tick%2 == 0:
		m.step(width)
	}
}

func (m *Mascot) step(width int) {
	if m.x+m.dir < 0 || m.x+m.dir > width-m.art.w {
		m.dir = -m.dir
	}
	m.x += m.dir
}

func (m *Mascot) pose() pose {
	// The artwork's tail is on its right, so it trails when the fox walks left.
	p := pose{faceLeft: m.dir < 0}
	switch m.state {
	case Thinking:
		p.lift = m.tick % 2
	case Happy:
		p.lift = hop[m.tick%len(hop)]
	case Sad, Sleeping:
		p.eyesClosed = true
	case Idle:
		p.eyesClosed = m.blink > 0
		if m.pause == 0 {
			p.lift = (m.tick / 2) % 2
		}
	}
	return p
}

// overlay is the small mark drawn beside the fox's head.
func (m *Mascot) overlay() string {
	switch m.state {
	case Thinking:
		return strings.Repeat("·", 1+(m.tick/3)%3)
	case Happy:
		return []string{"✦", "✧"}[(m.tick/3)%2]
	case Sad:
		return "…"
	case Sleeping:
		return []string{"z", "zZ", "zZz"}[(m.tick/6)%3]
	}
	return ""
}

func (m *Mascot) frame(p pose) []string {
	if lines, ok := m.frames[p]; ok {
		return lines
	}
	art := m.art
	if p.eyesClosed {
		art = art.eyesClosed()
	}
	if !p.faceLeft {
		art = art.flipped()
	}
	lines := art.render(Height, p.lift)
	m.frames[p] = lines
	return lines
}

// View returns Height lines, each no wider than width.
func (m *Mascot) View(width int) string {
	if m.art.w > width {
		return strings.Repeat("\n", Height-1)
	}
	x := max(0, min(m.x, width-m.art.w))
	lines := append([]string(nil), m.frame(m.pose())...)
	mark := m.overlay()
	markWidth := lipgloss.Width(mark)
	for i := range lines {
		pad := strings.Repeat(" ", x)
		if i == 1 && mark != "" {
			switch {
			case x+m.art.w+1+markWidth <= width:
				lines[i] = pad + lines[i] + " " + overlayStyle.Render(mark)
				continue
			case x >= markWidth+1:
				lines[i] = strings.Repeat(" ", x-markWidth-1) + overlayStyle.Render(mark) + " " + lines[i]
				continue
			}
		}
		lines[i] = pad + lines[i]
	}
	return strings.Join(lines, "\n")
}
