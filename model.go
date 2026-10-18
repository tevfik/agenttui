package agenttui

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
)

type initDoneMsg struct{}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	a.spinner.Style = lipgloss.NewStyle().Foreground(a.cfg.Theme.Secondary)

	return tea.Batch(
		textarea.Blink,
		a.spinner.Tick,
		tea.Tick(800*time.Millisecond, func(time.Time) tea.Msg { return initDoneMsg{} }),
	)
}

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.recalcLayout()
		return a, nil

	case initDoneMsg:
		a.initDone = true
		a.textarea.Reset()
		v := a.textarea.Value()
		if strings.Contains(v, ";rgb:") || strings.Contains(v, "\x1b") || strings.Contains(v, "11;") {
			a.textarea.SetValue("")
		}
		a.recalcViewport()
		a.viewport.SetContent(a.renderMessages())
		a.viewport.GotoBottom()
		return a, nil

	case notifyExpireMsg:
		if a.notification != nil && time.Now().After(a.notification.Expires) {
			a.notification = nil
		}
		return a, nil

	case tea.MouseWheelMsg:
		if !a.mouseEnabled {
			return a, nil
		}
		mouse := msg.Mouse()
		if mouse.Button == tea.MouseWheelUp {
			a.viewport.ScrollUp(3)
		} else if mouse.Button == tea.MouseWheelDown {
			a.viewport.ScrollDown(3)
		}
		return a, nil

	case tea.MouseClickMsg, tea.MouseReleaseMsg, tea.MouseMotionMsg:
		if !a.mouseEnabled {
			return a, nil
		}
		if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseLeft && a.hasThinkingMessages() {
			return a.toggleThinking()
		}
		var vcmd tea.Cmd
		a.viewport, vcmd = a.viewport.Update(msg)
		return a, vcmd

	case tea.KeyPressMsg:
		keyStr := msg.String()
		if strings.Contains(keyStr, ";rgb:") || strings.Contains(keyStr, "\x1b") ||
			strings.Contains(keyStr, "[?") || strings.Contains(keyStr, ";2c") ||
			strings.Contains(keyStr, "?1;") || strings.Contains(keyStr, ">0;") ||
			strings.HasPrefix(keyStr, "11;") || strings.ContainsAny(keyStr, "\x00\x07\x1b") {
			return a, nil
		}
		if !a.initDone {
			return a, nil
		}

		switch keyStr {
		case "ctrl+c":
			if a.streaming {
				a.cancelStream()
				return a, nil
			}
			a.quitting = true
			return a, tea.Quit

		case "esc":
			if a.streaming {
				a.cancelStream()
				return a, nil
			}
			if a.autoActive {
				a.autoActive = false
				a.recalcViewport()
				return a, nil
			}
			if a.notification != nil {
				a.notification = nil
				return a, nil
			}
			return a, nil

		case "ctrl+t":
			return a.toggleThinking()

		case "ctrl+m":
			a.mouseEnabled = !a.mouseEnabled
			status := "Mouse OFF (select text normally, scroll with PgUp/PgDn)"
			if a.mouseEnabled {
				status = "Mouse ON (scroll with wheel, Shift+drag to select)"
			}
			notifyCmd := a.SetNotification(status, NotifyInfo)
			return a, notifyCmd

		case "ctrl+y":
			if !a.streaming && !a.waiting {
				a.mu.RLock()
				for i := len(a.messages) - 1; i >= 0; i-- {
					if a.messages[i].Role == RoleAssistant && a.messages[i].Content != "" {
						content := a.messages[i].Content
						a.mu.RUnlock()
						if copyToClipboard(content) {
							return a, a.SetNotification("✓ Copied response to clipboard", NotifySuccess)
						}
						return a, a.SetNotification("⚠ Clipboard not available", NotifyWarning)
					}
				}
				a.mu.RUnlock()
			}
			return a, nil

		case "up":
			count := len(a.autoFiltered)
			if a.autoMode == "at" {
				count = len(a.autoAtFiles)
			}
			if a.autoActive && count > 0 {
				if a.autoCursor > 0 {
					a.autoCursor--
				}
				return a, nil
			}
			if !a.waiting && !a.streaming && len(a.history) > 0 {
				if a.historyIdx == -1 {
					a.historySave = a.textarea.Value()
					a.historyIdx = len(a.history) - 1
				} else if a.historyIdx > 0 {
					a.historyIdx--
				}
				a.textarea.SetValue(a.history[a.historyIdx])
				a.textarea.CursorEnd()
				return a, nil
			}

		case "down":
			count := len(a.autoFiltered)
			if a.autoMode == "at" {
				count = len(a.autoAtFiles)
			}
			if a.autoActive && count > 0 {
				if a.autoCursor < count-1 {
					a.autoCursor++
				}
				return a, nil
			}
			if !a.waiting && !a.streaming && a.historyIdx >= 0 {
				a.historyIdx++
				if a.historyIdx >= len(a.history) {
					a.historyIdx = -1
					a.textarea.SetValue(a.historySave)
				} else {
					a.textarea.SetValue(a.history[a.historyIdx])
				}
				a.textarea.CursorEnd()
				return a, nil
			}

		case "tab":
			if a.autoActive && a.autoMode == "at" && len(a.autoAtFiles) > 0 {
				a.completeAtFile()
				return a, nil
			}
			if a.autoActive && len(a.autoFiltered) > 0 {
				selected := a.autoFiltered[a.autoCursor]
				a.textarea.SetValue("/" + selected.Name + " ")
				a.textarea.CursorEnd()
				a.autoActive = false
				a.recalcViewport()
				return a, nil
			}
			val := strings.TrimSpace(a.textarea.Value())
			if val == "" {
				a.textarea.SetValue("/")
				a.textarea.CursorEnd()
				a.updateAutocomplete()
				a.recalcViewport()
				return a, nil
			}

		case "enter":
			if a.autoActive && a.autoMode == "at" && len(a.autoAtFiles) > 0 {
				a.completeAtFile()
				return a, nil
			}
			if a.autoActive && len(a.autoFiltered) > 0 {
				selected := a.autoFiltered[a.autoCursor]
				a.autoActive = false

				// If command requires mandatory argument (e.g. "<query>", "<file-path>"),
				// fill textarea so user can type the parameter.
				if strings.HasPrefix(strings.TrimSpace(selected.Usage), "<") {
					a.textarea.SetValue("/" + selected.Name + " ")
					a.textarea.CursorEnd()
					a.recalcViewport()
					return a, nil
				}

				// If actively streaming or waiting, cancel it first to run the slash command
				if a.streaming || a.waiting {
					a.cancelStream()
				}

				// Otherwise (no arguments or optional arguments), execute immediately!
				cmdStr := "/" + selected.Name
				a.textarea.Reset()
				a.history = append(a.history, cmdStr)
				a.historyIdx = -1
				cmd := a.handleSubmit(cmdStr)
				a.recalcViewport()
				return a, cmd
			}

			val := strings.TrimSpace(a.textarea.Value())
			if val != "" {
				// If actively streaming or waiting, cancel ongoing stream to process this new command/prompt!
				if a.streaming || a.waiting {
					a.cancelStream()
				}

				a.textarea.Reset()
				a.autoActive = false
				a.history = append(a.history, val)
				a.historyIdx = -1

				cmd := a.handleSubmit(val)
				a.recalcViewport()
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				return a, tea.Batch(cmds...)
			}
			return a, nil

		case " ", "space":
			if a.autoActive && a.autoMode == "at" && len(a.autoAtFiles) > 0 {
				a.completeAtFile()
				return a, nil
			}

		case "shift+enter", "alt+enter":
			a.textarea.InsertString("\n")
			return a, nil

		case "pgup", "ctrl+u":
			a.viewport.ScrollUp(6)
			return a, nil

		case "pgdown", "ctrl+f":
			a.viewport.ScrollDown(6)
			return a, nil
		}

	case streamChunkMsg:
		if !a.streaming {
			return a, nil
		}
		a.streamMu.Lock()
		a.streamBuf.WriteString(msg.chunk)
		a.streamMu.Unlock()

		a.viewport.SetContent(a.renderMessagesfast())
		a.viewport.GotoBottom()
		return a, waitForStream(a.streamCh)

	case streamDoneMsg:
		if !a.streaming {
			return a, nil
		}
		a.streamMu.Lock()
		finalContent := a.streamBuf.String()
		a.streamBuf.Reset()
		a.streamMu.Unlock()

		a.streaming = false
		a.waiting = false
		a.totalTokens += msg.tokens
		responseTime := time.Since(a.streamStartTime).Truncate(100 * time.Millisecond)

		if msg.err != nil {
			if finalContent != "" {
				thinking, clean := extractThinkingTags(finalContent)
				a.AddMessage(Message{Role: RoleAssistant, Content: clean, Thinking: thinking, ThinkingTime: responseTime})
			}
			a.AddMessage(Message{Role: RoleError, Content: msg.err.Error()})
			cmds = append(cmds, a.SetNotification(fmt.Sprintf("× Error after %s", responseTime), NotifyError))
		} else {
			if finalContent == "" {
				finalContent = "_No response._"
			}
			thinking, clean := extractThinkingTags(finalContent)
			if clean == "" && thinking != "" {
				clean = thinking
				thinking = ""
			}
			a.AddMessage(Message{
				Role:         RoleAssistant,
				Content:      clean,
				Thinking:     thinking,
				ThinkingTime: responseTime,
			})
			cmds = append(cmds, a.SetNotification(fmt.Sprintf("◇ %s · %s · %dtok", a.modelName, responseTime, msg.tokens), NotifyInfo))
		}

		a.cacheCount = 0 // force rebuild cache
		a.recalcViewport()
		a.viewport.SetContent(a.renderMessages())
		a.viewport.GotoBottom()
		return a, tea.Batch(cmds...)

	case streamEndMsg:
		a.streaming = false
		a.waiting = false
		a.recalcViewport()
		a.viewport.SetContent(a.renderMessages())
		a.viewport.GotoBottom()
		return a, nil

	case slashDoneMsg:
		a.waiting = false
		if msg.err != nil {
			a.AddMessage(Message{Role: RoleError, Content: msg.err.Error()})
		} else if msg.out != "" {
			if !msg.isLocal {
				// Non-local output means this is an enriched prompt (e.g. from /ask)
				// that needs to be sent to the LLM for streaming answer!
				a.cacheCount = 0
				a.recalcViewport()
				a.viewport.SetContent(a.renderMessages())
				a.viewport.GotoBottom()
				return a, a.startStreamPrompt(msg.out)
			}
			a.AddMessage(Message{Role: RoleSystem, Content: msg.out})
		}
		a.cacheCount = 0
		a.recalcViewport()
		a.viewport.SetContent(a.renderMessages())
		a.viewport.GotoBottom()
		return a, nil


	case spinner.TickMsg:
		// Only tick when actively streaming or waiting! Prevents runaway loops.
		if a.waiting || a.streaming {
			var cmd tea.Cmd
			a.spinner, cmd = a.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	// Forward to sidebar widget if present
	if a.sidebar != nil && a.sidebar.Visible() {
		var wCmd tea.Cmd
		a.sidebar, wCmd = a.sidebar.Update(msg)
		if wCmd != nil {
			cmds = append(cmds, wCmd)
		}
	}

	// Update textarea (always interactive, allows typing slash commands or redirecting prompt while streaming)
	if a.initDone {
		var taCmd tea.Cmd
		a.textarea, taCmd = a.textarea.Update(msg)
		cmds = append(cmds, taCmd)
		wasAuto := a.autoActive
		a.updateAutocomplete()
		if wasAuto != a.autoActive {
			a.recalcViewport()
		}
	}

	return a, tea.Batch(cmds...)
}

