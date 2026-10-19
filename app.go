package agenttui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
)

// Internal message types
type streamChunkMsg struct{ chunk string }
type streamDoneMsg struct {
	tokens int
	err    error
}
type streamEndMsg struct{}
type streamEventMsg struct{ ev Event }
type slashDoneMsg struct {
	input   string
	out     string
	isLocal bool
	err     error
}

// App manages the chat application lifecycle and Bubble Tea program.
type App struct {
	cfg Config

	// State
	mu          sync.RWMutex
	messages    []Message
	commands    map[string]SlashCommand
	cmdList     []SlashCommand
	sidebar     Widget
	modelName   string
	workDir     string
	totalTokens int

	// Active Bubble Tea components
	program   *tea.Program
	textarea  textarea.Model
	viewport  viewport.Model
	spinner   spinner.Model
	renderer  *glamour.TermRenderer
	renderW   int
	width     int
	height    int
	ready     bool
	quitting  bool
	streaming bool

	// Stream channel
	streamBuf    *strings.Builder
	streamCh     chan tea.Msg
	streamCtx    context.Context
	streamCancel context.CancelFunc

	// Context window tracking
	contextUsed int
	contextMax  int

	// Autocomplete state
	autoActive   bool
	autoMode     string // "slash" or "at"
	autoFiltered []SlashCommand
	autoAtFiles  []string
	autoCursor   int
	repoFiles    []string

	// History
	history    []string
	historyIdx int

	// Mouse enabled
	mouseEnabled bool

	// Session & Mode
	startTime       time.Time
	mode            string
	waiting         bool
	changedFiles    []string
	notification    *Notification
	initDone        bool
	renderedCache   string
	cacheCount      int
	historySave     string
	sidebarHidden   bool
	streamStartTime time.Time
	streamMu        *sync.Mutex
	ask             *askState
	askSeq          int
}

// New creates a new extensible Agent TUI instance.
func New(cfg Config) *App {
	if cfg.Title == "" {
		cfg.Title = "Agent"
	}
	if cfg.Placeholder == "" {
		cfg.Placeholder = "Type a message… (Enter send · Alt+Enter newline · / commands)"
	}
	mode := cfg.Mode
	if mode == "" {
		mode = "build"
	}

	ta := textarea.New()
	ta.Placeholder = cfg.Placeholder
	ta.Focus()
	ta.CharLimit = 8192
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	configureEditorKeys(&ta)

	// Clear textarea built-in border so we can draw the outer rounded border cleanly
	taStyles := ta.Styles()
	taStyles.Focused.CursorLine = lipgloss.NewStyle()
	taStyles.Focused.Base = lipgloss.NewStyle()
	ta.SetStyles(taStyles)

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	app := &App{
		cfg:          cfg,
		commands:     make(map[string]SlashCommand),
		cmdList:      make([]SlashCommand, 0),
		sidebar:      cfg.Sidebar,
		modelName:    cfg.ModelName,
		workDir:      cfg.WorkDir,
		mode:         mode,
		startTime:    time.Now(),
		textarea:     ta,
		spinner:      sp,
		streamBuf:    &strings.Builder{},
		streamMu:     &sync.Mutex{},
		historyIdx:   -1,
		mouseEnabled: cfg.EnableMouse,
	}

	// Register built-in slash commands
	app.RegisterCommand(SlashCommand{
		Name:        "help",
		Description: "Show available slash commands",
		Handler: func(ctx context.Context, args []string, a *App) (string, bool, error) {
			var sb strings.Builder
			sb.WriteString("### Available Slash Commands\n\n")
			for _, c := range a.cmdList {
				usage := c.Name
				if c.Usage != "" {
					usage += " " + c.Usage
				}
				sb.WriteString(fmt.Sprintf("- `/%-14s` %s\n", usage, c.Description))
			}
			return sb.String(), true, nil
		},
	})

	app.RegisterCommand(SlashCommand{
		Name:        "clear",
		Description: "Clear conversation history",
		Handler: func(ctx context.Context, args []string, a *App) (string, bool, error) {
			a.ClearMessages()
			return "Conversation cleared.", true, nil
		},
	})

	app.RegisterCommand(SlashCommand{
		Name:        "retry",
		Description: "Regenerate the last response",
		Handler: func(ctx context.Context, args []string, a *App) (string, bool, error) {
			a.dropTrailingCommandEcho()
			prompt, ok := a.RewindLastTurn()
			if !ok {
				return "Nothing to retry.", true, nil
			}
			a.textarea.Reset()
			a.AddMessage(Message{Role: RoleUser, Content: prompt})
			return a.expandFileReferences(prompt), false, nil
		},
	})

	for _, cmd := range cfg.Commands {
		app.RegisterCommand(cmd)
	}

	if cfg.Store != nil {
		if msgs, err := cfg.Store.Load(); err == nil {
			app.messages = msgs
		}
	}

	return app
}

