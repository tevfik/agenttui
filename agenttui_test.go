package agenttui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApp_NewAndSlashCommands(t *testing.T) {
	app := New(Config{
		Title:     "Test Chat",
		ModelName: "test-model",
		Theme:     DefaultTheme(),
	})

	require.NotNil(t, app)
	assert.Equal(t, "test-model", app.modelName)

	// Built-in /help command
	helpCmd, ok := app.commands["help"]
	require.True(t, ok)
	out, isLocal, err := helpCmd.Handler(context.Background(), nil, app)
	require.NoError(t, err)
	assert.True(t, isLocal)
	assert.Contains(t, out, "Available Slash Commands")
	assert.Contains(t, out, "/help")
	assert.Contains(t, out, "/clear")

	// Custom slash command registration
	app.RegisterCommand(SlashCommand{
		Name:        "ping",
		Description: "Check latency",
		Handler: func(ctx context.Context, args []string, a *App) (string, bool, error) {
			return "pong!", true, nil
		},
	})

	pingCmd, ok := app.commands["ping"]
	require.True(t, ok)
	out, _, err = pingCmd.Handler(context.Background(), nil, app)
	require.NoError(t, err)
	assert.Equal(t, "pong!", out)
}

func TestApp_Autocomplete(t *testing.T) {
	app := New(Config{
		Theme: DefaultTheme(),
	})

	app.RegisterCommand(SlashCommand{
		Name:        "plan",
		Description: "Create a plan",
	})
	app.RegisterCommand(SlashCommand{
		Name:        "patch",
		Description: "Apply patch",
	})
	app.RegisterCommand(SlashCommand{
		Name:        "model",
		Description: "Switch model",
	})

	// When typing "/p", matches "plan" and "patch"
	app.textarea.SetValue("/p")
	app.updateAutocomplete()
	assert.True(t, app.autoActive)
	require.Len(t, app.autoFiltered, 2)
	assert.Equal(t, "plan", app.autoFiltered[0].Name)
	assert.Equal(t, "patch", app.autoFiltered[1].Name)

	// When typing "/mo", matches "model"
	app.textarea.SetValue("/mo")
	app.updateAutocomplete()
	assert.True(t, app.autoActive)
	require.Len(t, app.autoFiltered, 1)
	assert.Equal(t, "model", app.autoFiltered[0].Name)

	// When space is entered "/model ", autocomplete closes
	app.textarea.SetValue("/model ")
	app.updateAutocomplete()
	assert.False(t, app.autoActive)
}

func TestApp_ToolLifecycle(t *testing.T) {
	app := New(Config{
		Theme: DefaultTheme(),
	})

	app.EmitToolStart("t1", "grep_search", "Query: foo")
	require.Len(t, app.messages, 1)
	assert.Equal(t, RoleTool, app.messages[0].Role)
	assert.Equal(t, ToolStatusRunning, app.messages[0].Tool.Status)

	app.EmitToolEnd("t1", "3 matches found", true)
	require.Len(t, app.messages, 1)
	assert.Equal(t, ToolStatusSuccess, app.messages[0].Tool.Status)
	assert.Equal(t, "3 matches found", app.messages[0].Tool.Output)

	rendered := app.renderMessages()
	assert.Contains(t, rendered, "Tool: grep_search(Query: foo)")
	assert.Contains(t, rendered, "3 matches found")
}

func TestApp_StreamHandling(t *testing.T) {
	app := New(Config{
		Theme: DefaultTheme(),
		Stream: func(ctx context.Context, prompt string, emit func(chunk string)) (int, error) {
			emit("Hello ")
			emit("world!")
			return 10, nil
		},
	})

	// Mock window size
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Submit message
	cmd := app.handleSubmit("hello there")
	require.NotNil(t, cmd)
	assert.True(t, app.streaming)

	// Process stream chunks
	app.Update(streamChunkMsg{chunk: "Hello "})
	app.Update(streamChunkMsg{chunk: "world!"})
	app.Update(streamDoneMsg{tokens: 10, err: nil})

	assert.False(t, app.streaming)
	assert.Equal(t, 10, app.totalTokens)
	rendered := app.renderMessages()
	assert.Contains(t, rendered, "hello there")
	assert.Contains(t, rendered, "Hello world")
}

