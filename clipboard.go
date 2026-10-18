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

// ExtractCodeBlocks extracts the raw code within markdown fenced code blocks (```...```).
// If no code fences are found, the original text is returned.
func ExtractCodeBlocks(text string) string {
	lines := strings.Split(text, "\n")
	var blocks []string
	var curBlock strings.Builder
	inBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
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

// CopyToClipboard copies arbitrary text to the system clipboard.
func (a *App) CopyToClipboard(text string) bool {
	return copyToClipboard(text)
}