func (a *App) cancelStream() {
	if a.streamCancel != nil {
		a.streamCancel()
	}
	a.streamMu.Lock()
	partial := a.streamBuf.String()
	a.streamBuf.Reset()
	a.streamMu.Unlock()
	a.streaming = false
	a.waiting = false
	if partial != "" {
		a.AddMessage(Message{Role: RoleAssistant, Content: partial + "\n\n_(cancelled)_"})
	} else {
		a.AddMessage(Message{Role: RoleSystem, Content: "_(cancelled)_"})
	}
	a.cacheCount = 0
	a.recalcViewport()
	a.viewport.SetContent(a.renderMessages())
	a.viewport.GotoBottom()
}

func (a *App) handleSubmit(input string) tea.Cmd {
	// 1. Instant Shell Execution with '!'
	if strings.HasPrefix(input, "!") {
		cmdStr := strings.TrimSpace(input[1:])
		if cmdStr != "" {
			a.AddMessage(Message{Role: RoleUser, Content: input})
			a.waiting = true
			a.cacheCount = 0
			a.viewport.SetContent(a.renderMessages())
			a.viewport.GotoBottom()

			return tea.Batch(a.spinner.Tick, func() tea.Msg {
				out, err := runShellCommand(a.workDir, cmdStr)
				return slashDoneMsg{
					input:   input,
					out:     out,
					isLocal: true,
					err:     err,
				}
			})
		}
	}

	// 2. Slash command
	if strings.HasPrefix(input, "/") {
		parts := strings.Fields(input[1:])
		if len(parts) > 0 {
			name := strings.ToLower(parts[0])
			args := parts[1:]

			a.mu.RLock()
			cmd, exists := a.commands[name]
			a.mu.RUnlock()

			if exists {
				a.AddMessage(Message{Role: RoleUser, Content: input})
				a.waiting = true
				a.cacheCount = 0
				a.viewport.SetContent(a.renderMessages())
				a.viewport.GotoBottom()

				cmdCopy := cmd
				argsCopy := args
				inputCopy := input
				return tea.Batch(a.spinner.Tick, func() tea.Msg {
					out, isLocal, err := cmdCopy.Handler(context.Background(), argsCopy, a)
					return slashDoneMsg{
						input:   inputCopy,
						out:     out,
						isLocal: isLocal,
						err:     err,
					}
				})
			} else {
				a.AddMessage(Message{Role: RoleUser, Content: input})
				a.AddMessage(Message{
					Role:      RoleSystem,
					Content:   fmt.Sprintf("❌ Unknown slash command `/%s`. Type `/` to see available commands.", name),
					Timestamp: time.Now(),
				})
				a.cacheCount = 0
				a.viewport.SetContent(a.renderMessages())
				a.viewport.GotoBottom()
				return nil
			}
		}
	}

	// 3. Normal streaming chat prompt
	a.AddMessage(Message{Role: RoleUser, Content: input})
	a.cacheCount = 0
	a.viewport.SetContent(a.renderMessages())
	a.viewport.GotoBottom()

	expanded := a.expandFileReferences(input)
	return a.startStreamPrompt(expanded)
}

