package agenttui

import (
	"fmt"
	"image/color"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
)

// SetTerminalBackground updates the terminal emulator window's background color via OSC 11.
// Most modern terminals (GNOME Terminal, Windows Terminal, Kitty, Alacritty, WezTerm, macOS Terminal, Foot, Ghostty)
// support this sequence to dynamically adapt their background to the active theme.
func SetTerminalBackground(hexColor string) {
	if hexColor == "" || os.Getenv("TERM") == "dumb" {
		return
	}
	if os.Getenv("AGENTTUI_NO_TERM_BG") != "" || os.Getenv("DEVRAN_NO_TERM_BG") != "" {
		return
	}
	fd := os.Stdout.Fd()
	if !isatty.IsTerminal(fd) && !isatty.IsCygwinTerminal(fd) {
		return
	}

	seq := fmt.Sprintf("\033]11;%s\007", hexColor)
	if os.Getenv("TMUX") != "" {
		seq = fmt.Sprintf("\033Ptmux;\033\033]11;%s\007\033\\", hexColor)
	}
	_, _ = os.Stdout.WriteString(seq)
}

// ResetTerminalBackground restores the terminal emulator window's default background color via OSC 111.
func ResetTerminalBackground() {
	if os.Getenv("TERM") == "dumb" {
		return
	}
	if os.Getenv("AGENTTUI_NO_TERM_BG") != "" || os.Getenv("DEVRAN_NO_TERM_BG") != "" {
		return
	}
	fd := os.Stdout.Fd()
	if !isatty.IsTerminal(fd) && !isatty.IsCygwinTerminal(fd) {
		return
	}

	seq := "\033]111\007"
	if os.Getenv("TMUX") != "" {
		seq = "\033Ptmux;\033\033]111\007\033\\"
	}
	_, _ = os.Stdout.WriteString(seq)
}

// ThemeHex converts a color.Color (like lipgloss.Color) to a hex string format like "#000000".
func ThemeHex(c color.Color) string {
	if c == nil {
		return ""
	}
	s := fmt.Sprint(c)
	if strings.HasPrefix(s, "#") {
		return s
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}
