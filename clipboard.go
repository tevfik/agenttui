package agenttui

import (
	"os/exec"
	"strings"
)

// copyToClipboard attempts to copy text to the system clipboard using
// available clipboard tools (xclip, xsel, wl-copy, pbcopy).
func copyToClipboard(text string) bool {
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
		cmd := exec.Command(path, tool.args...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	return false
}