func (a *App) expandFileReferences(prompt string) string {
	words := strings.Fields(prompt)
	var attachments []string
	seen := make(map[string]bool)

	for _, w := range words {
		if strings.HasPrefix(w, "@") && len(w) > 1 {
			relPath := strings.TrimPrefix(w, "@")
			relPath = strings.TrimRight(relPath, ",.:;!?)\"'")
			if seen[relPath] {
				continue
			}
			targetPath := relPath
			if a.workDir != "" && !filepath.IsAbs(targetPath) {
				targetPath = filepath.Join(a.workDir, relPath)
			}
			if stat, err := os.Stat(targetPath); err == nil && !stat.IsDir() {
				seen[relPath] = true
				if content, err := os.ReadFile(targetPath); err == nil {
					text := string(content)
					if len(text) > 40000 {
						text = text[:40000] + "\n...(truncated)"
					}
					attachments = append(attachments, fmt.Sprintf("<file path=%q>\n%s\n</file>", relPath, text))
				}
			}
		}
	}

	if len(attachments) > 0 {
		return prompt + "\n\n" + strings.Join(attachments, "\n\n")
	}
	return prompt
}

func runShellCommand(workDir, cmdStr string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)
	if workDir != "" {
		cmd.Dir = workDir
	}
	out, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(out))
	if err != nil {
		if outStr != "" {
			return fmt.Sprintf("```bash\n$ %s\n%s\n```\n_Error: %s_", cmdStr, outStr, err), nil
		}
		return "", err
	}
	if outStr == "" {
		outStr = "(command exited with status 0 and no output)"
	}
	return fmt.Sprintf("```bash\n$ %s\n%s\n```", cmdStr, outStr), nil
}

func (a *App) startStreamPrompt(prompt string) tea.Cmd {
	if a.cfg.Stream == nil {
		a.AddMessage(Message{Role: RoleError, Content: "No StreamHandler configured."})
		a.cacheCount = 0
		a.viewport.SetContent(a.renderMessages())
		return nil
	}

	a.streaming = true
	a.waiting = false
	a.streamStartTime = time.Now()
	a.streamMu.Lock()
	a.streamBuf.Reset()
	a.streamMu.Unlock()
	a.streamCh = make(chan tea.Msg, 64)
	a.streamCtx, a.streamCancel = context.WithCancel(context.Background())

	go func(p string, ch chan tea.Msg, ctx context.Context) {
		tokens, err := a.cfg.Stream(ctx, p, func(chunk string) {
			select {
			case <-ctx.Done():
			case ch <- streamChunkMsg{chunk: chunk}:
			}
		})
		ch <- streamDoneMsg{tokens: tokens, err: err}
		close(ch)
	}(prompt, a.streamCh, a.streamCtx)

	return tea.Batch(a.spinner.Tick, waitForStream(a.streamCh))
}

func (a *App) renderTokenBar(barW int) string {
	used := a.contextUsed
	if used == 0 {
		used = a.totalTokens
	}
	max := a.contextMax
	if max == 0 {
		max = 128000
	}

	pct := float64(used) / float64(max)
	if pct > 1.0 {
		pct = 1.0
	}
	filled := int(pct * float64(barW))
	if filled > barW {
		filled = barW
	}
	empty := barW - filled

	barCol := a.cfg.Theme.Success
	if pct >= 0.85 {
		barCol = a.cfg.Theme.Error
	} else if pct >= 0.60 {
		barCol = a.cfg.Theme.Warning
	}

	barFilledStr := lipgloss.NewStyle().Foreground(barCol).Render(strings.Repeat("█", filled))
	barEmptyStr := lipgloss.NewStyle().Foreground(a.cfg.Theme.Muted).Render(strings.Repeat("░", empty))

	usedK := float64(used) / 1000.0
	maxK := float64(max) / 1000.0

	return fmt.Sprintf("[%s%s] %.1fk/%.0fk (%d%%)",
		barFilledStr, barEmptyStr, usedK, maxK, int(pct*100),
	)
}

func (a *App) updateAutocomplete() {
	val := a.textarea.Value()

	// 1. Slash command autocomplete
	if strings.HasPrefix(val, "/") && !strings.Contains(val, " ") && !strings.Contains(val, "\n") {
		a.autoMode = "slash"
		query := strings.ToLower(val[1:])
		var matches []SlashCommand
		a.mu.RLock()
		for _, cmd := range a.cmdList {
			if strings.HasPrefix(strings.ToLower(cmd.Name), query) {
				matches = append(matches, cmd)
			}
		}
		a.mu.RUnlock()

		a.autoFiltered = matches
		a.autoAtFiles = nil
		if a.autoCursor >= len(matches) {
			a.autoCursor = 0
		}
		a.autoActive = len(matches) > 0
		return
	}

	// 2. @file reference autocomplete
	lastWord := ""
	if idx := strings.LastIndexAny(val, " \n\t"); idx >= 0 {
		lastWord = val[idx+1:]
	} else {
		lastWord = val
	}

	if strings.HasPrefix(lastWord, "@") {
		a.autoMode = "at"
		query := strings.ToLower(lastWord[1:])
		a.mu.RLock()
		repoFiles := make([]string, len(a.repoFiles))
		copy(repoFiles, a.repoFiles)
		a.mu.RUnlock()

		matches := rankFileMatches(repoFiles, query)
		a.autoAtFiles = matches
		a.autoFiltered = nil
		if a.autoCursor >= len(matches) {
			a.autoCursor = 0
		}
		a.autoActive = len(matches) > 0
		return
	}

	a.autoActive = false
	a.autoFiltered = nil
	a.autoAtFiles = nil
}

func (a *App) completeAtFile() {
	if len(a.autoAtFiles) == 0 {
		return
	}
	if a.autoCursor < 0 || a.autoCursor >= len(a.autoAtFiles) {
		a.autoCursor = 0
	}
	selectedFile := a.autoAtFiles[a.autoCursor]
	curVal := a.textarea.Value()
	if idx := strings.LastIndex(curVal, "@"); idx >= 0 {
		a.textarea.SetValue(curVal[:idx] + "@" + selectedFile + " ")
		a.textarea.CursorEnd()
	}
	a.autoActive = false
	a.autoAtFiles = nil
	a.recalcViewport()
}

