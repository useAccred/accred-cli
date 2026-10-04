// Package tui is the interactive chat terminal.
package tui

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/useAccred/accred-cli/internal/api"
	"github.com/useAccred/accred-cli/internal/config"
	"github.com/useAccred/accred-cli/internal/mascot"
)

const (
	tickInterval   = 120 * time.Millisecond
	maxInputRows   = 6
	noticeTicks    = 25
	defaultMaxToks = 1024
)

// Backend is the part of the API client the terminal needs.
type Backend interface {
	Chat(ctx context.Context, model string, messages []api.Message, maxTokens int, idempotencyKey string) (*api.Reply, error)
	Models(ctx context.Context) ([]api.Model, error)
}

type Options struct {
	Backend   Backend
	Config    *config.Config
	KeySource string
	Version   string
	Dark      bool
	// Save persists preferences. Nil disables persistence.
	Save func(*config.Config) error
	// HasKey reports whether an API key was found at startup.
	HasKey bool
	// Login verifies a key created on the Accred website, stores it and starts using it.
	Login func(ctx context.Context, key string) error
	// Logout forgets the stored key.
	Logout func() error
}

type (
	tickMsg   time.Time
	modelsMsg struct {
		models []api.Model
		err    error
	}
	replyMsg struct {
		reply *api.Reply
		err   error
	}
	loginMsg struct{ err error }
)

// reveal types a finished reply out before it is committed to the scrollback.
type reveal struct {
	runes []rune
	pos   int
	reply *api.Reply
}

type Model struct {
	backend   Backend
	cfg       *config.Config
	save      func(*config.Config) error
	keySource string
	version   string
	dark      bool

	login  func(ctx context.Context, key string) error
	logout func() error

	hasKey      bool
	keyOpen     bool // the API key box is showing
	keyChecking bool
	keyErr      string
	keyInput    textinput.Model

	width, height int
	input         textarea.Model
	mascot        *mascot.Mascot
	picker        *picker
	models        []api.Model

	history []api.Message
	system  string
	sent    []string // messages the user sent, for ↑ recall
	recall  int      // index into sent while browsing; len(sent) when not

	busy      bool
	busySince time.Time
	cancel    context.CancelFunc
	pending   string // the message being answered, restored to the input on failure
	reveal    *reveal

	spent     *big.Rat
	replies   int
	remaining string

	notice      string
	noticeLeft  int
	quitArmed   bool
	quitting    bool
	markdown    *glamour.TermRenderer
	markdownFor int
}

func New(opts Options) Model {
	input := textarea.New()
	input.Placeholder = "Ask anything, or type / for commands"
	input.ShowLineNumbers = false
	input.CharLimit = 0
	input.SetPromptFunc(2, func(line int) string {
		if line == 0 {
			return "› "
		}
		return "  "
	})
	input.FocusedStyle.CursorLine = lipgloss.NewStyle()
	input.FocusedStyle.Prompt = accentStyle
	input.FocusedStyle.Placeholder = mutedStyle
	input.SetHeight(1)
	input.Focus()

	keyInput := textinput.New()
	keyInput.Placeholder = "ct_live_…"
	keyInput.Prompt = "› "
	keyInput.PromptStyle = accentStyle
	keyInput.PlaceholderStyle = mutedStyle
	keyInput.EchoMode = textinput.EchoPassword
	keyInput.EchoCharacter = '•'
	keyInput.Focus()

	m := Model{
		login:     opts.Login,
		logout:    opts.Logout,
		hasKey:    opts.HasKey,
		keyOpen:   !opts.HasKey,
		keyInput:  keyInput,
		backend:   opts.Backend,
		cfg:       opts.Config,
		save:      opts.Save,
		keySource: opts.KeySource,
		version:   opts.Version,
		dark:      opts.Dark,
		width:     80,
		height:    24,
		input:     input,
		mascot:    mascot.New(time.Now().UnixNano()),
		spent:     new(big.Rat),
	}
	m.mascot.Set(mascot.Happy)
	m.layout()
	return m
}

func tick() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Init() tea.Cmd {
	backend := m.backend
	loadModels := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		models, err := backend.Models(ctx)
		return modelsMsg{models, err}
	}
	return tea.Batch(textarea.Blink, tick(), loadModels, tea.Println(m.bannerText()))
}

func (m *Model) layout() {
	m.input.SetWidth(max(10, m.width-4))
	m.keyInput.Width = max(10, m.width-6)
	m.input.SetHeight(max(1, min(m.input.LineCount(), maxInputRows)))
}

