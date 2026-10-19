package agenttui

import (
	"context"
	"fmt"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestPresentationHooks(t *testing.T) {
	var keys []string
	a := New(Config{
		Theme: DefaultTheme(), Title: "gleann", ModelName: "m1", ShowHeader: true, ShowFooter: true,
		HideMode:   true,
		HeaderInfo: func() string { return "idx=docs" },
		InfoRows:   func() [][2]string { return [][2]string{{"index", "docs"}, {"mode", "RAG"}} },
		FooterHint: "Ctrl+S settings · Enter send",
		Banner:     []string{"## BANNER ##"},
		OnKey:      func(k string, _ *App) bool { keys = append(keys, k); return k == "ctrl+s" },
	})
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	a.initDone = true
	view := ansi.Strip(a.View().Content)

	assert.NotContains(t, view, "[BUILD]")
	assert.Contains(t, view, "idx=docs")
	assert.Contains(t, view, "index")
	assert.Contains(t, view, "RAG")
	assert.Contains(t, view, "Ctrl+S settings")
	assert.NotContains(t, view, "Ctrl+B")
	assert.Contains(t, view, "## BANNER ##")

	a.textarea.SetValue("x")
	a.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	assert.Equal(t, []string{"ctrl+s"}, keys)
	assert.Equal(t, "x", a.textarea.Value(), "consumed key must not reach the input")
	a.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	assert.Equal(t, "xa", a.textarea.Value())
}

func TestSubmit_RunsSlashCommandLikeTyping(t *testing.T) {
	a := newSizedApp(Config{Commands: []SlashCommand{{Name: "ping", Handler: func(ctx context.Context, args []string, app *App) (string, bool, error) {
		return "pong", true, nil
	}}}})
	a.Submit("/ping") // no program yet: must not panic or block

	_, cmd := a.Update(submitMsg{"/ping"})
	assert.NotNil(t, cmd)
	assert.True(t, a.waiting, "command is running")
	assert.Equal(t, "/ping", a.messages[len(a.messages)-1].Content)

	_, cmd = a.Update(submitMsg{"/ping"})
	assert.Nil(t, cmd, "submit is ignored while a turn is running")
}

// Regression: Submit called from OnKey (i.e. on the UI goroutine) used to
// deadlock the whole program because Program.Send blocks on the event loop.
func TestSubmit_FromOnKeyDoesNotDeadlock(t *testing.T) {
	ran := make(chan struct{}, 1)
	var app *App
	app = New(Config{
		Theme: DefaultTheme(),
		Commands: []SlashCommand{{Name: "ping", Handler: func(ctx context.Context, args []string, a *App) (string, bool, error) {
			ran <- struct{}{}
			return "pong", true, nil
		}}},
		OnKey: func(k string, a *App) bool {
			if k == "ctrl+s" {
				a.Submit("/ping")
				return true
			}
			return false
		},
	})
	r, w := io.Pipe()
	p := tea.NewProgram(app, tea.WithInput(r), tea.WithOutput(io.Discard), tea.WithWindowSize(100, 30))
	app.mu.Lock()
	app.program = p
	app.mu.Unlock()
	done := make(chan struct{})
	go func() { _, _ = p.Run(); close(done) }()
	defer func() { p.Quit(); w.Close(); <-done }()

	time.Sleep(1200 * time.Millisecond) // wait for initDone
	_, _ = w.Write([]byte{0x13})        // Ctrl+S
	select {
	case <-ran:
	case <-time.After(3 * time.Second):
		t.Fatal("Submit from OnKey deadlocked the program")
	}
}

func TestForm_SliderShowsLimitsAndClamps(t *testing.T) {
	a := newSizedApp(Config{})
	st := openTestForm(a, Form{Title: "S", Note: "changing embedding needs a rebuild", Fields: []Field{
		{Label: "Temperature", Kind: FieldSlider, Value: "0.1", Options: []string{"0.0", "0.1", "0.2", "0.3"}, Hint: "creativity"},
		{Label: "Top-K", Kind: FieldSlider, Value: "5", Min: 1, Max: 6, Step: 1, Format: "%.0f"},
	}})
	view := ansi.Strip(strings.Join(a.renderForm(), "\n"))
	assert.Contains(t, view, "0.0 … 0.3")
	assert.Contains(t, view, "creativity")
	assert.Contains(t, view, "changing embedding")
	assert.Contains(t, view, "━")

	left, right := tea.KeyPressMsg{Code: tea.KeyLeft}, tea.KeyPressMsg{Code: tea.KeyRight}
	a.Update(left)
	a.Update(left) // clamps at the first preset
	assert.Equal(t, "0.0", st.f.Fields[0].Value)
	for i := 0; i < 6; i++ {
		a.Update(right)
	}
	assert.Equal(t, "0.3", st.f.Fields[0].Value, "clamps at the last preset")

	a.Update(keyMsg("down"))
	assert.Contains(t, ansi.Strip(strings.Join(a.renderForm(), "\n")), "1 … 6")
	a.Update(right)
	a.Update(right)
	assert.Equal(t, "6", st.f.Fields[1].Value, "numeric slider clamps at Max")
}

func TestHeaderInfoWideReplacesSidebarWhenHidden(t *testing.T) {
	a := New(Config{
		Theme: DefaultTheme(), Title: "app", ModelName: "m1", ShowHeader: true,
		HeaderInfo:     func() string { return "short" },
		HeaderInfoWide: func() string { return "t=0.7 · rep=1.10 · max=2048" },
		InfoRows:       func() [][2]string { return [][2]string{{"temperature", "0.7"}} },
	})
	a.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	a.initDone = true

	visible := ansi.Strip(a.View().Content)
	assert.Contains(t, visible, "short")
	assert.NotContains(t, visible, "rep=1.10", "sidebar shows the details, header stays compact")
	assert.Contains(t, visible, "temperature")

	a.ToggleSidebar()
	hidden := ansi.Strip(a.View().Content)
	assert.Contains(t, hidden, "t=0.7 · rep=1.10 · max=2048")
	assert.NotContains(t, hidden, "short ·")
	for _, line := range strings.Split(hidden, "\n") {
		assert.LessOrEqual(t, ansi.StringWidth(line), 140)
	}

	// Narrow terminal: no sidebar fits, so the wide info is used and truncated, not wrapped.
	n := New(Config{Theme: DefaultTheme(), Title: "app", ModelName: "m1", ShowHeader: true, WorkDir: "/very/long/working/directory/path/here",
		HeaderInfoWide: func() string { return strings.Repeat("x", 80) }})
	n.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	n.initDone = true
	first := strings.Split(ansi.Strip(n.View().Content), "\n")[0]
	assert.LessOrEqual(t, ansi.StringWidth(first), 70)
	assert.Contains(t, first, "app")
}

func TestRunShellCommand_TruncatesLargeOutput(t *testing.T) {
	out, err := runShellCommand("", "head -c 200000 /dev/zero | tr '\\0' x")
	require.NoError(t, err)
	assert.Contains(t, out, "output truncated")
	assert.Less(t, len(out), maxShellOutput+200)
}