// Dummy Widget for testing
type mockWidget struct {
	visible bool
}

func (m *mockWidget) Title() string                   { return "Mock Sidebar" }
func (m *mockWidget) Render(w, h int) string          { return "Sidebar Content" }
func (m *mockWidget) Update(msg tea.Msg) (Widget, tea.Cmd) { return m, nil }
func (m *mockWidget) Visible() bool                   { return m.visible }

func TestApp_SidebarWidget(t *testing.T) {
	app := New(Config{
		Theme:      DefaultTheme(),
		ShowHeader: true,
		ShowFooter: true,
		Sidebar:    &mockWidget{visible: true},
	})

	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := app.View()
	assert.True(t, view.AltScreen)
	assert.Contains(t, view.Content, "Sidebar Content")
}

func TestApp_AutocompleteEnterExecution(t *testing.T) {
	app := New(Config{
		Theme: DefaultTheme(),
	})
	app.initDone = true

	planExecuted := false
	app.RegisterCommand(SlashCommand{
		Name:  "plan",
		Usage: "[task]",
		Handler: func(ctx context.Context, args []string, a *App) (string, bool, error) {
			planExecuted = true
			return "plan created", true, nil
		},
	})

	app.RegisterCommand(SlashCommand{
		Name:  "search",
		Usage: "<query>",
		Handler: func(ctx context.Context, args []string, a *App) (string, bool, error) {
			return "search results", true, nil
		},
	})

	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// 1. Typing "/pl" matches "plan" (optional args: "[task]") -> pressing Enter must immediately execute!
	app.textarea.SetValue("/pl")
	app.updateAutocomplete()
	require.True(t, app.autoActive)
	require.Len(t, app.autoFiltered, 1)

	_, cmd := app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, app.autoActive)
	assert.NotNil(t, cmd)
	// Execute the command batch
	if cmd != nil {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, subCmd := range batch {
				if subCmd != nil {
					subMsg := subCmd()
					app.Update(subMsg)
				}
			}
		} else {
			app.Update(msg)
		}
	}
	assert.True(t, planExecuted, "Slash command without mandatory args must execute immediately on Enter")

	// 2. Typing "/se" matches "search" (mandatory args: "<query>") -> pressing Enter must prefill "/search "
	app.textarea.SetValue("/se")
	app.updateAutocomplete()
	require.True(t, app.autoActive)
	require.Len(t, app.autoFiltered, 1)

	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, app.autoActive)
	assert.Equal(t, "/search ", app.textarea.Value(), "Command with mandatory args must prefill textarea on Enter")
}

func TestApp_ThemesAndResponsiveScaling(t *testing.T) {
	// 1. Responsive sidebar widths
	assert.Equal(t, 0, sidebarWidthFor(80), "Narrow screen (<100) should disable sidebar")
	assert.Equal(t, 24, sidebarWidthFor(120), "Medium screen (100-139) should use 24-col sidebar")
	assert.Equal(t, 30, sidebarWidthFor(160), "Wide screen (>=140) should use 30-col sidebar")

	// 2. Theme palettes
	claude := ClaudeTheme()
	assert.Equal(t, "claude", claude.Name)

	devran := DevranTheme()
	assert.Equal(t, "devran", devran.Name)

	assert.Equal(t, "claude", ThemeByName("claude").Name)
	assert.Equal(t, "devran", ThemeByName("devran").Name)
	assert.Equal(t, "claude", ThemeByName("unknown").Name)

	// 3. Welcome hero banner rendering
	app := New(Config{
		Theme:     claude,
		ModelName: "test-model",
	})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	rendered := app.renderMessages()
	assert.Contains(t, rendered, "Autonomous Software Engineering Agent")
	assert.Contains(t, rendered, "/plan")
	assert.Contains(t, rendered, "/search")
}