// RegisterCommand adds or updates a slash command in the registry without duplicates.
func (a *App) RegisterCommand(cmd SlashCommand) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.commands[cmd.Name] = cmd
	for i, c := range a.cmdList {
		if c.Name == cmd.Name {
			a.cmdList[i] = cmd
			return
		}
	}
	a.cmdList = append(a.cmdList, cmd)
}

// SetModelName updates the active model label in the status bar.
func (a *App) SetModelName(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.modelName = name
}

// GetModelName returns the current active model identifier.
func (a *App) GetModelName() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.modelName
}

// SetWorkDir updates the active workspace path.
func (a *App) SetWorkDir(dir string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.workDir = dir
}

// GetWorkDir returns the active workspace path.
func (a *App) GetWorkDir() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.workDir
}

// SetSidebar attaches or replaces a sidecar widget.
func (a *App) SetSidebar(w Widget) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sidebar = w
}

// SetMode updates the active mode (e.g. "build", "plan").
func (a *App) SetMode(mode string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mode = mode
}

// Mode returns current mode.
func (a *App) Mode() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.mode
}

// SetContextTokens updates the context window usage indicator.
func (a *App) SetContextTokens(used, max int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.contextUsed = used
	if max > 0 {
		a.contextMax = max
	}
}

// SetRepoFiles sets the cached list of workspace files for @file completion.
func (a *App) SetRepoFiles(files []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.repoFiles = files
}

// ScanRepoFiles walks the workspace root and collects files for @file completion.
func (a *App) ScanRepoFiles(root string) {
	var files []string
	ignoreDirs := map[string]bool{
		".git": true, "node_modules": true, "vendor": true, "build": true,
	}
	for _, d := range a.cfg.IgnoreDirs {
		ignoreDirs[d] = true
	}

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || len(files) >= 1500 {
			return nil
		}
		if d.IsDir() {
			if ignoreDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err == nil && !strings.HasPrefix(rel, ".") {
			files = append(files, rel)
		}
		return nil
	})

	a.SetRepoFiles(files)
}

// SetChangedFiles updates the list of changed files shown in the sidebar.
func (a *App) SetChangedFiles(files []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.changedFiles = files
}

// AddChangedFile appends a file to the changed files list.
func (a *App) AddChangedFile(file string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, f := range a.changedFiles {
		if f == file {
			return
		}
	}
	a.changedFiles = append(a.changedFiles, file)
}

// AddMessage appends a message to the conversation.
func (a *App) AddMessage(msg Message) {
	a.mu.Lock()
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	if msg.Thinking == "" && strings.Contains(msg.Content, "<think>") {
		think, clean := extractThinkingTags(msg.Content)
		if think != "" {
			msg.Thinking = think
			msg.Content = clean
		}
	}
	a.messages = append(a.messages, msg)
	a.cacheCount = 0
	a.renderedCache = ""
	ready := a.ready
	a.mu.Unlock()

	if ready {
		a.viewport.SetContent(a.renderMessages())
	}
}

// dropTrailingCommandEcho removes the "/cmd" user message that handleSubmit
// echoes before running a slash command, so /retry rewinds the real last turn.
func (a *App) dropTrailingCommandEcho() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if n := len(a.messages); n > 0 && a.messages[n-1].Role == RoleUser && strings.HasPrefix(a.messages[n-1].Content, "/") {
		a.messages = a.messages[:n-1]
	}
}

// saveSession persists the transcript through cfg.Store, if configured.
func (a *App) saveSession() {
	if a.cfg.Store == nil {
		return
	}
	a.mu.RLock()
	msgs := append([]Message(nil), a.messages...)
	a.mu.RUnlock()
	_ = a.cfg.Store.Save(msgs)
}

// ClearMessages removes all messages from the conversation transcript.
func (a *App) ClearMessages() {
	a.mu.Lock()
	a.messages = nil
	a.mu.Unlock()
	a.cacheCount = 0
	a.renderedCache = ""
	if a.ready {
		a.viewport.SetContent(a.renderMessages())
		a.viewport.GotoTop()
	}
}

// ResetSession clears all conversation messages, history, sidebar widgets,
// changed files, and resets session timer and context counters.
func (a *App) ResetSession() {
	a.mu.Lock()
	a.messages = nil
	a.sidebar = nil
	a.changedFiles = nil
	a.contextUsed = 0
	a.totalTokens = 0
	a.historyIdx = -1
	a.startTime = time.Now()
	a.mu.Unlock()
	a.cacheCount = 0
	a.renderedCache = ""
	if a.ready {
		a.viewport.SetContent(a.renderMessages())
		a.viewport.GotoTop()
	}
}

