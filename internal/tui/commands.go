package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type command struct {
	name string
	args string
	help string
}

var commands = []command{
	{"/model", "[name]", "choose a model, or filter the list by name"},
	{"/max", "<tokens>", "set the most tokens a reply may use (1 to 8192)"},
	{"/system", "[text]", "set a system prompt; no text clears it"},
	{"/cost", "", "credits spent in this session"},
	{"/clear", "", "start a new conversation"},
	{"/mascot", "on|off", "show or hide the mascot"},
	{"/login", "", "add or replace your API key"},
	{"/logout", "", "forget the saved API key"},
	{"/key", "", "where the API key comes from"},
	{"/help", "", "list commands"},
	{"/quit", "", "exit"},
}

// matchingCommands returns the commands whose name starts with the typed prefix.
func matchingCommands(typed string) []command {
	name, _, _ := strings.Cut(typed, " ")
	var out []command
	for _, c := range commands {
		if strings.HasPrefix(c.name, name) {
			out = append(out, c)
		}
	}
	return out
}

func helpText() string {
	var b strings.Builder
	b.WriteString(boldStyle.Render("Commands") + "\n")
	for _, c := range commands {
		usage := strings.TrimSpace(c.name + " " + c.args)
		fmt.Fprintf(&b, "  %s %s\n", accentStyle.Render(fmt.Sprintf("%-16s", usage)), mutedStyle.Render(c.help))
	}
	b.WriteString(boldStyle.Render("Keys") + "\n")
	for _, k := range [][2]string{
		{"enter", "send"},
		{"alt+enter, ctrl+j", "new line"},
		{"↑ ↓", "previous and next messages you sent"},
		{"tab", "complete a command"},
		{"esc", "cancel the request in progress"},
		{"ctrl+c twice", "exit"},
	} {
		fmt.Fprintf(&b, "  %s %s\n", accentStyle.Render(fmt.Sprintf("%-18s", k[0])), mutedStyle.Render(k[1]))
	}
	return b.String()
}

func (m Model) runCommand(line string) (tea.Model, tea.Cmd) {
	name, arg, _ := strings.Cut(line, " ")
	arg = strings.TrimSpace(arg)
	say := func(text string) (tea.Model, tea.Cmd) { return m, tea.Println(text + "\n") }
	fail := func(text string) (tea.Model, tea.Cmd) { return m, tea.Println(errorStyle.Render(text) + "\n") }

	switch name {
	case "/help":
		return m, tea.Println(helpText())

	case "/quit", "/exit":
		m.quitting = true
		return m, tea.Quit

	case "/model":
		if m.models == nil {
			return fail("The model list has not loaded yet. Try again in a moment.")
		}
		for _, candidate := range m.models {
			if candidate.ID == arg {
				return m.selectModel(candidate)
			}
		}
		m.picker = newPicker(m.models, arg)
		return m, nil

	case "/max":
		n, err := strconv.Atoi(arg)
		if err != nil || n < 1 || n > 8192 {
			return fail("Usage: /max <tokens>, from 1 to 8192.")
		}
		m.cfg.MaxTokens = n
		m.persist()
		return say(mutedStyle.Render(fmt.Sprintf("Replies may now use up to %d tokens.", n)))

	case "/system":
		m.system = arg
		if arg == "" {
			return say(mutedStyle.Render("System prompt cleared."))
		}
		return say(mutedStyle.Render("System prompt set."))

	case "/cost":
		noun := "replies"
		if m.replies == 1 {
			noun = "reply"
		}
		text := fmt.Sprintf("This session: %s credits over %d %s.", trimDecimal(m.spent.FloatString(8), 6), m.replies, noun)
		if m.remaining != "" {
			text += fmt.Sprintf(" Available: %s credits.", trimDecimal(m.remaining, 4))
		}
		return say(mutedStyle.Render(text))

	case "/clear":
		m.history = nil
		return m, tea.Sequence(tea.ClearScreen, tea.Println(m.bannerText()))

	case "/mascot":
		switch arg {
		case "on":
			m.cfg.NoMascot = false
		case "off":
			m.cfg.NoMascot = true
		default:
			return fail("Usage: /mascot on|off")
		}
		m.persist()
		return m, nil

	case "/key":
		return say(mutedStyle.Render("API key loaded from the " + m.keySource + "."))

	case "/login":
		m.keyOpen = true
		return m, nil

	case "/logout":
		if err := m.logout(); err != nil {
			return fail("Could not remove the saved key: " + err.Error())
		}
		text := "Removed the saved API key."
		if m.keySource == "environment" {
			text += " ACCRED_API_KEY is still set in your shell and will be used next time."
		}
		m.hasKey, m.keyOpen = false, true
		return say(mutedStyle.Render(text))
	}
	return fail("Unknown command " + name + ". Type /help for the list.")
}
