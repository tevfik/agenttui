package agenttui

import (
	"context"
	"sync"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLLM answers each prompt with a partial chunk, then waits for release(prompt).
type fakeLLM struct {
	mu        sync.Mutex
	prompts   []string
	cancelled []string
	gates     map[string]chan struct{}
}

func newFakeLLM() *fakeLLM { return &fakeLLM{gates: map[string]chan struct{}{}} }

func (f *fakeLLM) gate(p string) chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gates[p] == nil {
		f.gates[p] = make(chan struct{})
	}
	return f.gates[p]
}

func (f *fakeLLM) release(p string) { close(f.gate(p)) }

func (f *fakeLLM) stream(ctx context.Context, p string, emit func(string)) (int, error) {
	f.mu.Lock()
	f.prompts = append(f.prompts, p)
	f.mu.Unlock()
	emit("partial:" + p)
	select {
	case <-f.gate(p):
		emit(" done:" + p)
		return 5, nil
	case <-ctx.Done():
		f.mu.Lock()
		f.cancelled = append(f.cancelled, p)
		f.mu.Unlock()
		return 0, ctx.Err()
	}
}

func (f *fakeLLM) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.prompts...)
}

// harness is a tiny bubbletea event loop for the App.
type harness struct {
	t    *testing.T
	app  *App
	llm  *fakeLLM
	msgs chan tea.Msg
}

func newHarness(t *testing.T) *harness {
	llm := newFakeLLM()
	app := newSizedApp(Config{Stream: llm.stream})
	return &harness{t: t, app: app, llm: llm, msgs: make(chan tea.Msg, 1024)}
}

func (h *harness) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	go func() {
		m := cmd()
		if b, ok := m.(tea.BatchMsg); ok {
			for _, c := range b {
				h.run(c)
			}
			return
		}
		if m != nil {
			h.msgs <- m
		}
	}()
}

func (h *harness) send(m tea.Msg) {
	_, cmd := h.app.Update(m)
	h.run(cmd)
}

func (h *harness) typeEnter(text string) {
	h.app.textarea.SetValue(text)
	h.send(tea.KeyPressMsg{Code: tea.KeyEnter})
}

func (h *harness) ctrlX(text string) {
	h.app.textarea.SetValue(text)
	h.send(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
}

// until pumps messages until cond holds (5s timeout: a hang fails the test).
func (h *harness) until(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.After(5 * time.Second)
	for !cond() {
		select {
		case m := <-h.msgs:
			if _, tick := m.(spinner.TickMsg); tick {
				continue
			}
			h.send(m)
		case <-time.After(10 * time.Millisecond):
		case <-deadline:
			h.t.Fatalf("timed out waiting for: %s (prompts %v, streaming %v, queue %v)", what, h.llm.seen(), h.app.streaming, h.app.queue)
		}
	}
}

func (h *harness) transcript() (out []string) {
	for _, m := range h.app.messages {
		out = append(out, string(m.Role)+": "+m.Content)
	}
	return
}

func TestQueueWhileStreamingSendsInOrder(t *testing.T) {
	h := newHarness(t)
	h.typeEnter("one")
	h.until("first stream", func() bool { return len(h.llm.seen()) == 1 && h.app.streamBuf.Len() > 0 })

	h.typeEnter("two")
	h.typeEnter("three")
	assert.True(t, h.app.streaming, "Enter while streaming must not cancel")
	assert.Equal(t, []string{"two", "three"}, h.app.QueuedMessages())
	assert.Contains(t, h.app.View().Content, "queued 1/2: two")

	h.llm.release("one")
	h.until("two started", func() bool { return len(h.llm.seen()) == 2 })
	assert.Equal(t, []string{"three"}, h.app.QueuedMessages())
	h.llm.release("two")
	h.until("three started", func() bool { return len(h.llm.seen()) == 3 })
	h.llm.release("three")
	h.until("idle", func() bool { return !h.app.busy() && len(h.app.messages) == 6 })

	assert.Equal(t, []string{"one", "two", "three"}, h.llm.seen())
	assert.Empty(t, h.llm.cancelled)
	assert.Empty(t, h.app.QueuedMessages())
	tr := h.transcript()
	require.Len(t, tr, 6)
	assert.Equal(t, "user: one", tr[0])
	assert.Contains(t, tr[1], "done:one")
	assert.Equal(t, "user: two", tr[2])
	assert.Contains(t, tr[3], "done:two")
	assert.Equal(t, "user: three", tr[4])
	assert.Contains(t, tr[5], "done:three")
}

func TestInterruptSendsNowAndKeepsPartial(t *testing.T) {
	h := newHarness(t)
	h.typeEnter("one")
	h.until("first stream", func() bool { return len(h.llm.seen()) == 1 && h.app.streamBuf.Len() > 0 })
	h.typeEnter("queued")

	h.ctrlX("two")
	h.until("two started", func() bool { return len(h.llm.seen()) == 2 })
	assert.Equal(t, []string{"one"}, h.llm.cancelled)
	assert.Equal(t, []string{"queued"}, h.app.QueuedMessages(), "the queue survives an interrupt")

	// the late "done" of the cancelled stream must not end the new one
	time.Sleep(50 * time.Millisecond)
	h.until("settle", func() bool { return h.app.streaming })
	assert.True(t, h.app.streaming)

	h.llm.release("two")
	h.until("queued started", func() bool { return len(h.llm.seen()) == 3 })
	h.llm.release("queued")
	h.until("idle", func() bool { return !h.app.busy() })

	assert.Equal(t, []string{"one", "two", "queued"}, h.llm.seen())
	tr := h.transcript()
	require.Len(t, tr, 6)
	assert.Equal(t, "user: one", tr[0])
	assert.Contains(t, tr[1], "partial:one")
	assert.Contains(t, tr[1], "_(interrupted)_")
	assert.Equal(t, "user: two", tr[2])
	assert.Contains(t, tr[3], "done:two")
	assert.Equal(t, "user: queued", tr[4])
}

func TestInterruptWithEmptyInputPromotesQueue(t *testing.T) {
	h := newHarness(t)
	h.typeEnter("one")
	h.until("first stream", func() bool { return len(h.llm.seen()) == 1 && h.app.streamBuf.Len() > 0 })
	h.typeEnter("next")
	h.ctrlX("")
	h.until("next started", func() bool { return len(h.llm.seen()) == 2 })
	assert.Equal(t, []string{"one"}, h.llm.cancelled)
	assert.Empty(t, h.app.QueuedMessages())
	h.llm.release("next")
	h.until("idle", func() bool { return !h.app.busy() })
}

func TestEscCancelsAndDiscardsQueue(t *testing.T) {
	h := newHarness(t)
	h.typeEnter("one")
	h.until("first stream", func() bool { return len(h.llm.seen()) == 1 && h.app.streamBuf.Len() > 0 })
	h.typeEnter("two")
	h.send(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.False(t, h.app.streaming)
	assert.Empty(t, h.app.QueuedMessages())
	tr := h.transcript()
	assert.Contains(t, tr[1], "_(cancelled)_")
	assert.Contains(t, tr[2], "1 queued message(s) discarded")
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, []string{"one"}, h.llm.seen(), "discarded message must not be sent")
}

func TestInterruptWhenIdleJustSends(t *testing.T) {
	h := newHarness(t)
	h.ctrlX("hello")
	h.until("sent", func() bool { return len(h.llm.seen()) == 1 })
	h.llm.release("hello")
	h.until("idle", func() bool { return !h.app.busy() })
}