// RewindLastTurn pops the last assistant response (and any associated tool calls)
// and the last user prompt from the conversation history, and restores that user
// prompt back into the input textarea so the user can easily revise and resubmit it.
func (a *App) RewindLastTurn() (string, bool) {
	a.mu.Lock()
	if len(a.messages) == 0 {
		a.mu.Unlock()
		return "", false
	}

	// 1. Pop trailing assistant, tool, or error messages
	for len(a.messages) > 0 {
		last := a.messages[len(a.messages)-1]
		if last.Role == RoleAssistant || last.Role == RoleTool || last.Role == RoleError {
			a.messages = a.messages[:len(a.messages)-1]
		} else {
			break
		}
	}

	// 2. Pop the user message and restore its text
	var userPrompt string
	if len(a.messages) > 0 && a.messages[len(a.messages)-1].Role == RoleUser {
		userPrompt = a.messages[len(a.messages)-1].Content
		a.messages = a.messages[:len(a.messages)-1]
	}

	a.cacheCount = 0
	a.renderedCache = ""
	ready := a.ready
	a.mu.Unlock()

	if userPrompt != "" {
		a.textarea.SetValue(userPrompt)
		a.textarea.CursorEnd()
	}

	if ready {
		a.viewport.SetContent(a.renderMessages())
		a.viewport.GotoBottom()
	}

	if userPrompt != "" {
		a.SetNotification("⤺ Rewound turn. Prompt restored to input.", NotifyInfo)
		return userPrompt, true
	}
	return "", false
}

// ToggleSidebar switches sidebar visibility on and off. When hidden, chat text
// expands across full width, allowing clean terminal copy/selection without sidebars.
func (a *App) ToggleSidebar() (*App, tea.Cmd) {
	a.sidebarHidden = !a.sidebarHidden
	a.recalcViewport()
	status := "Sidebar visible"
	if a.sidebarHidden {
		status = "Sidebar hidden (clean full-width view for text selection)"
	}
	return a, a.SetNotification(status, NotifyInfo)
}

// IsSidebarHidden reports whether the sidebar is currently hidden.
func (a *App) IsSidebarHidden() bool {
	return a.sidebarHidden
}

// EmitToolStart logs a tool beginning execution in the stream.
func (a *App) EmitToolStart(id, name, args string) {
	a.AddMessage(Message{
		Role: RoleTool,
		Tool: &ToolCall{
			ID:        id,
			Name:      name,
			Args:      args,
			Status:    ToolStatusRunning,
			Timestamp: time.Now(),
		},
	})
}

// EmitToolEnd updates a tool execution with result.
func (a *App) EmitToolEnd(id, output string, success bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := len(a.messages) - 1; i >= 0; i-- {
		if a.messages[i].Tool != nil && a.messages[i].Tool.ID == id {
			t := a.messages[i].Tool
			t.Output = output
			if success {
				t.Status = ToolStatusSuccess
			} else {
				t.Status = ToolStatusFailed
			}
			t.Duration = time.Since(t.Timestamp)
			break
		}
	}
}

// Run launches the Bubble Tea interactive TUI.
func (a *App) Run(ctx context.Context) error {
	defer ResetTerminalBackground()
	SetTerminalBackground(ThemeHex(a.cfg.Theme.BgDark))
	p := tea.NewProgram(a)
	a.mu.Lock()
	a.program = p
	a.mu.Unlock()
	_, err := p.Run()
	return err
}

func waitForStream(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return streamEndMsg{}
		}
		return msg
	}
}

// configureEditorKeys adds the editing shortcuts users expect from Claude Code
// and OpenCode on top of the textarea defaults (Home/End, Ctrl+A/E, Ctrl+K,
// Ctrl+W, Alt+arrows, Ctrl+Home/End are already bound):
//
//	Ctrl+←/→                  move by word
//	Ctrl+Shift+←/→            move by word (terminals that report Shift)
//	Ctrl+Backspace/Delete     delete word backward/forward
//	Ctrl+U                    delete to line start
func configureEditorKeys(ta *textarea.Model) {
	km := &ta.KeyMap
	km.WordForward = key.NewBinding(key.WithKeys("alt+right", "alt+f", "ctrl+right", "ctrl+shift+right"), key.WithHelp("ctrl+→", "word forward"))
	km.WordBackward = key.NewBinding(key.WithKeys("alt+left", "alt+b", "ctrl+left", "ctrl+shift+left"), key.WithHelp("ctrl+←", "word backward"))
	km.DeleteWordBackward = key.NewBinding(key.WithKeys("alt+backspace", "ctrl+w", "ctrl+backspace"), key.WithHelp("ctrl+backspace", "delete word backward"))
	km.DeleteWordForward = key.NewBinding(key.WithKeys("alt+delete", "alt+d", "ctrl+delete"), key.WithHelp("ctrl+delete", "delete word forward"))
}
