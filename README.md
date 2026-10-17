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
- 🛡️ **Table-Safe Turn Banners:** Horizontal divider banners (`── ❯ You · 15:04 ──` and `◈ Devran · 15:04`) that never corrupt markdown tables, code blocks, or ANSI codes.
- 🔔 **Toast Notifications & Statuses:** Non-intrusive status alerts for clipboard actions, mode changes, and errors.
- ⚡ **Instant Shell Escapes:** Run commands directly in chat via `!cmd` without interrupting conversational history.

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
		Title:     "Devran",
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
