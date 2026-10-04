package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/useAccred/accred-cli/internal/api"
	"github.com/useAccred/accred-cli/internal/config"
)

type fakeBackend struct {
	reply *api.Reply
	err   error
	calls [][]api.Message
}

func (f *fakeBackend) Chat(_ context.Context, _ string, messages []api.Message, _ int, _ string) (*api.Reply, error) {
	f.calls = append(f.calls, messages)
	return f.reply, f.err
}

func (f *fakeBackend) Models(context.Context) ([]api.Model, error) { return testModels, nil }

var (
	one, two   = "1", "2"
	testModels = []api.Model{
		{ID: "alpha/small", Provider: "alpha", InputCost: &one, OutputCost: &two},
		{ID: "beta/large", Provider: "beta"},
	}
	okReply = &api.Reply{ID: "r1", Model: "alpha/small", Content: "Hello **there**", FinishReason: "stop",
		InputTokens: 12, OutputTokens: 3, CreditsCharged: "0.25", RemainingCredits: "99.75"}
)

func newTestModel(backend *fakeBackend, cfg *config.Config) Model {
	m := New(Options{Backend: backend, Config: cfg, KeySource: "environment", Version: "test", Dark: true, HasKey: true})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	next, _ = next.(Model).Update(modelsMsg{models: testModels})
	return next.(Model)
}

func key(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
	return next.(Model)
}

// send types a message, presses enter and runs the resulting chat call.
func send(t *testing.T, m Model, text string) Model {
	t.Helper()
	m = typeText(t, m, text)
	next, cmd := m.Update(key(tea.KeyEnter))
	m = next.(Model)
	if !m.busy {
		t.Fatalf("not busy after sending %q", text)
	}
	for _, msg := range runBatch(cmd) {
		if reply, ok := msg.(replyMsg); ok {
			next, _ = m.Update(reply)
			return next.(Model)
		}
	}
	t.Fatal("sending produced no chat call")
	return m
}

// runBatch executes a command tree and returns the messages it produced.
func runBatch(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, runBatch(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func assertFits(t *testing.T, m Model) {
	t.Helper()
	for _, line := range strings.Split(m.View(), "\n") {
		if w := lipgloss.Width(line); w > m.width {
			t.Fatalf("line is %d wide on a %d-column screen: %q", w, m.width, line)
		}
	}
}

func TestSendRevealsThenCommitsReply(t *testing.T) {
	backend := &fakeBackend{reply: okReply}
	m := send(t, newTestModel(backend, &config.Config{Model: "alpha/small"}), "hi")

	if m.reveal == nil || !m.busy {
		t.Fatal("reply should be revealing")
	}
	assertFits(t, m)
	for m.reveal != nil {
		next, _ := m.Update(tickMsg{})
		m = next.(Model)
	}
	if m.busy {
		t.Fatal("still busy after the reveal finished")
	}
	want := []api.Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "Hello **there**"}}
	if len(m.history) != 2 || m.history[0] != want[0] || m.history[1] != want[1] {
		t.Fatalf("history = %+v", m.history)
	}
	if m.remaining != "99.75" || m.spent.FloatString(2) != "0.25" || m.replies != 1 {
		t.Fatalf("remaining=%q spent=%s replies=%d", m.remaining, m.spent.FloatString(2), m.replies)
	}
	if status := m.statusLine(); !strings.Contains(status, "99.75 credits") || !strings.Contains(status, "session 0.25") {
		t.Fatalf("status = %q", status)
	}
}

