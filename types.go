package agenttui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Role represents the role of a message participant.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
	RoleError     Role = "error"
)

// ToolStatus represents the lifecycle of a tool invocation.
type ToolStatus int

const (
	ToolStatusRunning ToolStatus = iota
	ToolStatusSuccess
	ToolStatusFailed
)

// ToolCall represents an invoked tool within the chat stream.
type ToolCall struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Args      string     `json:"args"`
	Output    string     `json:"output,omitempty"`
	Status    ToolStatus `json:"status"`
	Duration  time.Duration
	Timestamp time.Time
}

// Message represents a message in the conversation.
type Message struct {
	Role         Role          `json:"role"`
	Content      string        `json:"content"`
	Thinking     string        `json:"thinking,omitempty"`
	ThinkingOpen bool          `json:"thinking_open,omitempty"`
	ThinkingTime time.Duration `json:"thinking_time,omitempty"`
	Timestamp    time.Time     `json:"timestamp"`
	Tokens       int           `json:"tokens,omitempty"`
	Tool         *ToolCall     `json:"tool,omitempty"`
}

// StreamHandler is a streaming function called when the user sends a prompt.
// It yields chunks via emit() and returns the total tokens or an error when done.
type StreamHandler func(ctx context.Context, prompt string, emit func(chunk string)) (tokens int, err error)

// EventKind identifies the type of a streaming Event.
type EventKind int

const (
	EventText      EventKind = iota // Text: a chunk of answer content
	EventThinking                   // Text: a chunk of reasoning
	EventToolStart                  // ToolID, ToolName, ToolArgs
	EventToolEnd                    // ToolID, ToolOutput, ToolOK
)

// Event is one typed item of an agent response stream.
type Event struct {
	Kind       EventKind
	Text       string
	ToolID     string
	ToolName   string
	ToolArgs   string
	ToolOutput string
	ToolOK     bool
}

// EventStreamHandler is the typed alternative to StreamHandler. Reasoning and
// tool calls are reported as separate events instead of being inferred from
// <think> tags in the text.
type EventStreamHandler func(ctx context.Context, prompt string, emit func(Event)) (tokens int, err error)

// Store persists the conversation so a session can be resumed. Load is called
// once by New; Save after every completed turn.
type Store interface {
	Load() ([]Message, error)
	Save([]Message) error
}

// SlashCommand defines an interactive command starting with '/'.
type SlashCommand struct {
	Name        string // e.g. "help", "model", "clear"
	Description string // Short description shown in autocomplete popup
	Usage       string // Parameter usage info, e.g. "<model-name>"
	Handler     func(ctx context.Context, args []string, app *App) (result string, isLocal bool, err error)
}

// Widget represents an optional sidecar or overlay panel (e.g. Plan Tracker, Memory Tree).
type Widget interface {
	Title() string
	Render(width, height int) string
	Update(msg tea.Msg) (Widget, tea.Cmd)
	Visible() bool
}

// Config holds initialization parameters for the Agent TUI.
type Config struct {
	Title       string
	Subtitle    string
	ModelName   string
	WorkDir     string
	Mode        string // e.g. "build" or "plan"
	Placeholder string
	Theme       Theme
	Stream      StreamHandler
	// StreamEvents, when set, takes precedence over Stream.
	StreamEvents EventStreamHandler
	Store        Store    // optional session persistence
	IgnoreDirs   []string // extra directory names skipped by @file completion
	Commands     []SlashCommand
	Sidebar      Widget
	EnableMouse  bool
	ShowHeader   bool
	ShowFooter   bool
}
