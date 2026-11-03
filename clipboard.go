package agenttui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// maxOSC52 is the largest text sent through an OSC 52 escape sequence. Many
// terminals drop (or truncate) sequences above ~100000 bytes, and base64 adds a
// third, so stay safely below.
const maxOSC52 = 70000

// nativeCopy is the system clipboard writer (replaced in tests).
var nativeCopy = copyToClipboardTool

// copyToClipboard attempts to copy text to the system clipboard using
// available clipboard tools (xclip, xsel, wl-copy, pbcopy).
func copyToClipboard(text string) bool {
	_, ok := copyToClipboardTool(text)
	return ok
}

// copyToClipboardTool is copyToClipboard that also reports the tool used.
func copyToClipboardTool(text string) (string, bool) {
	tools := []struct {
		name string
		args []string
	}{
		{"xclip", []string{"-selection", "clipboard"}},
		{"xsel", []string{"--clipboard", "--input"}},
		{"wl-copy", nil},
		{"pbcopy", nil}, // macOS
	}

	for _, tool := range tools {
		path, err := exec.LookPath(tool.name)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		cmd := exec.CommandContext(ctx, path, tool.args...)
		cmd.Stdin = strings.NewReader(text)
		err = cmd.Run()
		cancel()
		if err == nil {
			return tool.name, true
		}
	}
	return "", false
}

func inSSH() bool {
	return os.Getenv("SSH_TTY") != "" || os.Getenv("SSH_CONNECTION") != ""
}

// copyOutcome describes how a copy ended, for tests and the notification.
type copyOutcome struct {
	Chars  int
	Method string // tool name, "osc52", "" when nothing worked
	Path   string // fallback file when the text was too large for OSC 52
	Err    error
}

// copyText copies text through every route that can work: the local clipboard
// tool (skipped over SSH, where it would only fill the server's clipboard) and
// an OSC 52 escape sequence (works over SSH and in tmux). Text too large for
// OSC 52 with no local tool is saved to a file instead. It returns the command
// that emits the escape sequence, if any, and the outcome.
func copyText(text string) (tea.Cmd, copyOutcome) {
	out := copyOutcome{Chars: utf8.RuneCountInString(text)}
	if text == "" {
		out.Err = fmt.Errorf("nothing to copy")
		return nil, out
	}
	if !inSSH() {
		if tool, ok := nativeCopy(text); ok {
			out.Method = tool
		}
	}
	var cmd tea.Cmd
	if len(text) <= maxOSC52 {
		seq := ansi.SetSystemClipboard(text)
		if os.Getenv("TMUX") != "" {
			// tmux ignores bare OSC 52 unless set-clipboard is "on"; the
			// passthrough form needs allow-passthrough. Send both.
			seq += ansi.TmuxPassthrough(seq)
		}
		cmd = tea.Raw(seq)
		if out.Method == "" {
			out.Method = "osc52"
		}
		return cmd, out
	}
	if out.Method != "" {
		return nil, out
	}
	// Too large for OSC 52 and no clipboard tool: keep the text in a file.
	if tool, ok := nativeCopy(text); ok { // SSH with X forwarding etc.
		out.Method = tool
		return nil, out
	}
	f, err := os.CreateTemp("", "agenttui-copy-*.txt")
	if err != nil {
		out.Err = err
		return nil, out
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		out.Err = err
		return nil, out
	}
	out.Path, _ = filepath.Abs(f.Name())
	return nil, out
}

// copyAndNotify copies text and returns the command batch that performs the
// copy and shows the result notification.
func (a *App) copyAndNotify(text, what string) tea.Cmd {
	cmd, out := copyText(text)
	var note tea.Cmd
	switch {
	case out.Err != nil:
		note = a.SetNotification(fmt.Sprintf("⚠ Copy failed: %v", out.Err), NotifyError)
	case out.Path != "":
		note = a.SetNotification(fmt.Sprintf("⚠ %d chars is too large for the terminal clipboard; saved to %s", out.Chars, out.Path), NotifyWarning)
	case out.Method == "osc52":
		note = a.SetNotification(fmt.Sprintf("✓ Copied %s: %d chars (terminal clipboard, OSC 52)", what, out.Chars), NotifySuccess)
	default:
		note = a.SetNotification(fmt.Sprintf("✓ Copied %s: %d chars (%s)", what, out.Chars, out.Method), NotifySuccess)
	}
	return tea.Batch(cmd, note)
}

// ExtractCodeBlocks extracts the raw code within markdown fenced code blocks (```...```).
// If no code fences are found, the original text is returned.
func ExtractCodeBlocks(text string) string {
	lines := strings.Split(text, "\n")
	var blocks []string
	var curBlock strings.Builder
	inBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if inBlock {
				blocks = append(blocks, strings.TrimRight(curBlock.String(), "\n"))
				curBlock.Reset()
				inBlock = false
			} else {
				inBlock = true
			}
			continue
		}
		if inBlock {
			curBlock.WriteString(line + "\n")
		}
	}
	if inBlock && curBlock.Len() > 0 {
		blocks = append(blocks, strings.TrimRight(curBlock.String(), "\n"))
	}
	if len(blocks) == 0 {
		return text
	}
	return strings.Join(blocks, "\n\n")
}

// hasCodeFence reports whether text contains a fenced code block.
func hasCodeFence(text string) bool {
	for _, l := range strings.Split(text, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			return true
		}
	}
	return false
}

// CopyToClipboard copies arbitrary text to the system clipboard.
func (a *App) CopyToClipboard(text string) bool {
	return copyToClipboard(text)
}

// copySource returns the raw markdown to copy: the answer being streamed, or
// else the latest assistant message.
func (a *App) copySource() (string, bool) {
	if a.streaming {
		a.streamMu.Lock()
		partial := a.streamBuf.String()
		a.streamMu.Unlock()
		if _, content := extractThinkingTags(partial); strings.TrimSpace(content) != "" {
			return strings.TrimSpace(content), true
		}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for i := len(a.messages) - 1; i >= 0; i-- {
		if a.messages[i].Role == RoleAssistant && strings.TrimSpace(a.messages[i].Content) != "" {
			c := a.messages[i].Content
			for _, mark := range []string{"cancelled", "interrupted"} {
				c = strings.TrimSuffix(c, "\n\n_("+mark+")_") // our own marker, not the model's text
			}
			return c, true
		}
	}
	return "", false
}