func rankFileMatches(repoFiles []string, query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		var rootFiles []string
		var otherFiles []string
		for _, f := range repoFiles {
			if !strings.Contains(f, "/") && !strings.Contains(f, "\\") {
				rootFiles = append(rootFiles, f)
			} else {
				otherFiles = append(otherFiles, f)
			}
		}
		res := append(rootFiles, otherFiles...)
		if len(res) > 8 {
			res = res[:8]
		}
		return res
	}

	var b1, b2, b3, b4, b5, b6 []string
	seen := make(map[string]bool)

	// B1: Exact base name match (with or without extension)
	for _, f := range repoFiles {
		base := filepath.Base(f)
		lowerBase := strings.ToLower(base)
		ext := filepath.Ext(lowerBase)
		baseNoExt := strings.TrimSuffix(lowerBase, ext)

		if lowerBase == query || baseNoExt == query {
			b1 = append(b1, f)
			seen[f] = true
		}
	}

	// B2: Basename starts with query (prioritizes TEST_PLAN.md for "test")
	for _, f := range repoFiles {
		if seen[f] {
			continue
		}
		base := filepath.Base(f)
		lowerBase := strings.ToLower(base)
		if strings.HasPrefix(lowerBase, query) {
			b2 = append(b2, f)
			seen[f] = true
		}
	}

	// B3: Path starts with query (e.g. "pkg/", "internal/")
	for _, f := range repoFiles {
		if seen[f] {
			continue
		}
		lowerPath := strings.ToLower(f)
		if strings.HasPrefix(lowerPath, query) {
			b3 = append(b3, f)
			seen[f] = true
		}
	}

	// B4: Basename contains query after a separator (_test, -test, .test)
	for _, f := range repoFiles {
		if seen[f] {
			continue
		}
		base := filepath.Base(f)
		lowerBase := strings.ToLower(base)
		if strings.Contains(lowerBase, "_"+query) || strings.Contains(lowerBase, "-"+query) || strings.Contains(lowerBase, "."+query) {
			b4 = append(b4, f)
			seen[f] = true
		}
	}

	// B5: Basename contains query anywhere
	for _, f := range repoFiles {
		if seen[f] {
			continue
		}
		base := filepath.Base(f)
		lowerBase := strings.ToLower(base)
		if strings.Contains(lowerBase, query) {
			b5 = append(b5, f)
			seen[f] = true
		}
	}

	// B6: Full path contains query anywhere
	for _, f := range repoFiles {
		if seen[f] {
			continue
		}
		lowerPath := strings.ToLower(f)
		if strings.Contains(lowerPath, query) {
			b6 = append(b6, f)
			seen[f] = true
		}
	}

	sortBucket := func(bucket []string) {
		sort.Slice(bucket, func(i, j int) bool {
			slashesI := strings.Count(bucket[i], "/") + strings.Count(bucket[i], "\\")
			slashesJ := strings.Count(bucket[j], "/") + strings.Count(bucket[j], "\\")
			if slashesI != slashesJ {
				return slashesI < slashesJ
			}
			if len(bucket[i]) != len(bucket[j]) {
				return len(bucket[i]) < len(bucket[j])
			}
			return bucket[i] < bucket[j]
		})
	}

	sortBucket(b1)
	sortBucket(b2)
	sortBucket(b3)
	sortBucket(b4)
	sortBucket(b5)
	sortBucket(b6)

	var result []string
	for _, b := range [][]string{b1, b2, b3, b4, b5, b6} {
		for _, item := range b {
			result = append(result, item)
			if len(result) >= 8 {
				return result
			}
		}
	}
	return result
}

func (a *App) chromeHeight() int {
	h := 0
	if a.cfg.ShowHeader {
		h += 2 // header + sep line
	}
	if a.waiting || a.streaming {
		h++ // status indicator
	}
	if a.autoActive && (len(a.autoFiltered) > 0 || len(a.autoAtFiles) > 0) {
		popup := a.renderAutocomplete()
		h += strings.Count(popup, "\n") + 1
	}
	// Textarea is height 3 + top border + bottom border = 5 lines
	h += 5
	if a.cfg.ShowFooter {
		h++ // footer
	}
	return h
}

// sidebarWidthFor calculates the optimal sidebar width based on terminal width.
// - < 100 cols: 0 (disabled to allow full breathing room for chat text)
// - 100-139 cols: 24 (balanced compact sidebar)
// - >= 140 cols: 30 (spacious sidebar)
func sidebarWidthFor(terminalWidth int) int {
	if terminalWidth < 100 {
		return 0
	}
	if terminalWidth < 140 {
		return 24
	}
	return 30
}

func (a *App) recalcViewport() {
	if !a.ready || a.height == 0 {
		return
	}

	sidebarW := sidebarWidthFor(a.width)
	vpW := a.width
	if sidebarW > 0 {
		vpW = a.width - sidebarW
	}
	if vpW < 40 {
		vpW = 40
	}

	chrome := a.chromeHeight()
	vpH := a.height - chrome
	if vpH < 4 {
		vpH = 4
	}

	a.viewport.SetWidth(vpW - 1)
	a.viewport.SetHeight(vpH)
	taWidth := vpW - 2
	if taWidth < 20 {
		taWidth = 20
	}
	a.textarea.SetWidth(taWidth)
}

func (a *App) recalcLayout() {
	if a.width == 0 || a.height == 0 {
		return
	}

	sidebarW := sidebarWidthFor(a.width)
	vpW := a.width
	if sidebarW > 0 {
		vpW = a.width - sidebarW
	}
	if vpW < 40 {
		vpW = 40
	}

	chrome := a.chromeHeight()
	vpH := a.height - chrome
	if vpH < 4 {
		vpH = 4
	}

	if !a.ready {
		a.viewport = viewport.New(viewport.WithWidth(vpW-1), viewport.WithHeight(vpH))
		a.viewport.Style = lipgloss.NewStyle().Padding(0, 1)
		a.ready = true
	} else {
		a.viewport.SetWidth(vpW - 1)
		a.viewport.SetHeight(vpH)
	}

	taWidth := vpW - 2
	if taWidth < 20 {
		taWidth = 20
	}
	a.textarea.SetWidth(taWidth)

	if a.renderer == nil || a.renderW != vpW {
		a.renderW = vpW
		r, _ := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(vpW-6),
		)
		a.renderer = r
		a.cacheCount = 0
		a.renderedCache = ""
	}

	a.viewport.SetContent(a.renderMessages())
	a.viewport.GotoBottom()
}

// SetTheme dynamically updates the theme and refreshes the UI.
func (a *App) SetTheme(t Theme) {
	a.cfg.Theme = t
	a.cacheCount = 0
	a.renderedCache = ""
	if a.ready {
		a.viewport.SetContent(a.renderMessages())
	}
}

// GetTheme returns the active theme.
func (a *App) GetTheme() Theme {
	return a.cfg.Theme
}

