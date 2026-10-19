package agenttui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ErrAskCancelled is returned by App.Ask when the user dismisses the prompt.
var ErrAskCancelled = errors.New("agenttui: question cancelled")

// ErrNotRunning is returned by App.Ask when the TUI is not running.
var ErrNotRunning = errors.New("agenttui: app is not running")

// Question is a prompt the agent puts to the user. With Options the user picks
// one (arrows, Enter, or the option number); without them the user types free
// text into the input box.
type Question struct {
	Text    string
	Detail  string   // optional background shown under the question
	Options []string // empty means free-text answer
	Default string   // pre-selected option, or the text used on an empty free-text answer
}

type askState struct {
	id     int
	q      Question
	cursor int
	reply  chan askReply
}

type askReply struct {
	answer string
	err    error
}

type askOpenMsg struct{ st *askState }
type askCloseMsg struct{ id int }

// Ask shows q and blocks until the user answers, dismisses it (ErrAskCancelled)
// or ctx is done. It is safe to call from stream and slash-command handlers,
// i.e. from any goroutine except the UI loop itself.
func (a *App) Ask(ctx context.Context, q Question) (string, error) {
	a.mu.Lock()
	p := a.program
	a.askSeq++
	st := &askState{id: a.askSeq, q: q, reply: make(chan askReply, 1)}
	a.mu.Unlock()
	if p == nil {
		return "", ErrNotRunning
	}
	for i, o := range q.Options {
		if o == q.Default {
			st.cursor = i
		}
	}

	p.Send(askOpenMsg{st})
	select {
	case r := <-st.reply:
		return r.answer, r.err
	case <-ctx.Done():
		p.Send(askCloseMsg{st.id})
		return "", ctx.Err()
	}
}

func (st *askState) finish(answer string, err error) {
	select {
	case st.reply <- askReply{answer, err}:
	default:
	}
}

// handleAskKey processes a key press while a question is open.
func (a *App) handleAskKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	st := a.ask
	key := msg.String()
	switch key {
	case "esc", "ctrl+c":
		st.finish("", ErrAskCancelled)
		a.closeAsk()
		return a, nil
	}
	if len(st.q.Options) == 0 {
		if key == "enter" {
			ans := strings.TrimSpace(a.textarea.Value())
			if ans == "" {
				ans = st.q.Default
			}
			a.textarea.Reset()
			st.finish(ans, nil)
			a.closeAsk()
			return a, nil
		}
		var cmd tea.Cmd
		a.textarea, cmd = a.textarea.Update(msg)
		return a, cmd
	}
	n := len(st.q.Options)
	switch key {
	case "up", "k":
		st.cursor = (st.cursor + n - 1) % n
	case "down", "j", "tab":
		st.cursor = (st.cursor + 1) % n
	case "enter":
		st.finish(st.q.Options[st.cursor], nil)
		a.closeAsk()
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			if i := int(key[0] - '1'); i < n {
				st.finish(st.q.Options[i], nil)
				a.closeAsk()
			}
		}
	}
	return a, nil
}

func (a *App) openAsk(st *askState) {
	a.ask = st
	if len(st.q.Options) == 0 {
		a.textarea.Reset()
	}
	a.recalcViewport()
}

func (a *App) closeAsk() {
	a.ask = nil
	a.recalcViewport()
	a.viewport.GotoBottom()
}

// renderAsk draws the question panel shown in place of the usual status line.
func (a *App) renderAsk() []string {
	st := a.ask
	th := a.cfg.Theme
	head := lipgloss.NewStyle().Bold(true).Foreground(th.Accent).Render("? " + st.q.Text)
	lines := []string{" " + head}
	dim := lipgloss.NewStyle().Foreground(th.Muted)
	if st.q.Detail != "" {
		for _, l := range strings.Split(st.q.Detail, "\n") {
			lines = append(lines, "   "+dim.Render(l))
		}
	}
	if len(st.q.Options) == 0 {
		hint := "Enter answer"
		if st.q.Default != "" {
			hint += fmt.Sprintf(" (empty = %q)", st.q.Default)
		}
		lines = append(lines, "   "+dim.Render(hint+" · Esc dismiss"))
		return lines
	}
	for i, o := range st.q.Options {
		mark, style := "  ", lipgloss.NewStyle().Foreground(th.SubText)
		if i == st.cursor {
			mark, style = "❯ ", lipgloss.NewStyle().Bold(true).Foreground(th.Primary)
		}
		lines = append(lines, "   "+style.Render(fmt.Sprintf("%s%d. %s", mark, i+1, o)))
	}
	lines = append(lines, "   "+dim.Render("↑/↓ select · Enter confirm · 1-9 quick pick · Esc dismiss"))
	return lines
}
