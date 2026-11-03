package agenttui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func numberedLines(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "row-%04d lorem ipsum\n\n", i)
	}
	return sb.String()
}

func chunk(a *App, s string) { a.Update(streamChunkMsg{chunk: s}) }

func streamingApp() *App {
	a := newSizedApp(Config{Stream: func(ctx context.Context, p string, emit func(string)) (int, error) { return 0, nil }})
	a.streaming = true
	a.streamCh = make(chan tea.Msg, 1)
	return a
}

func press(a *App, code rune, mod tea.KeyMod) { a.Update(tea.KeyPressMsg{Code: code, Mod: mod}) }

// flush lets the render throttle redraw pending chunks (as a spinner tick would).
func flush(a *App) {
	a.lastStreamDraw = time.Time{}
	a.Update(a.spinner.Tick())
}

func TestScroll_FollowsWhileStreamingAndPausesWhenScrolledUp(t *testing.T) {
	a := streamingApp()
	chunk(a, numberedLines(200))
	require.True(t, a.viewport.AtBottom(), "follows the bottom while streaming")

	press(a, tea.KeyPgUp, 0)
	off := a.viewport.YOffset()
	require.False(t, a.viewport.AtBottom())
	require.True(t, a.scrollAway)

	chunk(a, numberedLines(50))
	flush(a)
	assert.Equal(t, off, a.viewport.YOffset(), "new content must not move the view")
	assert.True(t, a.unseen)
	assert.Contains(t, a.View().Content, "new content")

	press(a, tea.KeyEnd, 0) // empty input: End = bottom
	assert.True(t, a.viewport.AtBottom())
	assert.False(t, a.scrollAway)
	assert.False(t, a.unseen)
	chunk(a, numberedLines(50))
	flush(a)
	assert.True(t, a.viewport.AtBottom(), "following resumed")
	assert.NotContains(t, a.View().Content, "new content")
}

func TestScroll_Keys(t *testing.T) {
	a := streamingApp()
	chunk(a, numberedLines(300))
	h := a.viewport.Height()

	bottom := a.viewport.YOffset()
	press(a, tea.KeyPgUp, 0)
	assert.Equal(t, h, bottom-a.viewport.YOffset(), "PgUp moves a page")
	press(a, tea.KeyHome, tea.ModCtrl)
	assert.True(t, a.viewport.AtTop())
	press(a, tea.KeyDown, 0) // busy + empty input: scrolls a line
	assert.Equal(t, 1, a.viewport.YOffset())
	press(a, tea.KeyUp, 0)
	assert.Equal(t, 0, a.viewport.YOffset())
	press(a, tea.KeyPgDown, 0)
	assert.Equal(t, h, a.viewport.YOffset())
	press(a, tea.KeyDown, tea.ModAlt)
	press(a, tea.KeyUp, tea.ModAlt)
	press(a, tea.KeyEnd, tea.ModCtrl)
	assert.True(t, a.viewport.AtBottom())
	assert.False(t, a.scrollAway)
	press(a, tea.KeyHome, 0)
	assert.True(t, a.viewport.AtTop())
	assert.True(t, a.scrollAway)

	// with text in the input, Home/End/Up/Down belong to the input
	a.textarea.SetValue("hello")
	press(a, tea.KeyEnd, 0)
	assert.True(t, a.viewport.AtTop())
}

func TestScroll_Wheel(t *testing.T) {
	a := streamingApp()
	a.mouseEnabled = true
	chunk(a, numberedLines(300))
	a.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.True(t, a.scrollAway)
	for i := 0; i < 500 && a.scrollAway; i++ {
		a.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	}
	assert.False(t, a.scrollAway, "wheel back to the bottom re-enables follow")
}