func (a *App) renderWelcomeHero(vpW int) string {
	dim := lipgloss.NewStyle().Foreground(a.cfg.Theme.Muted)
	sub := lipgloss.NewStyle().Foreground(a.cfg.Theme.SubText)
	primary := lipgloss.NewStyle().Bold(true).Foreground(a.cfg.Theme.Primary)
	secondary := lipgloss.NewStyle().Foreground(a.cfg.Theme.Secondary)
	cmdPill := lipgloss.NewStyle().Bold(true).Foreground(a.cfg.Theme.Primary)
	descStyle := lipgloss.NewStyle().Foreground(a.cfg.Theme.SubText)

	var sb strings.Builder
	sb.WriteString("\n")

	asciiBig := []string{
		"  ██████╗ ███████╗██╗   ██╗██████╗  █████╗ ███╗   ██╗",
		"  ██╔══██╗██╔════╝██║   ██║██╔══██╗██╔══██╗████╗  ██║",
		"  ██║  ██║█████╗  ██║   ██║██████╔╝███████║██╔██╗ ██║",
		"  ██║  ██║██╔══╝  ╚██╗ ██╔╝██╔══██╗██╔══██║██║╚██╗██║",
		"  ██████╔╝███████╗ ╚████╔╝ ██║  ██║██║  ██║██║ ╚████║",
		"  ╚═════╝ ╚══════╝  ╚═══╝  ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝",
	}

	asciiMini := []string{
		"   ___  _______   ______  ___   _  __",
		"  / _ \\/ __/\\ \\ / / _  / / _ | / |/ /",
		" / // / _/   \\ V / , _/ / __ |/    / ",
		"/____/___/    \\_/_/|_| /_/ |_/_/|_/  ",
	}

	if vpW >= 60 {
		for _, line := range asciiBig {
			sb.WriteString(primary.Render(line) + "\n")
		}
	} else if vpW >= 42 {
		for _, line := range asciiMini {
			sb.WriteString(primary.Render(line) + "\n")
		}
	} else {
		sb.WriteString(primary.Render("  [ DEVRAN AI AGENT ]") + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString("  " + secondary.Render("Autonomous Software Engineering Agent") + "\n")

	meta := fmt.Sprintf("  Model: %s", a.modelName)
	if a.workDir != "" {
		wd := a.workDir
		if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(wd, home) {
			wd = "~" + wd[len(home):]
		}
		meta += fmt.Sprintf("  ·  Workspace: %s", wd)
	}
	sb.WriteString(dim.Render(meta) + "\n\n")

	sb.WriteString("  " + sub.Render("Quick Actions:") + "\n")
	shortcuts := []struct {
		cmd  string
		desc string
	}{
		{"/plan", "Formulate implementation plan"},
		{"/search", "Semantic & AST code search via Gleann"},
		{"/review", "Multi-agent review working changes"},
		{"/test", "Run project test suite & verify fixes"},
		{"/theme", "Switch UI theme (claude / devran)"},
		{"/help", "List all slash commands & shortcuts"},
	}

	for _, s := range shortcuts {
		pill := cmdPill.Render(fmt.Sprintf("  %-8s", s.cmd))
		desc := descStyle.Render(s.desc)
		sb.WriteString(pill + " " + desc + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(dim.Render("  Tip: Type / for command autocomplete · Tab to complete · ↑/↓ history") + "\n")

	return sb.String()
}

func (a *App) renderMessages() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if len(a.messages) == 0 && !a.streaming {
		vpW := a.viewport.Width()
		if vpW == 0 {
			vpW = a.width
		}
		return a.renderWelcomeHero(vpW)
	}

	rendered := a.renderHistoryBlock()
	if a.streaming {
		rendered += a.renderStreamingTail()
	} else if a.waiting {
		rendered += " " + a.spinner.View() + " " + lipgloss.NewStyle().Foreground(a.cfg.Theme.Muted).Render("Thinking…") + "\n\n"
	}
	return rendered
}

func (a *App) renderMessagesfast() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if len(a.messages) == 0 && !a.streaming {
		vpW := a.viewport.Width()
		if vpW == 0 {
			vpW = a.width
		}
		return a.renderWelcomeHero(vpW)
	}

	history := a.renderHistoryBlock()
	if a.streaming {
		return history + a.renderStreamingTail()
	} else if a.waiting {
		return history + " " + a.spinner.View() + " " + lipgloss.NewStyle().Foreground(a.cfg.Theme.Muted).Render("Thinking…") + "\n\n"
	}
	return history
}

func (a *App) renderMarkdown(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}

	renderer := a.renderer
	if renderer == nil {
		vpW := a.width
		if vpW < 40 {
			vpW = 80
		}
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(vpW-6),
		)
		if err == nil {
			a.renderer = r
			a.renderW = vpW
			renderer = r
		}
	}

	if renderer != nil {
		rendered, err := renderer.Render(content)
		if err == nil {
			return strings.TrimRight(rendered, "\n")
		}
	}
	return content
}

func (a *App) renderStreamingTail() string {
	a.streamMu.Lock()
	partial := a.streamBuf.String()
	a.streamMu.Unlock()

	if partial == "" {
		return ""
	}

	asstTag := lipgloss.NewStyle().Bold(true).Foreground(a.cfg.Theme.Primary).Render("◈ " + a.cfg.Title)
	var sb strings.Builder
	sb.WriteString(asstTag + "\n\n")

	thinking, content := extractThinkingTags(partial)
	if strings.Contains(partial, "<think>") && !strings.Contains(partial, "</think>") {
		sb.WriteString(lipgloss.NewStyle().Foreground(a.cfg.Theme.Accent).Bold(true).Render("▼ Thinking…") + " " + a.spinner.View() + "\n\n")
		cardWidth := a.viewport.Width() - 6
		if cardWidth < 40 {
			cardWidth = 40
		}
		bodyStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(a.cfg.Theme.Border).
			Padding(0, 1).
			Foreground(a.cfg.Theme.Muted).
			Italic(true).
			Width(cardWidth)
		sb.WriteString(bodyStyle.Render(thinking))
		sb.WriteString("\n\n")
		return sb.String()
	}

	if thinking != "" {
		tagStyle := lipgloss.NewStyle().Foreground(a.cfg.Theme.Accent).Bold(true)
		headerStyle := lipgloss.NewStyle().Foreground(a.cfg.Theme.SubText).Italic(true)
		hintStyle := lipgloss.NewStyle().Foreground(a.cfg.Theme.Muted)
		sb.WriteString(headerStyle.Render(fmt.Sprintf("%s Thought ", tagStyle.Render("▶"))) + hintStyle.Render("[click or Ctrl+T to expand]") + "\n\n")
	}

	if strings.TrimSpace(content) != "" {
		sb.WriteString(a.renderMarkdown(content))
	}
	sb.WriteString(" " + a.spinner.View() + "\n\n")
	return sb.String()
}

func (a *App) renderHistoryBlock() string {
	if a.cacheCount == len(a.messages) && a.renderedCache != "" {
		return a.renderedCache
	}

	var sb strings.Builder

	for _, m := range a.messages {
		switch m.Role {
		case RoleUser:
			sb.WriteString(a.renderUserCard(m.Content, m.Timestamp))
			sb.WriteString("\n\n")

		case RoleAssistant:
			thinking := m.Thinking
			content := m.Content
			if thinking == "" && strings.Contains(content, "<think>") {
				thinking, content = extractThinkingTags(content)
			}
			if strings.TrimSpace(content) != "" || strings.TrimSpace(thinking) != "" {
				asstTag := lipgloss.NewStyle().Bold(true).Foreground(a.cfg.Theme.Primary).Render("◈ " + a.cfg.Title)
				tsTag := lipgloss.NewStyle().Foreground(a.cfg.Theme.Muted).Render(" · " + m.Timestamp.Format("15:04"))
				sb.WriteString(asstTag + tsTag + "\n\n")

				if strings.TrimSpace(thinking) != "" {
					sb.WriteString(a.renderThinkingCard(thinking, m.ThinkingOpen, m.ThinkingTime))
					sb.WriteString("\n\n")
				}

				if strings.TrimSpace(content) != "" {
					sb.WriteString(a.renderMarkdown(content))
					sb.WriteString("\n\n")
				}
			}

		case RoleTool:
			if m.Tool != nil {
				sb.WriteString(a.renderToolCard(m.Tool))
				sb.WriteString("\n\n")
			}

		case RoleSystem:
			sb.WriteString(a.renderMarkdown(m.Content))
			sb.WriteString("\n\n")

		case RoleError:
			sb.WriteString(a.renderErrorCard(m.Content, m.Timestamp))
			sb.WriteString("\n\n")
		}
	}

	res := sb.String()
	a.renderedCache = res
	a.cacheCount = len(a.messages)
	return res
}