func TestApp_NonLocalSlashCommandStartsStream(t *testing.T) {
	streamStarted := false
	app := New(Config{
		Theme: DefaultTheme(),
		Stream: func(ctx context.Context, prompt string, emit func(chunk string)) (int, error) {
			streamStarted = true
			assert.Contains(t, prompt, "Enriched RAG context")
			emit("Answer from LLM")
			return 5, nil
		},
	})
	app.initDone = true

	// Register /ask-like command returning isLocal: false
	app.RegisterCommand(SlashCommand{
		Name:  "ask",
		Usage: "<question>",
		Handler: func(ctx context.Context, args []string, a *App) (string, bool, error) {
			return "Enriched RAG context for question: " + strings.Join(args, " "), false, nil
		},
	})

	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Submit /ask what is this
	cmd := app.handleSubmit("/ask what is this")
	require.NotNil(t, cmd)

	// Process batch message
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, subCmd := range batch {
			if subCmd != nil {
				resMsg := subCmd()
				_, followCmd := app.Update(resMsg)
				if followCmd != nil {
					fMsg := followCmd()
					if fBatch, ok := fMsg.(tea.BatchMsg); ok {
						for _, fSub := range fBatch {
							if fSub != nil {
								streamMsg := fSub()
								app.Update(streamMsg)
							}
						}
					}
				}
			}
		}
	}

	assert.True(t, streamStarted, "Non-local slash command (like /ask) must invoke Stream handler")
}

func TestApp_SlashCommandDuringStreaming(t *testing.T) {
	app := New(Config{
		Theme: DefaultTheme(),
	})
	app.initDone = true

	planRan := false
	app.RegisterCommand(SlashCommand{
		Name: "plan",
		Handler: func(ctx context.Context, args []string, a *App) (string, bool, error) {
			planRan = true
			return "plan created", true, nil
		},
	})

	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Simulate an active stream
	app.streaming = true
	app.streamBuf.WriteString("partial response...")

	// User types "/plan" and presses Enter while streaming is still active
	app.textarea.SetValue("/plan")
	app.updateAutocomplete()
	require.True(t, app.autoActive)

	_, cmd := app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.NotNil(t, cmd)

	// Stream should have been canceled and reset
	assert.False(t, app.streaming)

	// Execute command batch
	if cmd != nil {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, subCmd := range batch {
				if subCmd != nil {
					resMsg := subCmd()
					app.Update(resMsg)
				}
			}
		}
	}

	assert.True(t, planRan, "Slash command must be executable while streaming")
}

func TestApp_TokenBar(t *testing.T) {
	app := New(Config{
		Theme: DefaultTheme(),
	})
	app.SetContextTokens(64000, 128000)

	bar := app.renderTokenBar(10)
	assert.Contains(t, bar, "64.0k/128k")
	assert.Contains(t, bar, "50%")
}

func TestApp_InstantShellExecution(t *testing.T) {
	app := New(Config{
		Theme: DefaultTheme(),
	})
	app.initDone = true
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Submit "!echo hello shell"
	cmd := app.handleSubmit("!echo hello shell")
	require.NotNil(t, cmd)

	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, subCmd := range batch {
			if subCmd != nil {
				res := subCmd()
				app.Update(res)
			}
		}
	}

	rendered := app.renderMessages()
	assert.Contains(t, rendered, "hello shell")
}

func TestApp_AtFileAutocompleteAndExpansion(t *testing.T) {
	app := New(Config{
		Theme: DefaultTheme(),
	})
	app.initDone = true
	app.SetRepoFiles([]string{"cmd/devran/main.go", "pkg/agenttui/model.go", "README.md"})
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// 1. Autocomplete test with "@model"
	app.textarea.SetValue("look at @model")
	app.updateAutocomplete()
	assert.True(t, app.autoActive)
	assert.Equal(t, "at", app.autoMode)
	require.Len(t, app.autoAtFiles, 1)
	assert.Equal(t, "pkg/agenttui/model.go", app.autoAtFiles[0])

	// 2. Tab completion
	app.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.False(t, app.autoActive)
	assert.Contains(t, app.textarea.Value(), "@pkg/agenttui/model.go ")

	// 3. Space key completion
	app.textarea.SetValue("/read @README")
	app.updateAutocomplete()
	assert.True(t, app.autoActive)
	assert.Equal(t, "at", app.autoMode)
	app.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	assert.False(t, app.autoActive)
	assert.Equal(t, "/read @README.md ", app.textarea.Value())

	// 4. File ranking prioritization: prefix/basename beats arbitrary suffix/syllable
	files := []string{
		"cmd/codesearch/search_test.go",
		"internal/agent/agent_test.go",
		"TEST_PLAN.md",
		"pkg/testutils/mock.go",
	}
	ranked := rankFileMatches(files, "test")
	require.NotEmpty(t, ranked)
	assert.Equal(t, "TEST_PLAN.md", ranked[0], "TEST_PLAN.md should rank first for 'test' query over *_test.go suffixes")

	// 5. File expansion test
	app.workDir = "."
	prompt := "Explain this file: @README.md"
	expanded := app.expandFileReferences(prompt)
	assert.Contains(t, expanded, "<file path=\"README.md\">")
}

