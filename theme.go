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

func newTheme(name string, primary, secondary, accent, success, warning, errCol, info, text, subText, muted, border, bgDark color.Color) Theme {
	return Theme{
		Name:      name,
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
			Foreground(bgDark).
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

// ClaudeTheme returns Anthropic Claude Code's signature warm aesthetic.
func ClaudeTheme() Theme {
	return newTheme(
		"claude",
		lipgloss.Color("#DA7756"), // Anthropic Terracotta
		lipgloss.Color("#E5A88B"), // Warm peach
		lipgloss.Color("#D97706"), // Warm amber
		lipgloss.Color("#86EFAC"), // Soft sage green
		lipgloss.Color("#FDE047"), // Soft warm yellow
		lipgloss.Color("#FCA5A5"), // Soft coral red
		lipgloss.Color("#93C5FD"), // Soft sky blue
		lipgloss.Color("#ECE7DF"), // Claude eggshell off-white
		lipgloss.Color("#A8A29E"), // Stone 400
		lipgloss.Color("#78716C"), // Stone 500
		lipgloss.Color("#44403C"), // Stone 700
		lipgloss.Color("#1C1917"), // Stone 900
	)
}

// DevranTheme returns the modern deep violet & cyan theme.
func DevranTheme() Theme {
	return newTheme(
		"devran",
		lipgloss.Color("#8B5CF6"), // Violet
		lipgloss.Color("#06B6D4"), // Cyan
		lipgloss.Color("#F59E0B"), // Amber
		lipgloss.Color("#10B981"), // Emerald
		lipgloss.Color("#F59E0B"), // Amber
		lipgloss.Color("#EF4444"), // Red
		lipgloss.Color("#3B82F6"), // Blue
		lipgloss.Color("#E2E8F0"), // Slate-200
		lipgloss.Color("#94A3B8"), // Slate-400
		lipgloss.Color("#475569"), // Slate-600
		lipgloss.Color("#334155"), // Slate-700
		lipgloss.Color("#0F172A"), // Slate-900
	)
}

// CatppuccinTheme returns the soothing Catppuccin Mocha pastel aesthetic.
func CatppuccinTheme() Theme {
	return newTheme(
		"catppuccin",
		lipgloss.Color("#CBA6F7"), // Mauve
		lipgloss.Color("#B4BEFE"), // Lavender
		lipgloss.Color("#FAB387"), // Peach
		lipgloss.Color("#A6E3A1"), // Green
		lipgloss.Color("#F9E2AF"), // Yellow
		lipgloss.Color("#F38BA8"), // Red
		lipgloss.Color("#89B4FA"), // Blue
		lipgloss.Color("#CDD6F4"), // Text
		lipgloss.Color("#A6ADC8"), // Subtext0
		lipgloss.Color("#6C7086"), // Overlay0
		lipgloss.Color("#45475A"), // Surface1
		lipgloss.Color("#1E1E2E"), // Base
	)
}

// TokyoNightTheme returns the neon cyber aesthetic of Tokyo Night.
func TokyoNightTheme() Theme {
	return newTheme(
		"tokyo-night",
		lipgloss.Color("#7AA2F7"), // Electric Blue
		lipgloss.Color("#BB9AF7"), // Purple
		lipgloss.Color("#FF9E64"), // Orange
		lipgloss.Color("#9ECE6A"), // Green
		lipgloss.Color("#E0AF68"), // Yellow
		lipgloss.Color("#F7768E"), // Red
		lipgloss.Color("#7DCFFF"), // Cyan
		lipgloss.Color("#C0CAF5"), // Text
		lipgloss.Color("#9AA5CE"), // Subtext
		lipgloss.Color("#565F89"), // Muted
		lipgloss.Color("#3B4261"), // Border
		lipgloss.Color("#1A1B26"), // Background
	)
}

// DraculaTheme returns the classic gothic Dracula aesthetic.
func DraculaTheme() Theme {
	return newTheme(
		"dracula",
		lipgloss.Color("#BD93F9"), // Purple
		lipgloss.Color("#FF79C6"), // Pink
		lipgloss.Color("#8BE9FD"), // Cyan
		lipgloss.Color("#50FA7B"), // Green
		lipgloss.Color("#F1FA8C"), // Yellow
		lipgloss.Color("#FF5555"), // Red
		lipgloss.Color("#6272A4"), // Blue
		lipgloss.Color("#F8F8F2"), // Text
		lipgloss.Color("#BFBFBF"), // Subtext
		lipgloss.Color("#6272A4"), // Muted
		lipgloss.Color("#44475A"), // Border
		lipgloss.Color("#282A36"), // Background
	)
}

// NordTheme returns the arctic cold aesthetic of Nord.
func NordTheme() Theme {
	return newTheme(
		"nord",
		lipgloss.Color("#88C0D0"), // Frost Cyan
		lipgloss.Color("#81A1C1"), // Frost Blue
		lipgloss.Color("#EBCB8B"), // Yellow
		lipgloss.Color("#A3BE8C"), // Green
		lipgloss.Color("#D08770"), // Orange
		lipgloss.Color("#BF616A"), // Red
		lipgloss.Color("#5E81AC"), // Deep Blue
		lipgloss.Color("#ECEFF4"), // Snow Storm
		lipgloss.Color("#D8DEE9"), // Snow Dark
		lipgloss.Color("#4C566A"), // Polar Night 3
		lipgloss.Color("#3B4252"), // Polar Night 1
		lipgloss.Color("#2E3440"), // Polar Night 0
	)
}

// GruvboxTheme returns the warm retro aesthetic of Gruvbox Dark.
func GruvboxTheme() Theme {
	return newTheme(
		"gruvbox",
		lipgloss.Color("#FABD2F"), // Bright Yellow
		lipgloss.Color("#FE8019"), // Bright Orange
		lipgloss.Color("#8EC07C"), // Aqua
		lipgloss.Color("#B8BB26"), // Green
		lipgloss.Color("#FABD2F"), // Yellow
		lipgloss.Color("#FB4934"), // Red
		lipgloss.Color("#83A598"), // Blue
		lipgloss.Color("#EBDBB2"), // Light Text
		lipgloss.Color("#D5C4A1"), // Subtext
		lipgloss.Color("#928374"), // Gray
		lipgloss.Color("#504945"), // Dark Border
		lipgloss.Color("#282828"), // Dark Background
	)
}

// OpenCodeTheme returns the signature OpenCode minimalist pitch black, crisp white, vibrant orange, and zinc gray aesthetic.
func OpenCodeTheme() Theme {
	return newTheme(
		"opencode",
		lipgloss.Color("#FF6A00"), // OpenCode Vibrant Orange
		lipgloss.Color("#FB923C"), // Warm Amber Orange
		lipgloss.Color("#EA580C"), // Deep Orange Accent
		lipgloss.Color("#22C55E"), // Emerald Green
		lipgloss.Color("#EAB308"), // Vivid Amber Yellow
		lipgloss.Color("#EF4444"), // Crimson Red
		lipgloss.Color("#38BDF8"), // Electric Cyan
		lipgloss.Color("#FFFFFF"), // Crisp Pure White Text
		lipgloss.Color("#D4D4D8"), // Zinc 300 Light Gray
		lipgloss.Color("#71717A"), // Zinc 500 Medium Gray
		lipgloss.Color("#27272A"), // Zinc 800 Dark Border
		lipgloss.Color("#000000"), // Pitch Black Background
	)
}

// AvailableThemes returns the identifiers of all built-in themes.
func AvailableThemes() []string {
	return []string{
		"claude",
		"devran",
		"opencode",
		"catppuccin",
		"tokyo-night",
		"dracula",
		"nord",
		"gruvbox",
	}
}

// DefaultTheme returns the default theme (ClaudeTheme).
func DefaultTheme() Theme {
	return ClaudeTheme()
}

// ThemeByName returns the matching theme by name, defaulting to Claude.
func ThemeByName(name string) Theme {
	switch name {
	case "opencode", "open-code", "orange":
		return OpenCodeTheme()
	case "devran", "violet":
		return DevranTheme()
	case "catppuccin", "mocha":
		return CatppuccinTheme()
	case "tokyo-night", "tokyonight", "tokyo":
		return TokyoNightTheme()
	case "dracula", "gothic":
		return DraculaTheme()
	case "nord", "arctic":
		return NordTheme()
	case "gruvbox", "retro":
		return GruvboxTheme()
	case "claude", "anthropic", "warm":
		return ClaudeTheme()
	default:
		return ClaudeTheme()
	}
}
