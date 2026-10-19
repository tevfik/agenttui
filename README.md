# AgentTUI ⚡

A modern, responsive, and extensible Terminal User Interface (TUI) engine built with [Bubble Tea v2](https://github.com/charmbracelet/bubbletea), [Lipgloss v2](https://github.com/charmbracelet/lipgloss), and [Glamour](https://github.com/charmbracelet/glamour). Specifically engineered for AI Coding Assistants, LLM Chat CLIs, and Autonomous Agents.

[![Go Reference](https://pkg.go.dev/badge/github.com/tevfik/agenttui.svg)](https://pkg.go.dev/github.com/tevfik/agenttui)
[![Go Report Card](https://goreportcard.com/badge/github.com/tevfik/agenttui)](https://goreportcard.com/report/github.com/tevfik/agenttui)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

---

## ✨ Features

- 🎯 **Slash Commands & Autocomplete:** Floating command popup when typing `/`, arrow key navigation, tab completion, and argument usage hints.
- 📎 **`@file` & `@mention` Completion:** Instant fuzzy file autocomplete when typing `@` with rank prioritization and automatic XML attachment injection.
- 🌊 **Streaming Markdown & Spinner:** Real-time token streaming with syntax highlighting, bullet points, headers, and thinking spinners without flickering.
- 🎨 **Theme Engine:** Anthropic Claude Code aesthetic (warm terracotta & peach) and modern deep Devran aesthetic (violet & cyan) out of the box.
- 📐 **Responsive Sidebar & Split View:** Adaptive layout automatically shows a rich context panel (modes, tools, stats, tokens) on wide screens and collapses gracefully on narrow viewports.
- 📜 **Proportional Viewport Scrollbar:** Smooth vertical scrollbar track with dynamic thumb sizing.
- 🛡️ **Table-Safe Turn Banners:** Horizontal divider banners (`── ❯ You · 15:04 ──` and `◈ <Title> · 15:04`) that never corrupt markdown tables, code blocks, or ANSI codes.
- 🔔 **Toast Notifications & Statuses:** Non-intrusive status alerts for clipboard actions, mode changes, and errors.
- ⚡ **Instant Shell Escapes:** Run commands directly in chat via `!cmd` without interrupting conversational history.

- ❓ **Ask / Approval Prompts:** `app.Ask(ctx, agenttui.Question{...})` blocks a handler until the user picks an option or types an answer.
- 🧩 **Typed Event Stream:** `Config.StreamEvents` reports text, thinking and tool start/end as separate events (no `<think>` tag parsing).
- 💾 **Session Store & Retry:** optional `Config.Store` (Load/Save) for resume; `/retry` regenerates the last answer.

---

## 🚀 Installation

```bash
go get github.com/tevfik/agenttui
```

---

## 🛠️ Quickstart

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/tevfik/agenttui"
)

func main() {
	app := agenttui.New(agenttui.Config{
		Title:     "My Agent",
		ModelName: "qwen3.5:9b",
		Theme:     agenttui.ClaudeTheme(),
		Stream: func(ctx context.Context, prompt string, emit func(chunk string)) (int, error) {
			// Simulate LLM streaming
			emit("Thinking about: " + prompt + "\n\n")
			for i := 1; i <= 3; i++ {
				time.Sleep(100 * time.Millisecond)
				emit(fmt.Sprintf("- Point %d\n", i))
			}
			return 42, nil // returns tokens used
		},
	})

	// Register slash commands
	app.RegisterCommand(agenttui.SlashCommand{
		Name:        "clear",
		Description: "Clear conversation transcript",
		Handler: func(ctx context.Context, args []string, a *agenttui.App) (string, bool, error) {
			a.ClearMessages()
			return "Transcript cleared.", true, nil
		},
	})

	ctx := context.Background()
	if err := app.Run(ctx); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
```

---

## ⌨️ Keyboard Shortcuts

| Key | Action |
|---|---|
| `Enter` | Submit prompt / Execute selected slash command |
| `Shift+Enter` / `Alt+Enter` | Insert new line |
| `Tab` | Autocomplete selected command or `@file` |
| `↑` / `↓` | Navigate command popup or browse command history |
| `Ctrl+Y` | Copy latest assistant response to system clipboard |
| `Ctrl+M` | Toggle mouse capture (switch between wheel scroll & native terminal selection) |
| `Ctrl+C` / `Esc` | Cancel streaming response or clear input |

---

## 📄 License

MIT License. See [LICENSE](LICENSE) for details.


## Agent-facing API

```go
app := agenttui.New(agenttui.Config{
	Title: "My Agent",
	StreamEvents: func(ctx context.Context, prompt string, emit func(agenttui.Event)) (int, error) {
		emit(agenttui.Event{Kind: agenttui.EventThinking, Text: "checking the repo…"})
		emit(agenttui.Event{Kind: agenttui.EventToolStart, ToolID: "1", ToolName: "shell", ToolArgs: "ls"})
		ans, err := app.Ask(ctx, agenttui.Question{Text: "Run `rm -rf build`?", Options: []string{"yes", "no"}, Default: "no"})
		emit(agenttui.Event{Kind: agenttui.EventToolEnd, ToolID: "1", ToolOutput: ans, ToolOK: err == nil})
		emit(agenttui.Event{Kind: agenttui.EventText, Text: "Done."})
		return 0, nil
	},
})
```

`Ask` returns `ErrAskCancelled` on Esc/Ctrl+C and `ErrNotRunning` before `Run`. Implement `Store` to persist the transcript between runs.

## Keyboard shortcuts & tmux

| Keys | Action |
|---|---|
| Ctrl+←/→ (or Alt+←/→) | move by word |
| Ctrl+Backspace / Alt+Backspace / Ctrl+W | delete word backward |
| Ctrl+Delete / Alt+D | delete word forward |
| Home/End, Ctrl+A/E | line start / end |
| Ctrl+Home/End | input start / end |
| Ctrl+U / Ctrl+K | delete to line start / end |
| Ctrl+B or F2 | toggle sidebar |

Works inside tmux/screen without touching their configuration: legacy `ESC[5D`/`ESC[5C` Ctrl+arrow sequences are translated, and the footer shows **F2** instead of Ctrl+B there (Ctrl+B is tmux's prefix). Limits that only the multiplexer config can lift: without tmux's `extended-keys`, Ctrl+Backspace is indistinguishable from Backspace — use Alt+Backspace or Ctrl+W; and tmux's default `escape-time` delays a lone Esc.

### Settings forms and long pick lists
`app.Form(ctx, agenttui.Form{Title, Fields})` shows an inline settings panel (choice, number, bool and text fields) and returns the edited values on Enter, or `ErrAskCancelled` on Esc. `Ask` windows long option lists (8 visible, scrolling).