// renderUserCard renders a user prompt turn with a sleek horizontal divider banner, prompt badge, and timestamp.
func (a *App) renderUserCard(content string, ts time.Time) string {
	maxW := a.viewport.Width()
	if maxW <= 0 {
		maxW = a.width - 2
	}
	divW := maxW - 2
	if divW < 20 {
		divW = 20
	}
	if divW > 96 {
		divW = 96
	}

	borderStyle := lipgloss.NewStyle().Foreground(a.cfg.Theme.Border)
	promptStyle := lipgloss.NewStyle().Bold(true).Foreground(a.cfg.Theme.Secondary)
	tsStyle := lipgloss.NewStyle().Foreground(a.cfg.Theme.Muted)

	promptBadge := promptStyle.Render("❯ You")
	timeStr := tsStyle.Render(ts.Format("15:04"))

	// Format: ── ❯ You · 15:04 ──────────────────────────────────
	left := borderStyle.Render("── ") + promptBadge + tsStyle.Render(" · ") + timeStr + borderStyle.Render(" ")
	leftLen := lipgloss.Width(left)
	dashCount := divW - leftLen
	if dashCount < 2 {
		dashCount = 2
	}
	banner := left + borderStyle.Render(strings.Repeat("─", dashCount))

	var bodyText string
	if strings.ContainsAny(content, "#*`_~[]\n") {
		bodyText = a.renderMarkdown(content)
	} else {
		bodyText = lipgloss.NewStyle().Foreground(a.cfg.Theme.Text).Render(content)
	}

	return banner + "\n\n" + bodyText
}

// renderErrorCard renders an error message with a distinct red error badge and timestamp.
func (a *App) renderErrorCard(content string, ts time.Time) string {
	errStyle := lipgloss.NewStyle().Bold(true).Foreground(a.cfg.Theme.Error)
	tsStyle := lipgloss.NewStyle().Foreground(a.cfg.Theme.Muted)
	header := errStyle.Render("✗ Error") + tsStyle.Render(" · "+ts.Format("15:04"))
	return header + "\n" + errStyle.Render(content)
}

func (a *App) renderThinkingCard(thinking string, open bool, duration time.Duration) string {
	thinking = strings.TrimSpace(thinking)
	if thinking == "" {
		return ""
	}

	timeStr := ""
	if duration > 0 {
		timeStr = fmt.Sprintf(" (%s)", duration.Round(100*time.Millisecond))
	}

	tagStyle := lipgloss.NewStyle().Foreground(a.cfg.Theme.Accent).Bold(true)
	headerStyle := lipgloss.NewStyle().Foreground(a.cfg.Theme.SubText).Italic(true)
	hintStyle := lipgloss.NewStyle().Foreground(a.cfg.Theme.Muted)

	if !open {
		return headerStyle.Render(fmt.Sprintf("%s Thought%s ", tagStyle.Render("▶"), timeStr)) +
			hintStyle.Render("[click or Ctrl+T to expand]")
	}

	header := headerStyle.Render(fmt.Sprintf("%s Thought%s ", tagStyle.Render("▼"), timeStr)) +
		hintStyle.Render("[click or Ctrl+T to collapse]")

	cardWidth := a.viewport.Width() - 6
	if cardWidth < 40 {
		cardWidth = 40
	}

	bodyStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(a.cfg.Theme.Border).
		Padding(0, 1).
		Foreground(a.cfg.Theme.Muted).
		Italic(true).
		Width(cardWidth)

	return header + "\n" + bodyStyle.Render(thinking)
}

func extractThinkingTags(raw string) (string, string) {
	startTag := "<think>"
	endTag := "</think>"
	startIdx := strings.Index(raw, startTag)
	if startIdx == -1 {
		return "", raw
	}
	endIdx := strings.Index(raw, endTag)
	if endIdx == -1 {
		return strings.TrimSpace(raw[startIdx+len(startTag):]), strings.TrimSpace(raw[:startIdx])
	}
	thinking := strings.TrimSpace(raw[startIdx+len(startTag) : endIdx])
	content := strings.TrimSpace(raw[:startIdx] + raw[endIdx+len(endTag):])
	return thinking, content
}

func (a *App) hasThinkingMessages() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, m := range a.messages {
		if m.Thinking != "" || strings.Contains(m.Content, "<think>") {
			return true
		}
	}
	return false
}

func (a *App) toggleThinking() (*App, tea.Cmd) {
	a.mu.Lock()
	anyOpen := false
	hasThinking := false
	for _, m := range a.messages {
		if m.Thinking != "" || strings.Contains(m.Content, "<think>") {
			hasThinking = true
			if m.ThinkingOpen {
				anyOpen = true
			}
		}
	}
	if !hasThinking {
		a.mu.Unlock()
		return a, nil
	}

	newOpen := !anyOpen
	count := 0
	for i := range a.messages {
		if a.messages[i].Thinking != "" || strings.Contains(a.messages[i].Content, "<think>") {
			a.messages[i].ThinkingOpen = newOpen
			count++
		}
	}
	a.cacheCount = 0
	a.renderedCache = ""
	a.mu.Unlock()

	a.viewport.SetContent(a.renderMessages())
	status := "Thinking hidden"
	if newOpen {
		status = fmt.Sprintf("Thinking expanded (%d message(s))", count)
	}
	return a, a.SetNotification(status, NotifyInfo)
}

func (a *App) renderToolCard(t *ToolCall) string {
	var icon string
	switch t.Status {
	case ToolStatusRunning:
		icon = a.spinner.View() + " "
	case ToolStatusSuccess:
		icon = lipgloss.NewStyle().Foreground(a.cfg.Theme.Success).Render("✓ ")
	case ToolStatusFailed:
		icon = lipgloss.NewStyle().Foreground(a.cfg.Theme.Error).Render("✗ ")
	}

	title := fmt.Sprintf("%sTool: %s(%s)", icon, t.Name, t.Args)
	if t.Duration > 0 {
		title += fmt.Sprintf(" [%s]", t.Duration.Round(time.Millisecond))
	}

	body := title
	if t.Output != "" {
		outLines := strings.Split(t.Output, "\n")
		if len(outLines) > 4 {
			body += "\n" + strings.Join(outLines[:4], "\n") + fmt.Sprintf("\n... (+%d lines)", len(outLines)-4)
		} else {
			body += "\n" + t.Output
		}
	}

	return a.cfg.Theme.ToolCard.Render(body)
}