func TestScroll_ResizeKeepsFollowAndPosition(t *testing.T) {
	a := streamingApp()
	chunk(a, numberedLines(300))
	a.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	assert.True(t, a.viewport.AtBottom(), "following view stays at the bottom after resize")

	press(a, tea.KeyHome, tea.ModCtrl)
	press(a, tea.KeyPgDown, 0)
	press(a, tea.KeyPgDown, 0)
	before := a.viewport.YOffset()
	total := a.viewport.TotalLineCount()
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	assert.True(t, a.scrollAway, "resize must not snap to the bottom")
	assert.False(t, a.viewport.AtBottom())
	assert.Equal(t, int(float64(before)*float64(a.viewport.TotalLineCount())/float64(total)+0.5), a.viewport.YOffset())
}

func TestScroll_ThrottledStreamingStaysFastAndFlushes(t *testing.T) {
	a := streamingApp()
	start := time.Now()
	for i := 0; i < 1500; i++ {
		chunk(a, fmt.Sprintf("row-%04d lorem ipsum\n\n", i))
	}
	assert.Less(t, time.Since(start), 5*time.Second, "1500 chunks used to take ~2 minutes (O(n^2) re-render)")
	flush(a)
	assert.Contains(t, a.viewport.GetContent(), "row-1499")
}

func TestScroll_QueueAndInterruptDoNotJumpWhenReading(t *testing.T) {
	h := newHarness(t)
	h.typeEnter("one")
	h.until("stream", func() bool { return len(h.llm.seen()) == 1 && h.app.streamBuf.Len() > 0 })
	h.app.streamMu.Lock()
	h.app.streamBuf.WriteString("\n\n" + numberedLines(200))
	h.app.streamMu.Unlock()
	h.send(streamChunkMsg{chunk: "x"})
	flush(h.app)
	press(h.app, tea.KeyHome, tea.ModCtrl)
	assert.True(t, h.app.scrollAway)

	h.typeEnter("later") // queued while reading
	assert.True(t, h.app.viewport.AtTop(), "queueing must not move the view")
	h.llm.release("one")
	h.until("queued started", func() bool { return len(h.llm.seen()) == 2 })
	assert.True(t, h.app.viewport.AtTop(), "response end + queue drain must not move the view")
	h.llm.release("later")
	h.until("idle", func() bool { return !h.app.busy() })

	// sending a message from the input jumps to the bottom and follows again
	h.typeEnter("three")
	assert.False(t, h.app.scrollAway)
	assert.True(t, h.app.viewport.AtBottom())
	h.ctrlX("four") // interrupt: also follows
	assert.True(t, h.app.viewport.AtBottom())
	h.llm.release("three")
	h.llm.release("four")
	h.until("idle", func() bool { return !h.app.busy() })
}

func TestScroll_CancelWhileReadingKeepsPosition(t *testing.T) {
	a := streamingApp()
	chunk(a, numberedLines(200))
	press(a, tea.KeyHome, tea.ModCtrl)
	a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.False(t, a.streaming)
	assert.True(t, a.viewport.AtTop())
}

func TestWrap_NothingIsCutOff(t *testing.T) {
	a := newSizedApp(Config{})
	long := strings.Repeat("W", 300)
	code := "```\n" + strings.Repeat("x", 250) + "\n```"
	wide := strings.Repeat("日本語🙂", 60)
	a.AddMessage(Message{Role: RoleUser, Content: long})
	a.AddMessage(Message{Role: RoleAssistant, Content: code + "\n\n" + wide})
	a.AddMessage(Message{Role: RoleError, Content: long})
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	content := ansi.Strip(a.viewport.GetContent())
	maxW := 0
	for _, l := range strings.Split(content, "\n") {
		maxW = max(maxW, ansi.StringWidth(l))
	}
	assert.LessOrEqual(t, maxW, a.viewport.Width(), "no line wider than the viewport")
	flat := strings.NewReplacer("\n", "", " ", "").Replace(content)
	assert.Equal(t, 2*300+250, strings.Count(flat, "W")+strings.Count(flat, "x"), "all W and x survive the wrapping")
	assert.Equal(t, 60, strings.Count(flat, "日")+0)
	assert.Equal(t, 60, strings.Count(flat, "🙂"))
}

