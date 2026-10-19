package agenttui

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// FieldKind selects how a Form field is edited.
type FieldKind int

const (
	FieldChoice FieldKind = iota // ←/→ cycles through Options
	FieldNumber                  // ←/→ changes the value by Step within [Min, Max]
	FieldBool                    // ←/→ or Space toggles "on"/"off"
	FieldText                    // Enter edits the value in the input box
)

// Field is one editable setting of a Form. Values are exchanged as strings:
// Bool uses "on"/"off", Number uses plain decimal notation.
type Field struct {
	Label    string
	Kind     FieldKind
	Value    string
	Options  []string // FieldChoice
	Min, Max float64  // FieldNumber (ignored when Max <= Min)
	Step     float64  // FieldNumber (default 1)
}

// Form is a settings panel: a list of fields edited in place.
type Form struct {
	Title  string
	Fields []Field
}

type formState struct {
	id      int
	f       Form
	cursor  int
	editing bool
	reply   chan formReply
}

type formReply struct {
	values []string
	err    error
}

type formOpenMsg struct{ st *formState }
type formCloseMsg struct{ id int }

// Form shows f and blocks until the user saves it (Enter), dismisses it
// (ErrAskCancelled) or ctx is done. It returns the field values in order.
// Same calling rules as Ask: not from the UI goroutine.
func (a *App) Form(ctx context.Context, f Form) ([]string, error) {
	a.mu.Lock()
	p := a.program
	a.askSeq++
	st := &formState{id: a.askSeq, f: Form{Title: f.Title, Fields: append([]Field(nil), f.Fields...)}, reply: make(chan formReply, 1)}
	a.mu.Unlock()
	if p == nil {
		return nil, ErrNotRunning
	}
	if len(f.Fields) == 0 {
		return nil, nil
	}
	p.Send(formOpenMsg{st})
	select {
	case r := <-st.reply:
		return r.values, r.err
	case <-ctx.Done():
		p.Send(formCloseMsg{st.id})
		return nil, ctx.Err()
	}
}

func (st *formState) finish(values []string, err error) {
	select {
	case st.reply <- formReply{values, err}:
	default:
	}
}

func (st *formState) values() []string {
	out := make([]string, len(st.f.Fields))
	for i, f := range st.f.Fields {
		out[i] = f.Value
	}
	return out
}

func (a *App) openForm(st *formState) {
	a.form = st
	a.recalcViewport()
}

func (a *App) closeForm() {
	a.form = nil
	a.editingField = false
	a.textarea.Reset()
	a.recalcViewport()
	a.viewport.GotoBottom()
}

// handleFormKey processes a key press while a form is open.
func (a *App) handleFormKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	st := a.form
	key := msg.String()
	f := &st.f.Fields[st.cursor]

	if st.editing {
		switch key {
		case "enter":
			f.Value = strings.TrimSpace(a.textarea.Value())
			st.editing = false
			a.textarea.Reset()
			a.recalcViewport()
		case "esc":
			st.editing = false
			a.textarea.Reset()
			a.recalcViewport()
		default:
			var cmd tea.Cmd
			a.textarea, cmd = a.textarea.Update(msg)
			return a, cmd
		}
		return a, nil
	}

	switch key {
	case "esc", "ctrl+c":
		st.finish(nil, ErrAskCancelled)
		a.closeForm()
	case "up", "k":
		st.cursor = (st.cursor + len(st.f.Fields) - 1) % len(st.f.Fields)
	case "down", "j", "tab":
		st.cursor = (st.cursor + 1) % len(st.f.Fields)
	case "left", "h":
		f.adjust(-1)
	case "right", "l":
		f.adjust(+1)
	case "space", " ":
		if f.Kind == FieldBool {
			f.adjust(+1)
		}
	case "enter":
		if f.Kind == FieldText {
			st.editing = true
			a.textarea.SetValue(f.Value)
			a.textarea.CursorEnd()
			a.recalcViewport()
			return a, nil
		}
		st.finish(st.values(), nil)
		a.closeForm()
	}
	return a, nil
}

// adjust moves the field value one step in direction dir (-1 or +1).
func (f *Field) adjust(dir int) {
	switch f.Kind {
	case FieldChoice:
		n := len(f.Options)
		if n == 0 {
			return
		}
		idx := 0
		for i, o := range f.Options {
			if o == f.Value {
				idx = i
			}
		}
		f.Value = f.Options[(idx+dir+n)%n]
	case FieldBool:
		if f.Value == "on" {
			f.Value = "off"
		} else {
			f.Value = "on"
		}
	case FieldNumber:
		v, err := strconv.ParseFloat(f.Value, 64)
		if err != nil {
			v = f.Min
		}
		step := f.Step
		if step <= 0 {
			step = 1
		}
		v += float64(dir) * step
		if f.Max > f.Min {
			v = math.Max(f.Min, math.Min(f.Max, v))
		}
		f.Value = strconv.FormatFloat(math.Round(v*1e6)/1e6, 'f', -1, 64)
	}
}

// renderForm draws the settings panel shown above the input box.
func (a *App) renderForm() []string {
	st := a.form
	th := a.cfg.Theme
	dim := lipgloss.NewStyle().Foreground(th.Muted)
	lines := []string{" " + lipgloss.NewStyle().Bold(true).Foreground(th.Accent).Render("⚙ "+st.f.Title)}

	labelW := 0
	for _, f := range st.f.Fields {
		if w := lipgloss.Width(f.Label); w > labelW {
			labelW = w
		}
	}
	maxVal := a.width - labelW - 12
	if maxVal < 10 {
		maxVal = 10
	}
	for i, f := range st.f.Fields {
		val := f.Value
		if f.Kind == FieldText {
			val = strings.Join(strings.Fields(val), " ")
			if val == "" {
				val = "(empty)"
			}
		}
		if lipgloss.Width(val) > maxVal {
			val = string([]rune(val)[:maxVal-1]) + "…"
		}
		switch f.Kind {
		case FieldChoice, FieldNumber, FieldBool:
			val = "‹ " + val + " ›"
		}
		mark, style := "  ", lipgloss.NewStyle().Foreground(th.SubText)
		if i == st.cursor {
			mark, style = "❯ ", lipgloss.NewStyle().Bold(true).Foreground(th.Primary)
		}
		pad := strings.Repeat(" ", labelW-lipgloss.Width(f.Label))
		lines = append(lines, "   "+style.Render(fmt.Sprintf("%s%s%s  %s", mark, f.Label, pad, val)))
	}
	hint := "↑/↓ field · ←/→ change · Enter save · Esc cancel"
	if st.editing {
		hint = "editing " + st.f.Fields[st.cursor].Label + " — Enter keep · Esc discard"
	} else if st.f.Fields[st.cursor].Kind == FieldText {
		hint = "↑/↓ field · Enter edit text · Esc cancel"
	}
	return append(lines, "   "+dim.Render(hint))
}
