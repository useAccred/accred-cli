package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/useAccred/accred-cli/internal/api"
	"github.com/useAccred/accred-cli/internal/mascot"
)

func (m Model) bannerText() string {
	model := m.cfg.Model
	if model == "" {
		model = "no model selected"
	}
	title := accentStyle.Render(banner) + mutedStyle.Render("  v"+m.version)
	fox, foxWidth := mascot.Banner()
	// The fox stands beside the title when the screen has room for both.
	if m.cfg.NoMascot || m.width < foxWidth+2+lipgloss.Width(title) {
		return title + "\n" +
			mutedStyle.Render(runewidth.Truncate(" Tokenized LLM credits in your terminal · "+model+" · /help for commands", max(1, m.width-1), "…")) + "\n"
	}
	text := title + "\n\n" +
		mutedStyle.Render(" Tokenized LLM credits in your terminal") + "\n" +
		mutedStyle.Render(runewidth.Truncate(" "+model+" · /help for commands", max(1, m.width-foxWidth-3), "…"))
	return lipgloss.JoinHorizontal(lipgloss.Center, fox, "  ", text) + "\n"
}

// showMascot reports whether the roaming fox is drawn; short screens keep the room for chat.
func (m Model) showMascot() bool {
	return !m.cfg.NoMascot && m.height >= 20
}

func (m Model) userText(text string) string {
	body := lipgloss.NewStyle().Width(max(10, m.width-2)).Render(text)
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if i == 0 {
			lines[i] = accentStyle.Render("› ") + boldStyle.Render(line)
		} else {
			lines[i] = "  " + boldStyle.Render(line)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m *Model) renderMarkdown(text string) string {
	width := max(20, m.width-2)
	if m.markdown == nil || m.markdownFor != width {
		style := "light"
		if m.dark {
			style = "dark"
		}
		renderer, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(width))
		if err != nil {
			return text
		}
		m.markdown, m.markdownFor = renderer, width
	}
	out, err := m.markdown.Render(text)
	if err != nil {
		return text
	}
	return strings.Trim(out, "\n")
}

func (m Model) assistantText(reply *api.Reply) string {
	meta := fmt.Sprintf("  %s · %d in / %d out · %s credits",
		reply.Model, reply.InputTokens, reply.OutputTokens, trimDecimal(reply.CreditsCharged, 6))
	if reply.FinishReason == "length" {
		meta += " · cut off at the token limit, raise it with /max"
	}
	return m.renderMarkdown(reply.Content) + "\n" + mutedStyle.Render(meta) + "\n"
}

// revealView shows the tail of the reply being typed out, so the live area never outgrows the screen.
func (m Model) revealView() string {
	text := string(m.reveal.runes[:min(m.reveal.pos, len(m.reveal.runes))])
	body := lipgloss.NewStyle().Width(max(10, m.width-4)).Render(text)
	lines := strings.Split(body, "\n")
	if room := max(3, m.height-mascot.Height-8); len(lines) > room {
		lines = lines[len(lines)-room:]
	}
	for i := range lines {
		lines[i] = "  " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func (m Model) statusLine() string {
	if m.notice != "" {
		return accentStyle.Render(" " + runewidth.Truncate(m.notice, max(1, m.width-2), "…"))
	}
	model := m.cfg.Model
	if model == "" {
		model = "no model · /model"
	}
	parts := []string{model}
	if m.remaining != "" {
		parts = append(parts, trimDecimal(m.remaining, 4)+" credits")
	}
	if m.replies > 0 {
		parts = append(parts, "session "+trimDecimal(m.spent.FloatString(8), 6))
	}
	parts = append(parts, "/help")
	// Drop detail from the right until the line fits.
	for len(parts) > 1 && runewidth.StringWidth(" "+strings.Join(parts, " · ")) > m.width {
		parts = parts[:len(parts)-1]
	}
	return mutedStyle.Render(" " + runewidth.Truncate(strings.Join(parts, " · "), max(1, m.width-1), "…"))
}

func (m Model) hints() string {
	value := m.input.Value()
	if !strings.HasPrefix(value, "/") || strings.Contains(value, " ") {
		return ""
	}
	var lines []string
	for _, c := range matchingCommands(value) {
		usage := strings.TrimSpace(c.name + " " + c.args)
		line := fmt.Sprintf("  %-16s %s", usage, c.help)
		lines = append(lines, mutedStyle.Render(runewidth.Truncate(line, max(1, m.width-1), "…")))
	}
	return strings.Join(lines, "\n")
}

func (m Model) keyEntryView() string {
	lines := []string{
		boldStyle.Render("Add your API key"),
		mutedStyle.Render("Create a key on the API page at ") + accentStyle.Render("https://accred.sh") + mutedStyle.Render(", then paste it here."),
		mutedStyle.Render("It is stored in your system keychain, never in a file."),
		m.keyInput.View(),
	}
	switch {
	case m.keyChecking:
		lines = append(lines, mutedStyle.Render("  checking the key…"))
	case m.keyErr != "":
		lines = append(lines, errorStyle.Render("  "+m.keyErr))
	case m.hasKey:
		lines = append(lines, mutedStyle.Render("  enter save · esc cancel"))
	default:
		lines = append(lines, mutedStyle.Render("  enter save · ctrl+c exit"))
	}
	for i, line := range lines {
		lines[i] = lipgloss.NewStyle().MaxWidth(m.width).Render(line)
	}
	return strings.Join(lines, "\n")
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	var sections []string

	switch {
	case m.reveal != nil:
		sections = append(sections, m.revealView())
	case m.busy:
		elapsed := int(time.Since(m.busySince).Seconds())
		sections = append(sections, mutedStyle.Render(fmt.Sprintf("  thinking… %ds · esc to cancel", elapsed)))
	}

	if m.showMascot() {
		sections = append(sections, m.mascot.View(m.width))
	}

	if m.keyOpen {
		sections = append(sections, m.keyEntryView())
	} else if m.picker != nil {
		sections = append(sections, m.picker.view(m.width))
	} else {
		sections = append(sections, inputBorder.Width(max(10, m.width-2)).Render(m.input.View()))
		if hints := m.hints(); hints != "" {
			sections = append(sections, hints)
		}
	}

	sections = append(sections, m.statusLine())
	return strings.Join(sections, "\n")
}