func (m *Model) persist() {
	if m.save == nil {
		return
	}
	if err := m.save(m.cfg); err != nil {
		m.setNotice("Could not save preferences: " + err.Error())
	}
}

func (m *Model) setNotice(text string) {
	m.notice, m.noticeLeft = text, noticeTicks
}

func (m Model) maxTokens() int {
	if m.cfg.MaxTokens > 0 {
		return m.cfg.MaxTokens
	}
	return defaultMaxToks
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil
	case tickMsg:
		return m.onTick()
	case modelsMsg:
		return m.onModels(msg)
	case replyMsg:
		return m.onReply(msg)
	case loginMsg:
		return m.onLogin(msg)
	case tea.KeyMsg:
		return m.onKey(msg)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) onTick() (tea.Model, tea.Cmd) {
	m.mascot.Tick(m.width)
	if m.noticeLeft > 0 {
		if m.noticeLeft--; m.noticeLeft == 0 {
			m.notice = ""
		}
	}
	if m.reveal != nil {
		// Long replies speed up so the reveal never takes more than a few seconds.
		m.reveal.pos += max(4, len(m.reveal.runes)/30)
		if m.reveal.pos >= len(m.reveal.runes) {
			return m.finishReveal(tick())
		}
	}
	return m, tick()
}

func (m Model) onModels(msg modelsMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.mascot.Set(mascot.Sad)
		return m, tea.Println(errorStyle.Render("Could not load the model list: "+msg.err.Error()) + "\n")
	}
	m.models = msg.models
	known := false
	for _, candidate := range m.models {
		known = known || candidate.ID == m.cfg.Model
	}
	if known {
		return m, nil
	}
	if m.cfg.Model != "" {
		m.setNotice(m.cfg.Model + " is no longer available. Pick another model.")
		m.cfg.Model = ""
	}
	m.picker = newPicker(m.models, "")
	return m, nil
}

func (m Model) selectModel(choice api.Model) (tea.Model, tea.Cmd) {
	m.cfg.Model = choice.ID
	m.persist()
	m.picker = nil
	return m, tea.Println(mutedStyle.Render("Model set to ") + accentStyle.Render(choice.ID) + "\n")
}

func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mascot.Wake()
	if msg.Type != tea.KeyCtrlC {
		m.quitArmed = false
	}

	if m.keyOpen {
		return m.onKeyEntry(msg)
	}

	if m.picker != nil {
		if done, choice := m.picker.update(msg); done {
			if choice != nil {
				return m.selectModel(*choice)
			}
			m.picker = nil
		}
		return m, nil
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		switch {
		case m.busy && m.reveal == nil:
			m.cancel()
		case m.input.Value() != "":
			m.input.Reset()
			m.layout()
		case m.quitArmed:
			m.quitting = true
			return m, tea.Quit
		default:
			m.quitArmed = true
			m.setNotice("Press ctrl+c again to exit")
		}
		return m, nil

	case tea.KeyCtrlD:
		if m.input.Value() == "" {
			m.quitting = true
			return m, tea.Quit
		}

	case tea.KeyEsc:
		if m.reveal != nil {
			return m.finishReveal(nil)
		}
		if m.busy {
			m.cancel()
		}
		return m, nil

	case tea.KeyEnter:
		if msg.Alt {
			m.input.InsertString("\n")
			m.layout()
			return m, nil
		}
		if m.reveal != nil {
			return m.finishReveal(nil)
		}
		if m.busy {
			return m, nil
		}
		return m.submit()

	case tea.KeyCtrlJ:
		m.input.InsertString("\n")
		m.layout()
		return m, nil

	case tea.KeyTab:
		if value := m.input.Value(); strings.HasPrefix(value, "/") && !strings.Contains(value, " ") {
			if matches := matchingCommands(value); len(matches) > 0 {
				m.input.SetValue(matches[0].name + " ")
				m.input.CursorEnd()
			}
		}
		return m, nil

	case tea.KeyUp, tea.KeyDown:
		if m.input.LineCount() == 1 && len(m.sent) > 0 {
			if msg.Type == tea.KeyUp {
				m.recall = max(0, m.recall-1)
			} else {
				m.recall = min(len(m.sent), m.recall+1)
			}
			if m.recall == len(m.sent) {
				m.input.Reset()
			} else {
				m.input.SetValue(m.sent[m.recall])
				m.input.CursorEnd()
			}
			m.layout()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.layout()
	return m, cmd
}

// onKeyEntry handles the box where a key created on the website is pasted.
func (m Model) onKeyEntry(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyCtrlD:
		if !m.hasKey {
			m.quitting = true
			return m, tea.Quit
		}
		m.closeKeyEntry()
		return m, nil
	case tea.KeyEsc:
		if m.hasKey {
			m.closeKeyEntry()
		}
		return m, nil
	case tea.KeyEnter:
		key := strings.TrimSpace(m.keyInput.Value())
		if m.keyChecking || key == "" {
			return m, nil
		}
		m.keyChecking, m.keyErr = true, ""
		m.mascot.Set(mascot.Thinking)
		login := m.login
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			return loginMsg{login(ctx, key)}
		}
	}
	if m.keyChecking {
		return m, nil
	}
	var cmd tea.Cmd
	m.keyInput, cmd = m.keyInput.Update(msg)
	return m, cmd
}