func TestSystemPromptAndHistoryAreSent(t *testing.T) {
	backend := &fakeBackend{reply: okReply}
	m := newTestModel(backend, &config.Config{Model: "alpha/small"})
	m = typeText(t, m, "/system Be brief.")
	next, _ := m.Update(key(tea.KeyEnter))
	m = send(t, next.(Model), "first")
	next, _ = m.Update(key(tea.KeyEsc)) // skip the reveal
	m = send(t, next.(Model), "second")

	got := backend.calls[1]
	want := []api.Message{
		{Role: "system", Content: "Be brief."},
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "Hello **there**"},
		{Role: "user", Content: "second"},
	}
	if len(got) != len(want) {
		t.Fatalf("messages = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("messages[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestFailedSendRestoresTheMessage(t *testing.T) {
	backend := &fakeBackend{err: &api.Error{Status: 402, Message: "Not enough credit"}}
	m := send(t, newTestModel(backend, &config.Config{Model: "alpha/small"}), "expensive question")

	if m.busy || len(m.history) != 0 {
		t.Fatalf("busy=%v history=%+v", m.busy, m.history)
	}
	if m.input.Value() != "expensive question" {
		t.Fatalf("input = %q", m.input.Value())
	}
	if !strings.Contains(describeError(backend.err), "Activate more credit") {
		t.Fatal("402 should explain how to add credit")
	}
}

func TestCancelledSendIsNotAnError(t *testing.T) {
	backend := &fakeBackend{err: context.Canceled}
	m := send(t, newTestModel(backend, &config.Config{Model: "alpha/small"}), "never mind")
	if m.busy || m.notice != "Cancelled" || m.input.Value() != "never mind" {
		t.Fatalf("busy=%v notice=%q input=%q", m.busy, m.notice, m.input.Value())
	}
}

func TestFirstRunOpensPickerAndSavesChoice(t *testing.T) {
	var saved string
	cfg := &config.Config{}
	m := New(Options{Backend: &fakeBackend{}, Config: cfg, Version: "test", HasKey: true,
		Save: func(c *config.Config) error { saved = c.Model; return nil }})
	next, _ := m.Update(modelsMsg{models: testModels})
	m = next.(Model)
	if m.picker == nil {
		t.Fatal("picker should open when no model is saved")
	}
	assertFits(t, m)

	m = typeText(t, m, "beta")
	if len(m.picker.matches) != 1 {
		t.Fatalf("filter matched %d models", len(m.picker.matches))
	}
	next, _ = m.Update(key(tea.KeyEnter))
	m = next.(Model)
	if m.picker != nil || cfg.Model != "beta/large" || saved != "beta/large" {
		t.Fatalf("picker=%v model=%q saved=%q", m.picker, cfg.Model, saved)
	}
}

func TestSavedModelThatVanishedReopensPicker(t *testing.T) {
	cfg := &config.Config{Model: "gone/model"}
	m := newTestModel(&fakeBackend{}, cfg)
	if m.picker == nil || cfg.Model != "" || !strings.Contains(m.notice, "no longer available") {
		t.Fatalf("picker=%v model=%q notice=%q", m.picker, cfg.Model, m.notice)
	}
}

func TestCommands(t *testing.T) {
	cfg := &config.Config{Model: "alpha/small"}
	m := newTestModel(&fakeBackend{reply: okReply}, cfg)
	run := func(line string) Model {
		m = typeText(t, m, line)
		next, _ := m.Update(key(tea.KeyEnter))
		m = next.(Model)
		return m
	}

	if run("/max 2048"); cfg.MaxTokens != 2048 || m.maxTokens() != 2048 {
		t.Fatalf("max tokens = %d", cfg.MaxTokens)
	}
	if run("/max 99999"); cfg.MaxTokens != 2048 {
		t.Fatal("an out-of-range /max must be rejected")
	}
	if !strings.Contains(m.View(), "▀") {
		t.Fatal("mascot should be visible by default")
	}
	if run("/mascot off"); !cfg.NoMascot || strings.Contains(m.View(), "▀") {
		t.Fatal("mascot should be hidden")
	}
	if run("/model beta/large"); cfg.Model != "beta/large" || m.picker != nil {
		t.Fatalf("exact /model should switch directly, got %q", cfg.Model)
	}
	if run("/model alp"); m.picker == nil || m.picker.filter != "alp" {
		t.Fatal("a partial /model should open the picker with the filter applied")
	}
	next, _ := m.Update(key(tea.KeyEsc))
	m = next.(Model)
	if run("/quit"); !m.quitting {
		t.Fatal("/quit should exit")
	}
}

func TestTabCompletesCommandAndHintsFit(t *testing.T) {
	m := newTestModel(&fakeBackend{}, &config.Config{Model: "alpha/small"})
	m = typeText(t, m, "/mo")
	if hints := m.hints(); !strings.Contains(hints, "/model") || strings.Contains(hints, "/max") {
		t.Fatalf("hints = %q", hints)
	}
	assertFits(t, m)
	next, _ := m.Update(key(tea.KeyTab))
	if got := next.(Model).input.Value(); got != "/model " {
		t.Fatalf("input = %q", got)
	}
}

func TestCtrlCTwiceQuits(t *testing.T) {
	m := newTestModel(&fakeBackend{}, &config.Config{Model: "alpha/small"})
	next, _ := m.Update(key(tea.KeyCtrlC))
	m = next.(Model)
	if m.quitting || !m.quitArmed {
		t.Fatal("first ctrl+c should only arm")
	}
	next, cmd := m.Update(key(tea.KeyCtrlC))
	if !next.(Model).quitting || cmd == nil {
		t.Fatal("second ctrl+c should quit")
	}
}

func TestViewFitsNarrowAndWideScreens(t *testing.T) {
	for _, width := range []int{24, 60, 200} {
		m := newTestModel(&fakeBackend{reply: okReply}, &config.Config{Model: "alpha/small"})
		next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		m = typeText(t, next.(Model), strings.Repeat("a long message ", 20))
		assertFits(t, m)
		m.remaining = "1234.5678"
		m.input.Reset()
		assertFits(t, m)
	}
}

// runLogin presses enter in the key box and feeds the result back in.
func runLogin(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := m.Update(key(tea.KeyEnter))
	m = next.(Model)
	if cmd == nil {
		t.Fatal("enter did not start a key check")
	}
	if !m.keyChecking {
		t.Fatal("key check should be in progress")
	}
	next, _ = m.Update(cmd())
	return next.(Model)
}

func TestFirstRunAsksForKeyBeforeAnythingElse(t *testing.T) {
	var got []string
	loginErr := errors.New("the key was not accepted: revoked")
	m := New(Options{Backend: &fakeBackend{}, Config: &config.Config{}, Version: "test",
		Login: func(_ context.Context, k string) error {
			got = append(got, k)
			if k == "ct_live_good" {
				return nil
			}
			return loginErr
		}})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	next, _ = next.(Model).Update(modelsMsg{models: testModels})
	m = next.(Model)

	if !m.keyOpen || !strings.Contains(m.View(), "Add your API key") || strings.Contains(m.View(), "Select a model") {
		t.Fatal("the key box should be showing, ahead of the model picker")
	}
	assertFits(t, m)

	m = runLogin(t, typeText(t, m, "ct_live_bad"))
	if !m.keyOpen || m.hasKey || !strings.Contains(m.View(), "revoked") {
		t.Fatalf("a rejected key should keep the box open with the reason; view:\n%s", m.View())
	}
	if strings.Contains(m.View(), "ct_live_bad") {
		t.Fatal("the key must never be shown on screen")
	}
	assertFits(t, m)

	m.keyInput.SetValue("")
	m = runLogin(t, typeText(t, m, "  ct_live_good  "))
	if m.keyOpen || !m.hasKey || m.keySource != "keychain" || m.keyInput.Value() != "" {
		t.Fatalf("keyOpen=%v hasKey=%v source=%q", m.keyOpen, m.hasKey, m.keySource)
	}
	if len(got) != 2 || got[1] != "ct_live_good" {
		t.Fatalf("login received %q", got)
	}
	if m.picker == nil || !strings.Contains(m.View(), "Select a model") {
		t.Fatal("the model picker should follow once the key is saved")
	}
}

func TestLoginAndLogoutCommands(t *testing.T) {
	loggedOut := false
	m := New(Options{Backend: &fakeBackend{}, Config: &config.Config{Model: "alpha/small"}, Version: "test",
		HasKey: true, KeySource: "keychain",
		Login:  func(context.Context, string) error { return nil },
		Logout: func() error { loggedOut = true; return nil }})
	next, _ := m.Update(modelsMsg{models: testModels})
	m = next.(Model)

	next, _ = typeText(t, m, "/login").Update(key(tea.KeyEnter))
	m = next.(Model)
	if !m.keyOpen {
		t.Fatal("/login should open the key box")
	}
	next, _ = m.Update(key(tea.KeyEsc))
	m = next.(Model)
	if m.keyOpen || !m.hasKey {
		t.Fatal("esc should close the box and keep the existing key")
	}

	next, _ = typeText(t, m, "/logout").Update(key(tea.KeyEnter))
	m = next.(Model)
	if !loggedOut || m.hasKey || !m.keyOpen {
		t.Fatalf("loggedOut=%v hasKey=%v keyOpen=%v", loggedOut, m.hasKey, m.keyOpen)
	}
	next, _ = m.Update(key(tea.KeyEsc))
	if !next.(Model).keyOpen {
		t.Fatal("without a key the box cannot be dismissed")
	}
	next, cmd := next.(Model).Update(key(tea.KeyCtrlC))
	if !next.(Model).quitting || cmd == nil {
		t.Fatal("ctrl+c should exit from the key box when there is no key")
	}
}
