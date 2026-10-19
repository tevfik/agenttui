package agenttui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Item represents a generic item in a status list widget.
type Item struct {
	Text   string
	Status string // "pending", "in_progress", "done", "failed"
}

// StatusListWidget displays a vertical list of tasks/steps with status icons.
type StatusListWidget struct {
	title   string
	items   []Item
	visible bool
	theme   Theme
}

// NewStatusListWidget creates a status list widget.
func NewStatusListWidget(title string, theme Theme) *StatusListWidget {
	return &StatusListWidget{
		title:   title,
		visible: true,
		theme:   theme,
	}
}

// SetItems updates the list items.
func (w *StatusListWidget) SetItems(items []Item) {
	w.items = items
}

// SetVisible controls widget visibility.
func (w *StatusListWidget) SetVisible(v bool) {
	w.visible = v
}

// Title returns the widget title.
func (w *StatusListWidget) Title() string {
	return w.title
}

// Visible returns whether the widget is visible.
func (w *StatusListWidget) Visible() bool {
	return w.visible && len(w.items) > 0
}

// Update handles Bubble Tea messages.
func (w *StatusListWidget) Update(msg tea.Msg) (Widget, tea.Cmd) {
	return w, nil
}

// Render renders the widget into the given dimensions.
func (w *StatusListWidget) Render(width, height int) string {
	if !w.visible || len(w.items) == 0 {
		return ""
	}

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(w.theme.Primary).
		Render("📋 " + w.title)

	var lines []string
	lines = append(lines, header)

	for _, it := range w.items {
		var icon string
		switch it.Status {
		case "done", "completed":
			icon = lipgloss.NewStyle().Foreground(w.theme.Success).Render("✓ ")
		case "in_progress", "running":
			icon = lipgloss.NewStyle().Foreground(w.theme.Warning).Render("⟳ ")
		case "failed", "error":
			icon = lipgloss.NewStyle().Foreground(w.theme.Error).Render("✗ ")
		default:
			icon = lipgloss.NewStyle().Foreground(w.theme.Muted).Render("○ ")
		}

		itemText := it.Text
		maxTextW := width - 6
		if maxTextW > 0 && len(itemText) > maxTextW {
			itemText = itemText[:maxTextW] + "…"
		}

		lines = append(lines, icon+itemText)
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(w.theme.Border).
		Width(width-2).
		Height(height-2).
		Padding(0, 1)

	return box.Render(strings.Join(lines, "\n"))
}

// KeyValWidget displays key-value pairs (e.g. system info, index stats).
type KeyValWidget struct {
	title   string
	pairs   [][2]string
	visible bool
	theme   Theme
}

// NewKeyValWidget creates a key-value widget.
func NewKeyValWidget(title string, theme Theme) *KeyValWidget {
	return &KeyValWidget{
		title:   title,
		visible: true,
		theme:   theme,
	}
}

// SetPairs updates the key-value pairs.
func (w *KeyValWidget) SetPairs(pairs [][2]string) {
	w.pairs = pairs
}

// Title returns the widget title.
func (w *KeyValWidget) Title() string {
	return w.title
}

// Visible returns visibility status.
func (w *KeyValWidget) Visible() bool {
	return w.visible && len(w.pairs) > 0
}

// Update handles Bubble Tea messages.
func (w *KeyValWidget) Update(msg tea.Msg) (Widget, tea.Cmd) {
	return w, nil
}

// Render renders the key-value widget.
func (w *KeyValWidget) Render(width, height int) string {
	if !w.visible || len(w.pairs) == 0 {
		return ""
	}

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(w.theme.Secondary).
		Render("ℹ " + w.title)

	var lines []string
	lines = append(lines, header)

	for _, p := range w.pairs {
		k := lipgloss.NewStyle().Foreground(w.theme.Muted).Render(p[0] + ": ")
		v := lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB")).Render(p[1])
		lines = append(lines, fmt.Sprintf("%s%s", k, v))
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(w.theme.Border).
		Width(width-2).
		Height(height-2).
		Padding(0, 1)

	return box.Render(strings.Join(lines, "\n"))
}
