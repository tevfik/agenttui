package agenttui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme defines the visual styling tokens for the TUI.
// Theme defines the visual styling tokens for the TUI.
type Theme struct {
	Name      string
	Primary   color.Color
	Secondary color.Color
	Accent    color.Color
	Success   color.Color
	Warning   color.Color
	Error     color.Color
	Info      color.Color
	Text      color.Color
	SubText   color.Color
	Muted     color.Color
	Border    color.Color
	BgDark    color.Color

	// Styles
	HeaderTitle lipgloss.Style
	HeaderSub   lipgloss.Style
	UserBubble  lipgloss.Style
	SystemText  lipgloss.Style
	ErrorText   lipgloss.Style
	ToolCard    lipgloss.Style
	StatusBar   lipgloss.Style
	Badge       lipgloss.Style
	PopupBox    lipgloss.Style
	PopupItem   lipgloss.Style
	PopupSel    lipgloss.Style
}

// ClaudeTheme returns Anthropic Claude Code's signature warm aesthetic.
// Characterized by terracotta (#DA7756), warm peach (#E5A88B), eggshell off-white (#ECE7DF),
// and stone slate borders/accents.
func ClaudeTheme() Theme {
	primary := lipgloss.Color("#DA7756")   // Anthropic Terracotta
	secondary := lipgloss.Color("#E5A88B") // Warm peach
	accent := lipgloss.Color("#D97706")    // Warm amber
	success := lipgloss.Color("#86EFAC")   // Soft sage green
	warning := lipgloss.Color("#FDE047")   // Soft warm yellow
	errCol := lipgloss.Color("#FCA5A5")    // Soft coral red
	info := lipgloss.Color("#93C5FD")      // Soft sky blue
	text := lipgloss.Color("#ECE7DF")      // Claude eggshell off-white
	subText := lipgloss.Color("#A8A29E")   // Stone 400
	muted := lipgloss.Color("#78716C")     // Stone 500
	border := lipgloss.Color("#44403C")    // Stone 700
	bgDark := lipgloss.Color("#1C1917")    // Stone 900

	return Theme{
		Name:      "claude",
		Primary:   primary,
		Secondary: secondary,
		Accent:    accent,
		Success:   success,
		Warning:   warning,
		Error:     errCol,
		Info:      info,
		Text:      text,
		SubText:   subText,
		Muted:     muted,
		Border:    border,
		BgDark:    bgDark,

		HeaderTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(primary),

		HeaderSub: lipgloss.NewStyle().
			Foreground(muted),

		UserBubble: lipgloss.NewStyle().
			Bold(true).
			Foreground(secondary),

		SystemText: lipgloss.NewStyle().
			Foreground(muted).
			Italic(true),

		ErrorText: lipgloss.NewStyle().
			Foreground(errCol).
			Bold(true),

		ToolCard: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			Padding(0, 1).
			MarginTop(1).
			MarginBottom(1),

		StatusBar: lipgloss.NewStyle().
			Foreground(muted).
			Padding(0, 1),

		Badge: lipgloss.NewStyle().
			Background(primary).
			Foreground(lipgloss.Color("#1C1917")).
			Padding(0, 1).
			Bold(true),

		PopupBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			Padding(0, 1),

		PopupItem: lipgloss.NewStyle().
			Foreground(muted),

		PopupSel: lipgloss.NewStyle().
			Bold(true).
			Foreground(primary),
	}
}

// DevranTheme returns the modern deep violet & cyan theme.
func DevranTheme() Theme {
	primary := lipgloss.Color("#8B5CF6")   // Violet
	secondary := lipgloss.Color("#06B6D4") // Cyan
	accent := lipgloss.Color("#F59E0B")    // Amber
	success := lipgloss.Color("#10B981")   // Emerald
	warning := lipgloss.Color("#F59E0B")   // Amber
	errCol := lipgloss.Color("#EF4444")    // Red
	info := lipgloss.Color("#3B82F6")      // Blue
	text := lipgloss.Color("#E2E8F0")      // Slate-200
	subText := lipgloss.Color("#94A3B8")   // Slate-400
	muted := lipgloss.Color("#475569")     // Slate-600
	border := lipgloss.Color("#334155")    // Slate-700
	bgDark := lipgloss.Color("#0F172A")    // Slate-900

	return Theme{
		Name:      "devran",
		Primary:   primary,
		Secondary: secondary,
		Accent:    accent,
		Success:   success,
		Warning:   warning,
		Error:     errCol,
		Info:      info,
		Text:      text,
		SubText:   subText,
		Muted:     muted,
		Border:    border,
		BgDark:    bgDark,

		HeaderTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(primary),

		HeaderSub: lipgloss.NewStyle().
			Foreground(muted),

		UserBubble: lipgloss.NewStyle().
			Bold(true).
			Foreground(secondary),

		SystemText: lipgloss.NewStyle().
			Foreground(muted).
			Italic(true),

		ErrorText: lipgloss.NewStyle().
			Foreground(errCol).
			Bold(true),

		ToolCard: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			Padding(0, 1).
			MarginTop(1).
			MarginBottom(1),

		StatusBar: lipgloss.NewStyle().
			Foreground(muted).
			Padding(0, 1),

		Badge: lipgloss.NewStyle().
			Background(primary).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1).
			Bold(true),

		PopupBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			Padding(0, 1),

		PopupItem: lipgloss.NewStyle().
			Foreground(muted),

		PopupSel: lipgloss.NewStyle().
			Bold(true).
			Foreground(primary),
	}
}

// DefaultTheme returns the default theme (ClaudeTheme).
func DefaultTheme() Theme {
	return ClaudeTheme()
}

// ThemeByName returns the matching theme by name ("claude", "devran", "dark"), defaulting to Claude.
func ThemeByName(name string) Theme {
	switch name {
	case "devran", "violet":
		return DevranTheme()
	case "claude", "anthropic", "warm":
		return ClaudeTheme()
	default:
		return ClaudeTheme()
	}
}