func TestCopy_OSC52AndNotification(t *testing.T) {
	t.Setenv("SSH_TTY", "/dev/pts/1") // no local tool route
	t.Setenv("TMUX", "")
	cmd, out := copyText("héllo **md**")
	require.NoError(t, out.Err)
	assert.Equal(t, "osc52", out.Method)
	assert.Equal(t, 12, out.Chars, "chars, not bytes")
	require.NotNil(t, cmd)
}

func TestCopy_TmuxWrapsPassthrough(t *testing.T) {
	t.Setenv("SSH_TTY", "1")
	t.Setenv("TMUX", "/tmp/tmux-1/default,1,0")
	cmd, _ := copyText("abc")
	require.NotNil(t, cmd)
}

func TestCopy_LargeTextFallsBackToFile(t *testing.T) {
	t.Setenv("SSH_TTY", "")
	t.Setenv("SSH_CONNECTION", "")
	old := nativeCopy
	nativeCopy = func(string) (string, bool) { return "", false }
	defer func() { nativeCopy = old }()
	big := strings.Repeat("a", maxOSC52+1)
	cmd, out := copyText(big)
	assert.Nil(t, cmd, "no truncated OSC 52 is emitted")
	require.NotEmpty(t, out.Path)
	defer os.Remove(out.Path)
	b, err := os.ReadFile(out.Path)
	require.NoError(t, err)
	assert.Equal(t, big, string(b))

	a := newSizedApp(Config{})
	a.copyAndNotify(big, "response")
	assert.Contains(t, a.notification.Message, "too large")
	os.Remove(out.Path)
}

func TestCopy_LargeTextUsesNativeToolWhenAvailable(t *testing.T) {
	t.Setenv("SSH_TTY", "")
	t.Setenv("SSH_CONNECTION", "")
	var got string
	old := nativeCopy
	nativeCopy = func(s string) (string, bool) { got = s; return "wl-copy", true }
	defer func() { nativeCopy = old }()
	big := strings.Repeat("a", 500000)
	cmd, out := copyText(big)
	assert.Nil(t, cmd)
	assert.Equal(t, "wl-copy", out.Method)
	assert.Equal(t, big, got, "the full text, untruncated")
	assert.Empty(t, out.Path)
}

func TestCopy_EmptyReportsError(t *testing.T) {
	a := newSizedApp(Config{})
	a.copyAndNotify("", "x")
	assert.Equal(t, NotifyError, a.notification.Type)
}

func TestCopy_CtrlYIsRawMarkdownAndWorksWhileStreaming(t *testing.T) {
	t.Setenv("SSH_TTY", "1")
	a := newSizedApp(Config{})
	a.AddMessage(Message{Role: RoleAssistant, Content: "# Title\n\n```go\nfmt.Println(1)\n```\n\n_(cancelled)_"})
	src, ok := a.copySource()
	require.True(t, ok)
	assert.Contains(t, src, "# Title", "raw markdown, not the rendered/ANSI text")
	assert.NotContains(t, src, "\x1b")

	a.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	require.NotNil(t, a.notification)
	assert.Contains(t, a.notification.Message, "Copied response")
	assert.Contains(t, a.notification.Message, "chars")

	a.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModAlt})
	assert.Contains(t, a.notification.Message, "code blocks")

	// while streaming: the partial answer is copied
	a.streaming = true
	a.streamBuf.WriteString("partial **answer**")
	src, ok = a.copySource()
	require.True(t, ok)
	assert.Equal(t, "partial **answer**", src)
	a.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	assert.Contains(t, a.notification.Message, "18 chars")
}

func TestCopy_AltYWithoutCodeBlock(t *testing.T) {
	a := newSizedApp(Config{})
	a.AddMessage(Message{Role: RoleAssistant, Content: "plain text"})
	a.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModAlt})
	assert.Contains(t, a.notification.Message, "No code block")
}

func TestExtractCodeBlocks_TildeFences(t *testing.T) {
	assert.Equal(t, "a\nb", ExtractCodeBlocks("x\n~~~\na\nb\n~~~\ny"))
}