func (a *App) renderAutocomplete() string {
	if !a.autoActive {
		return ""
	}

	var sb strings.Builder

	if a.autoMode == "at" {
		if len(a.autoAtFiles) == 0 {
			return ""
		}
		for i, f := range a.autoAtFiles {
			fileStyle := lipgloss.NewStyle().Foreground(a.cfg.Theme.SubText)
			prefix := "  "
			if i == a.autoCursor {
				fileStyle = fileStyle.Bold(true).Foreground(a.cfg.Theme.Primary)
				prefix = "▸ "
			}
			sb.WriteString(prefix + fileStyle.Render("@"+f) + "\n")
		}
	} else {
		if len(a.autoFiltered) == 0 {
			return ""
		}
		limit := 8
		start := 0
		end := len(a.autoFiltered)
		if end > limit {
			if a.autoCursor >= limit {
				start = a.autoCursor - limit + 1
			}
			end = start + limit
			if end > len(a.autoFiltered) {
				end = len(a.autoFiltered)
				start = end - limit
			}
		}

		for i := start; i < end; i++ {
			cmd := a.autoFiltered[i]
			cmdStyle := lipgloss.NewStyle().Width(14)
			descStyle := lipgloss.NewStyle().Foreground(a.cfg.Theme.Muted)

			if i == a.autoCursor {
				cmdStyle = cmdStyle.Bold(true).Foreground(a.cfg.Theme.Primary)
				descStyle = descStyle.Foreground(a.cfg.Theme.Text)
				sb.WriteString("▸ ")
			} else {
				sb.WriteString("  ")
			}

			cmdText := cmdStyle.Render("/" + cmd.Name)
			descText := descStyle.Render(cmd.Description)
			sb.WriteString(cmdText + " " + descText + "\n")
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(a.cfg.Theme.Border).
		Padding(0, 1).
		Render(strings.TrimRight(sb.String(), "\n"))
}

// renderScrollbar draws a vertical scrollbar for the viewport, matching gleann's implementation.
func (a *App) renderScrollbar(h int) string {
	if h <= 0 {
		return ""
	}

	totalLines := a.viewport.TotalLineCount()
	visibleLines := h

	thumbStyle := lipgloss.NewStyle().Foreground(a.cfg.Theme.Primary)
	trackStyle := lipgloss.NewStyle().Foreground(a.cfg.Theme.Muted)

	// If all content fits, show a subtle track
	if totalLines <= visibleLines {
		lines := make([]string, h)
		for i := range lines {
			lines[i] = trackStyle.Render("│")
		}
		return strings.Join(lines, "\n")
	}

	// Calculate thumb size and position
	thumbSize := max(1, visibleLines*visibleLines/totalLines)
	scrollOffset := a.viewport.ScrollPercent()
	maxOffset := float64(totalLines - visibleLines)
	thumbTop := 0
	if maxOffset > 0 {
		thumbTop = int(scrollOffset * float64(h-thumbSize))
	}
	if thumbTop < 0 {
		thumbTop = 0
	}
	if thumbTop+thumbSize > h {
		thumbTop = h - thumbSize
	}
	thumbBottom := thumbTop + thumbSize

	lines := make([]string, h)
	for i := range lines {
		if i >= thumbTop && i < thumbBottom {
			lines[i] = thumbStyle.Render("▓")
		} else {
			lines[i] = trackStyle.Render("│")
		}
	}
	return strings.Join(lines, "\n")
}

// renderSidebar returns the right-side info panel.
func (a *App) renderSidebar(w, h int) string {
	dim := lipgloss.NewStyle().Foreground(a.cfg.Theme.Muted)
	label := lipgloss.NewStyle().Foreground(a.cfg.Theme.SubText)
	val := lipgloss.NewStyle().Foreground(a.cfg.Theme.Text)

	var lines []string
	lines = append(lines, dim.Render("─── info ───"))
	lines = append(lines, "")

	// Mode
	modeColor := a.cfg.Theme.Primary
	if a.mode == "build" {
		modeColor = a.cfg.Theme.Accent
	}
	lines = append(lines, label.Render("mode"))
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(modeColor).Render(strings.ToUpper(a.mode)))
	lines = append(lines, "")

	// Model
	lines = append(lines, label.Render("model"))
	modelDisp := a.modelName
	if modelDisp == "" {
		modelDisp = "?"
	}
	if len(modelDisp) > w-4 {
		modelDisp = modelDisp[:w-7] + "…"
	}
	lines = append(lines, val.Render(modelDisp))
	lines = append(lines, "")

	// Context Window
	lines = append(lines, label.Render("context"))
	barW := (w - 12) / 2
	if barW < 4 {
		barW = 4
	}
	lines = append(lines, a.renderTokenBar(barW))
	lines = append(lines, "")

	// Turns
	userCount := 0
	a.mu.RLock()
	for _, m := range a.messages {
		if m.Role == RoleUser {
			userCount++
		}
	}
	a.mu.RUnlock()
	lines = append(lines, label.Render("turns"))
	lines = append(lines, val.Render(fmt.Sprintf("%d", userCount)))
	lines = append(lines, "")

	// Status
	lines = append(lines, label.Render("status"))
	if a.streaming {
		lines = append(lines, lipgloss.NewStyle().Foreground(a.cfg.Theme.Success).Render("● stream"))
	} else if a.waiting {
		lines = append(lines, lipgloss.NewStyle().Foreground(a.cfg.Theme.Warning).Render("● think"))
	} else {
		lines = append(lines, lipgloss.NewStyle().Foreground(a.cfg.Theme.Muted).Render("● idle"))
	}
	lines = append(lines, "")

	// Duration
	lines = append(lines, label.Render("session"))
	elapsed := time.Since(a.startTime).Truncate(time.Second)
	lines = append(lines, val.Render(elapsed.String()))
	lines = append(lines, "")

	// Changed files
	a.mu.RLock()
	cf := a.changedFiles
	a.mu.RUnlock()
	if len(cf) > 0 {
		lines = append(lines, dim.Render("─ changes ─"))
		for _, f := range cf {
			name := f
			if idx := strings.LastIndex(f, "/"); idx >= 0 {
				name = f[idx+1:]
			}
			if len(name) > w-4 {
				name = name[:w-7] + "…"
			}
			lines = append(lines, val.Render(name))
		}
		lines = append(lines, "")
	}

	// Attached Widget (e.g. Plan Tracker)
	if a.sidebar != nil && a.sidebar.Visible() {
		lines = append(lines, dim.Render("─ "+a.sidebar.Title()+" ─"))
		sideRender := a.sidebar.Render(w-2, h/2)
		if sideRender != "" {
			lines = append(lines, sideRender)
		}
	}

	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}

	content := strings.Join(lines, "\n")
	rendered := lipgloss.NewStyle().
		Width(w).
		Padding(0, 1).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(a.cfg.Theme.Border).
		Render(content)

	rLines := strings.Split(rendered, "\n")
	if len(rLines) > h {
		rLines = rLines[:h]
	}
	for len(rLines) < h {
		rLines = append(rLines, "")
	}
	return strings.Join(rLines, "\n")
}

