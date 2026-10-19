package agenttui

import (
	"context"
	"fmt"
	uv "github.com/charmbracelet/ultraviolet"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSizedApp(cfg Config) *App {
	cfg.Theme = DefaultTheme()
	a := New(cfg)
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	a.initDone = true
	return a
}

func keyMsg(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

func TestAsk_NotRunning(t *testing.T) {
	a := newSizedApp(Config{})
	_, err := a.Ask(context.Background(), Question{Text: "x?"})
	assert.ErrorIs(t, err, ErrNotRunning)
}

func openTestAsk(a *App, q Question) *askState {
	st := &askState{id: 1, q: q, reply: make(chan askReply, 1)}
	a.Update(askOpenMsg{st})
	return st
}

func TestAsk_OptionsSelectWithArrowsAndEnter(t *testing.T) {
	a := newSizedApp(Config{})
	st := openTestAsk(a, Question{Text: "Run?", Options: []string{"yes", "no", "always"}, Default: "yes"})
	require.NotNil(t, a.ask)
	assert.Contains(t, a.View().Content, "Run?")

	a.Update(keyMsg("down"))
	a.Update(keyMsg("enter"))
	r := <-st.reply
	assert.NoError(t, r.err)
	assert.Equal(t, "no", r.answer)
	assert.Nil(t, a.ask)
}

func TestAsk_NumberQuickPickAndDismiss(t *testing.T) {
	a := newSizedApp(Config{})
	st := openTestAsk(a, Question{Text: "Pick", Options: []string{"a", "b"}})
	a.Update(keyMsg("2"))
	assert.Equal(t, "b", (<-st.reply).answer)

	st = openTestAsk(a, Question{Text: "Pick", Options: []string{"a", "b"}})
	a.Update(keyMsg("esc"))
	assert.ErrorIs(t, (<-st.reply).err, ErrAskCancelled)
	assert.Nil(t, a.ask)
}

func TestAsk_FreeTextAndDefault(t *testing.T) {
	a := newSizedApp(Config{})
	st := openTestAsk(a, Question{Text: "Name?"})
	a.Update(keyMsg("h"))
	a.Update(keyMsg("i"))
	a.Update(keyMsg("enter"))
	assert.Equal(t, "hi", (<-st.reply).answer)

	st = openTestAsk(a, Question{Text: "Name?", Default: "anon"})
	a.Update(keyMsg("enter"))
	assert.Equal(t, "anon", (<-st.reply).answer)
}

func TestAsk_ClosedByContextMessage(t *testing.T) {
	a := newSizedApp(Config{})
	openTestAsk(a, Question{Text: "?", Options: []string{"a"}})
	a.Update(askCloseMsg{id: 99}) // stale id ignored
	assert.NotNil(t, a.ask)
	a.Update(askCloseMsg{id: 1})
	assert.Nil(t, a.ask)
}

func TestStreamEvents_ToolOrderingAndThinking(t *testing.T) {
	a := newSizedApp(Config{
		StreamEvents: func(ctx context.Context, p string, emit func(Event)) (int, error) { return 1, nil },
	})
	a.handleSubmit("go")
	require.True(t, a.streaming)

	a.Update(streamChunkMsg{chunk: "before tool"})
	a.Update(streamEventMsg{Event{Kind: EventToolStart, ToolID: "t1", ToolName: "grep", ToolArgs: "x"}})
	a.Update(streamEventMsg{Event{Kind: EventToolEnd, ToolID: "t1", ToolOutput: "ok", ToolOK: true}})
	a.Update(streamChunkMsg{chunk: "<think>hmm</think>after tool"})
	a.Update(streamDoneMsg{tokens: 3})

	roles := []Role{}
	for _, m := range a.messages {
		roles = append(roles, m.Role)
	}
	assert.Equal(t, []Role{RoleUser, RoleAssistant, RoleTool, RoleAssistant}, roles)
	assert.Equal(t, "before tool", a.messages[1].Content)
	assert.Equal(t, ToolStatusSuccess, a.messages[2].Tool.Status)
	assert.Equal(t, "hmm", a.messages[3].Thinking)
	assert.Equal(t, "after tool", a.messages[3].Content)
}

func TestStreamEvents_EmitWrapsThinking(t *testing.T) {
	ch := make(chan tea.Msg, 16)
	a := newSizedApp(Config{
		StreamEvents: func(ctx context.Context, p string, emit func(Event)) (int, error) {
			emit(Event{Kind: EventThinking, Text: "t1"})
			emit(Event{Kind: EventThinking, Text: "t2"})
			emit(Event{Kind: EventText, Text: "answer"})
			return 0, nil
		},
	})
	a.handleSubmit("q")
	ch = a.streamCh
	var got string
	for m := range ch {
		if c, ok := m.(streamChunkMsg); ok {
			got += c.chunk
		}
	}
	assert.Equal(t, "<think>t1t2</think>answer", got)
}

type memStore struct{ msgs []Message }

func (m *memStore) Load() ([]Message, error) { return m.msgs, nil }
func (m *memStore) Save(x []Message) error   { m.msgs = x; return nil }

func TestStore_LoadAndSave(t *testing.T) {
	st := &memStore{msgs: []Message{{Role: RoleUser, Content: "old"}}}
	a := newSizedApp(Config{Store: st, Stream: func(ctx context.Context, p string, emit func(string)) (int, error) { return 0, nil }})
	require.Len(t, a.messages, 1)

	a.handleSubmit("new")
	a.Update(streamChunkMsg{chunk: "reply"})
	a.Update(streamDoneMsg{})
	require.Len(t, st.msgs, 3)
	assert.Equal(t, "reply", st.msgs[2].Content)
}

func TestRetry_RegeneratesLastTurn(t *testing.T) {
	var prompts []string
	a := newSizedApp(Config{Stream: func(ctx context.Context, p string, emit func(string)) (int, error) {
		prompts = append(prompts, p)
		return 0, nil
	}})
	a.handleSubmit("question")
	a.Update(streamChunkMsg{chunk: "first"})
	a.Update(streamDoneMsg{})

	cmd := a.handleSubmit("/retry")
	require.NotNil(t, cmd)
	var done slashDoneMsg
	var ok bool
	if batch, isBatch := cmd().(tea.BatchMsg); isBatch {
		for _, c := range batch {
			if c == nil {
				continue
			}
			if d, is := c().(slashDoneMsg); is {
				done, ok = d, true
			}
		}
	}
	require.True(t, ok)
	assert.False(t, done.isLocal)
	assert.Equal(t, "question", done.out)

	a.Update(done)
	assert.True(t, a.streaming)
	a.Update(streamChunkMsg{chunk: "second"})
	a.Update(streamDoneMsg{})

	require.Len(t, a.messages, 2)
	assert.Equal(t, "question", a.messages[0].Content)
	assert.Equal(t, "second", a.messages[1].Content)
}

func TestIgnoreDirs(t *testing.T) {
	dir := t.TempDir()
	mk := func(p string) {
		require.NoError(t, osMkdirWrite(dir, p))
	}
	mk("keep/a.go")
	mk("skipme/b.go")
	a := New(Config{Theme: DefaultTheme(), IgnoreDirs: []string{"skipme"}})
	a.ScanRepoFiles(dir)
	assert.Equal(t, []string{"keep/a.go"}, a.repoFiles)
}

func osMkdirWrite(root, rel string) error {
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte("x"), 0o644)
}

func TestEditorKeys_WordNavigationAndDelete(t *testing.T) {
	a := newSizedApp(Config{})
	a.textarea.SetValue("hello big world")
	a.textarea.CursorEnd()
	mod := func(code rune, m tea.KeyMod) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code, Mod: m} }

	a.Update(mod(tea.KeyLeft, tea.ModCtrl))
	assert.Equal(t, 10, a.textarea.Column()) // start of "world"
	a.Update(mod(tea.KeyLeft, tea.ModCtrl))
	assert.Equal(t, 6, a.textarea.Column()) // start of "big"
	a.Update(mod(tea.KeyRight, tea.ModCtrl))
	assert.Equal(t, 9, a.textarea.Column()) // end of "big"

	a.Update(mod(tea.KeyBackspace, tea.ModCtrl))
	assert.Equal(t, "hello  world", a.textarea.Value())

	a.textarea.CursorEnd()
	a.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	assert.Equal(t, "", a.textarea.Value())

	a.textarea.SetValue("abc def")
	a.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	assert.Equal(t, 0, a.textarea.Column())
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	assert.Equal(t, 7, a.textarea.Column())
}

