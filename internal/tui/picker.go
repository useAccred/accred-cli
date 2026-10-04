package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
	"github.com/useAccred/accred-cli/internal/api"
)

const pickerRows = 8

// picker is the filterable model list.
type picker struct {
	models  []api.Model
	filter  string
	matches []int // indexes into models
	cursor  int
	offset  int
}

func newPicker(models []api.Model, filter string) *picker {
	p := &picker{models: models, filter: filter}
	p.refilter()
	return p
}

func (p *picker) refilter() {
	terms := strings.Fields(strings.ToLower(p.filter))
	p.matches = p.matches[:0]
	for i, m := range p.models {
		haystack := strings.ToLower(m.ID + " " + m.Name + " " + m.Provider)
		ok := true
		for _, term := range terms {
			if !strings.Contains(haystack, term) {
				ok = false
				break
			}
		}
		if ok {
			p.matches = append(p.matches, i)
		}
	}
	p.cursor, p.offset = 0, 0
}

func (p *picker) move(delta int) {
	if len(p.matches) == 0 {
		return
	}
	p.cursor = max(0, min(p.cursor+delta, len(p.matches)-1))
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+pickerRows {
		p.offset = p.cursor - pickerRows + 1
	}
}

func (p *picker) selected() (api.Model, bool) {
	if len(p.matches) == 0 {
		return api.Model{}, false
	}
	return p.models[p.matches[p.cursor]], true
}

// update handles a key and reports whether the picker should close, and with which model.
func (p *picker) update(msg tea.KeyMsg) (done bool, choice *api.Model) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		return true, nil
	case tea.KeyEnter:
		if m, ok := p.selected(); ok {
			return true, &m
		}
	case tea.KeyUp:
		p.move(-1)
	case tea.KeyDown:
		p.move(1)
	case tea.KeyPgUp:
		p.move(-pickerRows)
	case tea.KeyPgDown:
		p.move(pickerRows)
	case tea.KeyBackspace:
		if r := []rune(p.filter); len(r) > 0 {
			p.filter = string(r[:len(r)-1])
			p.refilter()
		}
	case tea.KeyRunes, tea.KeySpace:
		p.filter += string(msg.Runes)
		p.refilter()
	}
	return false, nil
}

func price(m api.Model) string {
	if m.InputCost == nil || m.OutputCost == nil {
		return ""
	}
	return fmt.Sprintf("$%s in / $%s out per M", *m.InputCost, *m.OutputCost)
}

func (p *picker) view(width int) string {
	lines := []string{
		boldStyle.Render("Select a model") + mutedStyle.Render("  type to filter · ↑↓ move · enter select · esc cancel"),
		accentStyle.Render("› ") + p.filter + accentStyle.Render("▏"),
	}
	if len(p.matches) == 0 {
		lines = append(lines, mutedStyle.Render("  no models match"))
	}
	for row := p.offset; row < min(p.offset+pickerRows, len(p.matches)); row++ {
		m := p.models[p.matches[row]]
		detail := price(m)
		idWidth := max(10, width-runewidth.StringWidth(detail)-6)
		id := runewidth.Truncate(m.ID, idWidth, "…")
		gap := strings.Repeat(" ", max(2, width-2-runewidth.StringWidth(id)-runewidth.StringWidth(detail)-1))
		if row == p.cursor {
			lines = append(lines, accentStyle.Render("❯ "+id)+gap+mutedStyle.Render(detail))
		} else {
			lines = append(lines, "  "+id+gap+mutedStyle.Render(detail))
		}
	}
	if len(p.matches) > 0 {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("  %d of %d", p.cursor+1, len(p.matches))))
	}
	return strings.Join(lines, "\n")
}