// View implements tea.Model.
func (a *App) View() tea.View {
	if a.quitting {
		v := tea.NewView(lipgloss.NewStyle().Foreground(a.cfg.Theme.Muted).Render("Goodbye! 👋\n"))
		return v
	}
	if !a.ready {
		return tea.NewView("Initializing…\n")
	}

	dim := lipgloss.NewStyle().Foreground(a.cfg.Theme.Muted)
	var topLines []string
	// 1. Header (HeaderLine + Separator)
	if a.cfg.ShowHeader {
		title := a.cfg.Theme.HeaderTitle.Render(a.cfg.Title)
		modeColor := a.cfg.Theme.Primary
		if a.mode == "build" {
			modeColor = a.cfg.Theme.Accent
		}
		modeBadge := lipgloss.NewStyle().Bold(true).Foreground(modeColor).Render("[" + strings.ToUpper(a.mode) + "]")
		left := title + " " + modeBadge

		if a.workDir != "" {
			wd := a.workDir
			if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(wd, home) {
				wd = "~" + wd[len(home):]
			}
			maxW := a.width - 45
			if len(wd) > maxW && maxW > 10 {
				wd = "…" + wd[len(wd)-maxW+1:]
			}
			left += " " + dim.Render(wd)
		}

		right := dim.Render(a.modelName)
		sidebarW := sidebarWidthFor(a.width)
		if sidebarW == 0 && a.width >= 60 {
			right += " " + a.renderTokenBar(8)
		} else if a.totalTokens > 0 && sidebarW == 0 {
			right += dim.Render(fmt.Sprintf(" · %dtok", a.totalTokens))
		}

		pad := a.width - lipgloss.Width(left) - lipgloss.Width(right) - 4
		if pad < 1 {
			pad = 1
		}
		headerLine := " " + left + strings.Repeat(" ", pad) + right + " "
		sepLine := dim.Render(strings.Repeat("─", a.width))
		topLines = append(topLines, headerLine, sepLine)
	}

	// 2. Bottom components: status, autocomplete popup, textarea, footer
	var bottomLines []string
	if a.waiting || a.streaming {
		var indicator string
		if a.streaming {
			streamText := "streaming…"
			phase := int(time.Now().UnixMilli()/120) % len(streamText)
			streamColors := []color.Color{
				lipgloss.Color("#2ECC71"), lipgloss.Color("#27AE60"), lipgloss.Color("#1ABC9C"), lipgloss.Color("#16A085"), lipgloss.Color("#00B894"),
				lipgloss.Color("#00CEC9"), lipgloss.Color("#00B894"), lipgloss.Color("#16A085"), lipgloss.Color("#1ABC9C"), lipgloss.Color("#27AE60"),
			}
			var coloredText strings.Builder
			for i, ch := range streamText {
				idx := (i + phase) % len(streamColors)
				coloredText.WriteString(lipgloss.NewStyle().Foreground(streamColors[idx]).Render(string(ch)))
			}
			indicator = a.spinner.View() + " " + coloredText.String() + dim.Render(" (type to redirect · Esc cancel)")
		} else {
			indicator = a.spinner.View() + " " + lipgloss.NewStyle().Foreground(a.cfg.Theme.SubText).Italic(true).Render("thinking…")
		}
		bottomLines = append(bottomLines, " "+indicator)
	}

	if a.autoActive && len(a.autoFiltered) > 0 {
		popup := a.renderAutocomplete()
		bottomLines = append(bottomLines, strings.Split(popup, "\n")...)
	}

	// Textarea: always 5 lines, never pushed off-screen
	modeColor := a.cfg.Theme.Primary
	if a.mode == "build" {
		modeColor = a.cfg.Theme.Accent
	}
	taView := a.textarea.View()
	taBordered := lipgloss.NewStyle().
		BorderForeground(modeColor).
		Border(lipgloss.RoundedBorder()).
		Render(taView)
	bottomLines = append(bottomLines, strings.Split(taBordered, "\n")...)

	// Footer: always 1 line
	if a.cfg.ShowFooter {
		notification := a.renderNotification(a.width - 4)
		var foot string
		if notification != "" {
			foot = " " + notification
		} else if a.streaming {
			secs := time.Since(a.streamStartTime).Seconds()
			streamTimer := fmt.Sprintf("⋯ streaming %5.1fs", secs)
			if secs >= 60 {
				mins := int(secs) / 60
				remSecs := secs - float64(mins*60)
				streamTimer = fmt.Sprintf("⋯ streaming %02d:%04.1fs", mins, remSecs)
			}
			left := dim.Render(" Esc cancel · Ctrl+Y copy · Ctrl+C quit")
			right := dim.Render(streamTimer)
			gap := a.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
			if gap < 1 {
				gap = 1
			}
			foot = " " + left + strings.Repeat(" ", gap) + right
		} else {
			foot = dim.Render(" Enter send · / commands · Ctrl+T thinking · Ctrl+Y copy · ↑↓ history · Ctrl+M mouse · Ctrl+C quit")
		}
		bottomLines = append(bottomLines, foot)
	}

	// 3. Middle content (Viewport + Scrollbar + Sidebar): exactly consumes available vertical space
	bodyH := a.height - len(topLines) - len(bottomLines)
	if bodyH < 3 {
		bodyH = 3
	}
	if a.viewport.Height() != bodyH {
		a.viewport.SetHeight(bodyH)
	}

	sidebarW := sidebarWidthFor(a.width)
	mainView := a.viewport.View()

	// Viewport with vertical scrollbar (matching gleann chat)
	scrollbar := a.renderScrollbar(bodyH)
	vpLines := strings.Split(mainView, "\n")
	sbLines := strings.Split(scrollbar, "\n")
	for len(sbLines) < len(vpLines) {
		sbLines = append(sbLines, "│")
	}

	var vpWithScroll strings.Builder
	for i, line := range vpLines {
		sb := ""
		if i < len(sbLines) {
			sb = sbLines[i]
		}
		vpWithScroll.WriteString(line + sb + "\n")
	}
	vpJoined := strings.TrimRight(vpWithScroll.String(), "\n")

	var bodyBlock string
	if sidebarW > 0 {
		sidebar := a.renderSidebar(sidebarW, bodyH)
		bodyBlock = lipgloss.JoinHorizontal(lipgloss.Top, vpJoined, sidebar)
	} else {
		bodyBlock = vpJoined
	}

	bodyLines := strings.Split(bodyBlock, "\n")
	if len(bodyLines) > bodyH {
		bodyLines = bodyLines[:bodyH]
	}
	for len(bodyLines) < bodyH {
		bodyLines = append(bodyLines, "")
	}

	// 4. Assemble final screen
	var finalLines []string
	finalLines = append(finalLines, topLines...)
	finalLines = append(finalLines, bodyLines...)
	finalLines = append(finalLines, bottomLines...)

	// If overall height ever exceeds a.height, trim body lines, NEVER bottom input controls!
	if len(finalLines) > a.height {
		overflow := len(finalLines) - a.height
		bodyEnd := len(topLines) + len(bodyLines)
		if overflow < len(bodyLines) {
			finalLines = append(finalLines[:bodyEnd-overflow], finalLines[bodyEnd:]...)
		} else {
			finalLines = finalLines[:a.height]
		}
	}

	result := strings.Join(finalLines, "\n")
	v := tea.NewView(result)
	v.AltScreen = true
	if a.mouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