func TestTmux_LegacyCtrlArrowSequences(t *testing.T) {
	a := newSizedApp(Config{})
	a.textarea.SetValue("one two three")
	a.textarea.CursorEnd()

	a.Update(uv.UnknownCsiEvent("\x1b[5D"))
	assert.Equal(t, 8, a.textarea.Column())
	a.Update(uv.UnknownCsiEvent("\x1b[5D"))
	assert.Equal(t, 4, a.textarea.Column())
	a.Update(uv.UnknownCsiEvent("\x1b[5C"))
	assert.Equal(t, 7, a.textarea.Column())
	a.Update(uv.UnknownCsiEvent("\x1b[9Z")) // unrelated sequences are ignored
	assert.Equal(t, 7, a.textarea.Column())
}

func TestTmux_SidebarF2AndHint(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1,0")
	assert.Equal(t, "F2", sidebarHint())
	a := newSizedApp(Config{ShowFooter: true, Sidebar: &mockWidget{visible: true}})
	before := a.IsSidebarHidden()
	a.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	assert.NotEqual(t, before, a.IsSidebarHidden())

	t.Setenv("TMUX", "")
	t.Setenv("STY", "")
	assert.Equal(t, "Ctrl+B", sidebarHint())
}

func openTestForm(a *App, f Form) *formState {
	st := &formState{id: 1, f: f, reply: make(chan formReply, 1)}
	a.Update(formOpenMsg{st})
	return st
}

