package mascot

import (
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func assertFits(t *testing.T, m *Mascot, width int) {
	t.Helper()
	lines := strings.Split(m.View(width), "\n")
	if len(lines) != Height {
		t.Fatalf("view has %d lines, want %d", len(lines), Height)
	}
	for _, line := range lines {
		if w := lipgloss.Width(line); w > width {
			t.Fatalf("line %q is %d wide in a lane of %d (state %d)", line, w, width, m.State())
		}
	}
}

func TestStaysInsideLaneInEveryState(t *testing.T) {
	for _, width := range []int{12, 40, 120} {
		for _, state := range []State{Idle, Thinking, Happy, Sad, Sleeping} {
			m := New(1)
			m.Set(state)
			for range 2000 {
				m.Tick(width)
				m.Wake() // keep Idle from napping so wandering is exercised
				assertFits(t, m, width)
			}
		}
	}
}

func TestSurvivesShrinkingLane(t *testing.T) {
	m := New(2)
	m.Set(Thinking)
	for range 300 {
		m.Tick(100)
	}
	m.Tick(14)
	assertFits(t, m, 14)
	assertFits(t, m, 3) // narrower than the sprite: draws nothing
	if strings.TrimSpace(m.View(3)) != "" {
		t.Fatal("a lane narrower than the fox should be empty")
	}
}

func TestHappyAndSadReturnToIdle(t *testing.T) {
	for _, state := range []State{Happy, Sad} {
		m := New(3)
		m.Set(state)
		for range sadTicks + 2 {
			m.Tick(40)
		}
		if m.State() != Idle {
			t.Fatalf("state %d did not return to idle", state)
		}
	}
}

func TestBannerIsTheFox(t *testing.T) {
	art, width := Banner()
	lines := strings.Split(art, "\n")
	if len(lines) != len(fox)/2 || width != len(fox[0]) {
		t.Fatalf("banner is %d rows by %d columns", len(lines), width)
	}
	for _, line := range lines {
		if lipgloss.Width(line) != width {
			t.Fatalf("banner row %q is %d wide, want %d", line, lipgloss.Width(line), width)
		}
	}
}

func TestDrawingIsWellFormed(t *testing.T) {
	for _, rows := range [][]string{fox, feet} {
		for _, row := range rows {
			if len(row) != len(fox[0]) {
				t.Fatalf("row %q is %d wide, want %d", row, len(row), len(fox[0]))
			}
			if extra := strings.Trim(row, "BWKP."); extra != "" {
				t.Fatalf("row %q uses unknown colours %q", row, extra)
			}
		}
	}
	if len(fox)%2 != 0 {
		t.Fatal("the drawing must have an even number of pixel rows")
	}
}

func TestPosesChangeOnlyTheirOwnPart(t *testing.T) {
	base := draw(pose{faceLeft: true})
	differs := func(p pose) (rows map[int]bool) {
		rows = map[int]bool{}
		other := draw(p)
		for i := range base.px {
			if base.px[i] != other.px[i] {
				rows[i/base.w] = true
			}
		}
		return rows
	}

	if rows := differs(pose{faceLeft: true, eyesClosed: true}); len(rows) != 1 || !rows[eyeRow] {
		t.Fatalf("closing the eyes changed rows %v", rows)
	}
	if rows := differs(pose{faceLeft: true, feet: 1}); len(rows) != 1 || !rows[feetRow] {
		t.Fatalf("stepping changed rows %v", rows)
	}
	for row := range differs(pose{faceLeft: true, tailDown: true}) {
		if row < tailTop || row >= tailTop+len(tailDown) {
			t.Fatalf("wagging changed row %d, outside the tail", row)
		}
	}
	// Facing right mirrors the drawing: the tail tip moves to the left edge.
	if _, ok := draw(pose{}).at(0, tailTop); !ok {
		t.Fatal("the mirrored fox should have its tail on the left")
	}
}

func TestSolidCellsAreDrawnWithoutBlockGlyphs(t *testing.T) {
	blue, white := color.NRGBA{0, 0, 255, 255}, color.NRGBA{255, 255, 255, 255}
	// Two columns, two pixel rows: the first column is one colour, the second is split.
	s := sprite{w: 2, h: 2, px: []color.NRGBA{blue, blue, blue, white}}
	if got := s.render(1, 0)[0]; got != " ▀" {
		t.Fatalf("row = %q, want a blank solid cell then a half block", got)
	}
}

func TestSleepsWhenQuietAndWakesOnActivity(t *testing.T) {
	m := New(4)
	for range sleepAfter + 2 {
		m.Tick(40)
	}
	if m.State() != Sleeping {
		t.Fatalf("state = %d, want sleeping", m.State())
	}
	m.Wake()
	if m.State() != Idle {
		t.Fatalf("state = %d, want idle after waking", m.State())
	}
}

func TestThinkingMovesEveryTick(t *testing.T) {
	m := New(5)
	m.Set(Thinking)
	seen := map[string]bool{}
	for range 10 {
		m.Tick(60)
		seen[m.View(60)] = true
	}
	if len(seen) < 10 {
		t.Fatalf("only %d distinct frames in 10 ticks", len(seen))
	}
}