func TestApp_MarkdownRendering(t *testing.T) {
	app := New(Config{
		Theme: DefaultTheme(),
	})
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// 1. Assistant message with markdown
	app.AddMessage(Message{
		Role:    RoleAssistant,
		Content: "# Plan\nHere is a list:\n- Item 1\n- Item 2\n\n```go\nfunc Hello() {}\n```",
	})

	// 2. System message with markdown (like /read output)
	app.AddMessage(Message{
		Role:    RoleSystem,
		Content: "**README.md**\n```markdown\n# Devran\n```\n_Success reading file._",
	})

	rendered := app.renderMessages()
	t.Logf("RENDERED OUTPUT:\n%s", rendered)

	// Verify Assistant markdown was parsed:
	// - `# Plan` becomes formatted heading without leading `#`
	// - `- Item 1` becomes bullet list with syntax tokens
	assert.Contains(t, rendered, "Plan")
	assert.Contains(t, rendered, "Item")
	assert.Contains(t, rendered, "Hello")

	// Verify System markdown (/read output) was parsed:
	// - `_Success reading file._` becomes italicized without literal `_`
	assert.NotContains(t, rendered, "_Success reading file._")
	assert.Contains(t, rendered, "Success reading file.")
}

func TestApp_Scrollbar(t *testing.T) {
	app := New(Config{
		Theme: DefaultTheme(),
	})
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Add enough messages to overflow viewport
	for i := 0; i < 40; i++ {
		app.AddMessage(Message{
			Role:    RoleUser,
			Content: fmt.Sprintf("Message number %d with some content to fill lines", i),
		})
	}
	app.viewport.SetContent(app.renderMessages())

	sb := app.renderScrollbar(20)
	assert.NotEmpty(t, sb)
	assert.Contains(t, sb, "▓", "Scrollbar should render thumb ▓ when totalLines > visibleLines")
}

func TestApp_HeaderNoDuplicateTokenBar(t *testing.T) {
	app := New(Config{
		Theme:      DefaultTheme(),
		ShowHeader: true,
	})
	app.SetContextTokens(1000, 100000)

	// Wide screen (120 cols): sidebar is visible, header should NOT have token bar
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	viewWide := app.View().Content
	headerLine := strings.Split(viewWide, "\n")[0]
	assert.NotContains(t, headerLine, "1.0k/100k", "Header should not duplicate token bar when sidebar is visible")

	// Narrow screen (80 cols): sidebar is hidden, header SHOULD show compact token bar
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	viewNarrow := app.View().Content
	headerNarrow := strings.Split(viewNarrow, "\n")[0]
	assert.Contains(t, headerNarrow, "1.0k/100k", "Header should show compact token bar when sidebar is hidden")
}

func TestApp_ClaudeStyleFraming(t *testing.T) {
	app := New(Config{
		Theme: DefaultTheme(),
	})
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Add User message
	app.AddMessage(Message{
		Role:    RoleUser,
		Content: "Can you analyze this project?",
	})

	// Add Assistant message
	app.AddMessage(Message{
		Role:    RoleAssistant,
		Content: "Sure, here is the project architecture overview.",
	})

	// Add Error message
	app.AddMessage(Message{
		Role:    RoleError,
		Content: "Failed to connect to backend",
	})

	rendered := app.renderMessages()

	// Verify User prompt divider banner with identity badge (── ❯ You ...)
	assert.Contains(t, rendered, "── ")
	assert.Contains(t, rendered, "❯ You")
	assert.Contains(t, rendered, "Can you analyze this project?")

	// Verify Assistant message has identity indicator
	assert.Contains(t, rendered, "◈ Devran")
	assert.Contains(t, rendered, "architecture")
	assert.Contains(t, rendered, "overview")

	// Verify Error card framing
	assert.Contains(t, rendered, "✗ Error")
	assert.Contains(t, rendered, "Failed to connect to backend")
}