func TestForm_NotRunning(t *testing.T) {
	a := newSizedApp(Config{})
	_, err := a.Form(context.Background(), Form{Title: "x", Fields: []Field{{Label: "a"}}})
	assert.ErrorIs(t, err, ErrNotRunning)
}

func TestForm_EditFieldsAndSave(t *testing.T) {
	a := newSizedApp(Config{})
	st := openTestForm(a, Form{Title: "Settings", Fields: []Field{
		{Label: "Model", Kind: FieldChoice, Value: "a", Options: []string{"a", "b", "c"}},
		{Label: "Temp", Kind: FieldNumber, Value: "0.7", Min: 0, Max: 1, Step: 0.1},
		{Label: "Rerank", Kind: FieldBool, Value: "off"},
		{Label: "Prompt", Kind: FieldText, Value: "old"},
	}})
	require.NotNil(t, a.form)
	assert.Contains(t, a.View().Content, "Settings")

	right, left := tea.KeyPressMsg{Code: tea.KeyRight}, tea.KeyPressMsg{Code: tea.KeyLeft}
	a.Update(right) // Model a -> b
	a.Update(keyMsg("down"))
	a.Update(right) // 0.7 -> 0.8
	a.Update(right) // 0.9
	a.Update(right) // 1.0
	a.Update(right) // clamped at 1
	a.Update(left)  // 0.9
	a.Update(keyMsg("down"))
	a.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}) // Rerank on
	a.Update(keyMsg("down"))
	a.Update(keyMsg("enter")) // start editing text
	require.True(t, st.editing)
	a.textarea.SetValue("new prompt")
	a.Update(keyMsg("enter")) // commit text
	require.False(t, st.editing)
	a.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	a.Update(keyMsg("enter")) // save

	r := <-st.reply
	require.NoError(t, r.err)
	assert.Equal(t, []string{"b", "0.9", "on", "new prompt"}, r.values)
	assert.Nil(t, a.form)
}

func TestForm_EscCancelsAndEditDiscard(t *testing.T) {
	a := newSizedApp(Config{})
	st := openTestForm(a, Form{Title: "S", Fields: []Field{{Label: "P", Kind: FieldText, Value: "keep"}}})
	a.Update(keyMsg("enter"))
	a.textarea.SetValue("changed")
	a.Update(keyMsg("esc")) // discards the edit, form stays open
	require.NotNil(t, a.form)
	assert.Equal(t, "keep", st.f.Fields[0].Value)
	a.Update(keyMsg("esc"))
	assert.ErrorIs(t, (<-st.reply).err, ErrAskCancelled)
	assert.Nil(t, a.form)
}

func TestAsk_LongListIsWindowed(t *testing.T) {
	a := newSizedApp(Config{})
	opts := make([]string, 30)
	for i := range opts {
		opts[i] = fmt.Sprintf("item-%02d", i)
	}
	openTestAsk(a, Question{Text: "Pick", Options: opts})
	assert.Less(t, len(a.renderAsk()), 14)
	for i := 0; i < 15; i++ {
		a.Update(keyMsg("down"))
	}
	joined := strings.Join(a.renderAsk(), "\n")
	assert.Contains(t, joined, "item-15")
	assert.Contains(t, joined, "more")
}
