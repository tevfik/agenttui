package agenttui

import (
	"os"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
)

func TestTermBg_ThemeHex(t *testing.T) {
	assert.Equal(t, "#000000", ThemeHex(OpenCodeTheme().BgDark))
	assert.Equal(t, "#1c1917", strings.ToLower(ThemeHex(ClaudeTheme().BgDark)))
	assert.Equal(t, "#0f172a", strings.ToLower(ThemeHex(DevranTheme().BgDark)))
	assert.Equal(t, "#1e1e2e", strings.ToLower(ThemeHex(CatppuccinTheme().BgDark)))

	// Direct lipgloss.Color
	assert.Equal(t, "#ff6a00", strings.ToLower(ThemeHex(lipgloss.Color("#FF6A00"))))
	assert.Equal(t, "", ThemeHex(nil))
}

func TestTermBg_NoPanicOnDumbOrDisabled(t *testing.T) {
	// Should not crash even in non-terminal or dumb environments
	_ = os.Setenv("AGENTTUI_NO_TERM_BG", "1")
	defer os.Unsetenv("AGENTTUI_NO_TERM_BG")

	SetTerminalBackground("#000000")
	ResetTerminalBackground()
}
