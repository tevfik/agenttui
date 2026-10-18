package agenttui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	streamStartTime time.Time
	streamMu        *sync.Mutex
}

// New creates a new extensible Agent TUI instance.
func New(cfg Config) *App {
	if cfg.Title == "" {
		cfg.Title = "Devran"
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

	for _, cmd := range cfg.Commands {
		app.RegisterCommand(cmd)
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

// SetWorkDir updates the active workspace path.
func (a *App) SetWorkDir(dir string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.workDir = dir
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
		".git": true, "node_modules": true, "vendor": true,
		"build": true, ".devran": true, ".yaver": true, ".gleann": true,
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
	p := tea.NewProgram(a)
	a.program = p
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