func (m *Model) closeKeyEntry() {
	m.keyOpen, m.keyChecking, m.keyErr = false, false, ""
	m.keyInput.SetValue("")
}

func (m Model) onLogin(msg loginMsg) (tea.Model, tea.Cmd) {
	m.keyChecking = false
	if msg.err != nil {
		m.keyErr = msg.err.Error()
		m.mascot.Set(mascot.Sad)
		return m, nil
	}
	m.hasKey, m.keySource = true, "keychain"
	m.closeKeyEntry()
	m.mascot.Set(mascot.Happy)
	return m, tea.Println(mutedStyle.Render("API key verified and saved to your system keychain.") + "\n")
}

func (m Model) submit() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return m, nil
	}
	m.input.Reset()
	m.layout()
	m.sent = append(m.sent, text)
	m.recall = len(m.sent)

	if strings.HasPrefix(text, "/") {
		return m.runCommand(text)
	}
	if m.cfg.Model == "" {
		if m.models == nil {
			m.input.SetValue(text)
			m.setNotice("Still loading the model list. Try again in a moment.")
			return m, nil
		}
		m.input.SetValue(text)
		m.layout()
		m.picker = newPicker(m.models, "")
		return m, nil
	}

	m.history = append(m.history, api.Message{Role: "user", Content: text})
	messages := make([]api.Message, 0, len(m.history)+1)
	if m.system != "" {
		messages = append(messages, api.Message{Role: "system", Content: m.system})
	}
	messages = append(messages, m.history...)

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.busy, m.busySince, m.pending = true, time.Now(), text
	m.mascot.Set(mascot.Thinking)

	backend, model, maxTokens := m.backend, m.cfg.Model, m.maxTokens()
	call := func() tea.Msg {
		reply, err := backend.Chat(ctx, model, messages, maxTokens, api.NewIdempotencyKey())
		return replyMsg{reply, err}
	}
	return m, tea.Batch(tea.Println(m.userText(text)), call)
}

func (m Model) onReply(msg replyMsg) (tea.Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if msg.err != nil {
		// Drop the unanswered message and hand it back so it can be edited or resent.
		m.busy = false
		m.history = m.history[:len(m.history)-1]
		if m.input.Value() == "" {
			m.input.SetValue(m.pending)
			m.input.CursorEnd()
			m.layout()
		}
		if errors.Is(msg.err, context.Canceled) {
			m.mascot.Set(mascot.Idle)
			m.setNotice("Cancelled")
			return m, nil
		}
		m.mascot.Set(mascot.Sad)
		return m, tea.Println(errorStyle.Render(describeError(msg.err)) + "\n")
	}

	reply := msg.reply
	m.replies++
	if charged, ok := new(big.Rat).SetString(reply.CreditsCharged); ok {
		m.spent.Add(m.spent, charged)
	}
	if reply.RemainingCredits != "" {
		m.remaining = reply.RemainingCredits
	}
	m.mascot.Set(mascot.Idle)
	m.reveal = &reveal{runes: []rune(reply.Content), reply: reply}
	return m, nil
}

// finishReveal commits the reply to the scrollback. extra is batched with the print.
func (m Model) finishReveal(extra tea.Cmd) (tea.Model, tea.Cmd) {
	reply := m.reveal.reply
	m.reveal = nil
	m.busy = false
	m.history = append(m.history, api.Message{Role: "assistant", Content: reply.Content})
	return m, tea.Batch(tea.Println(m.assistantText(reply)), extra)
}

func describeError(err error) string {
	switch api.StatusOf(err) {
	case http.StatusUnauthorized:
		return err.Error() + "\nType /login to add a working key."
	case http.StatusPaymentRequired:
		return err.Error() + "\nActivate more credit on the Wallet page at accred.sh, or lower the limit with /max."
	case http.StatusTooManyRequests:
		return err.Error() + "\nToo many requests. Wait a moment and send again."
	}
	return fmt.Sprintf("%s\nNothing was charged. Press enter to send again.", err.Error())
}
