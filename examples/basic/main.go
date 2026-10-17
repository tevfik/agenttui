package main

import (
	"context"
	"fmt"
	"time"

	"github.com/tevfik/agenttui"
)

func main() {
	app := agenttui.New(agenttui.Config{
		Title:      "AgentTUI Demo",
		ModelName:  "qwen3.5:9b",
		Theme:      agenttui.ClaudeTheme(),
		ShowHeader: true,
		Stream: func(ctx context.Context, prompt string, emit func(chunk string)) (int, error) {
			emit(fmt.Sprintf("I received your prompt: **%s**\n\n", prompt))
			time.Sleep(150 * time.Millisecond)
			emit("Here is how I can assist you:\n\n")
			emit("- Point 1: Autonomous agent execution\n")
			time.Sleep(100 * time.Millisecond)
			emit("- Point 2: File expansion with `@file`\n")
			time.Sleep(100 * time.Millisecond)
			emit("- Point 3: Live interactive slash commands\n\n")
			emit("```go\nfunc Ready() bool {\n    return true\n}\n```\n")
			return 55, nil
		},
	})

	// Add built-in command
	app.RegisterCommand(agenttui.SlashCommand{
		Name:        "ping",
		Description: "Simple healthcheck ping command",
		Handler: func(ctx context.Context, args []string, a *agenttui.App) (string, bool, error) {
			return "pong! 🏓", true, nil
		},
	})

	// Scan repository for @file completion
	go app.ScanRepoFiles(".")

	ctx := context.Background()
	if err := app.Run(ctx); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
