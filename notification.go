package agenttui

import (
	"image/color"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// NotificationType defines the severity of a notification.
type NotificationType int

const (
	NotifyInfo NotificationType = iota
	NotifySuccess
	NotifyWarning
	NotifyError
)

// Notification represents a toast notification in the status bar.
type Notification struct {
	Message string
	Type    NotificationType
	Expires time.Time
}

type notifyExpireMsg struct{}

const defaultNotifyTTL = 5 * time.Second
const shortNotifyTTL = 3 * time.Second

var notificationIndicators = map[NotificationType]struct {
	icon  string
	color color.Color
}{
	NotifyInfo:    {"ℹ", lipgloss.Color("#3B82F6")},
	NotifySuccess: {"✓", lipgloss.Color("#10B981")},
	NotifyWarning: {"⚠", lipgloss.Color("#F59E0B")},
	NotifyError:   {"×", lipgloss.Color("#EF4444")},
}

// SetNotification sets a toast notification.
func (a *App) SetNotification(msg string, ntype NotificationType) tea.Cmd {
	ttl := defaultNotifyTTL
	if ntype == NotifyInfo {
		ttl = shortNotifyTTL
	}
	a.notification = &Notification{
		Message: msg,
		Type:    ntype,
		Expires: time.Now().Add(ttl),
	}
	return tea.Tick(ttl, func(time.Time) tea.Msg {
		return notifyExpireMsg{}
	})
}

func (a *App) renderNotification(maxW int) string {
	if a.notification == nil || time.Now().After(a.notification.Expires) {
		return ""
	}
	ind, ok := notificationIndicators[a.notification.Type]
	if !ok {
		ind = notificationIndicators[NotifyInfo]
	}

	iconStr := lipgloss.NewStyle().Foreground(ind.color).Bold(true).Render(ind.icon)
	textStr := lipgloss.NewStyle().Foreground(a.cfg.Theme.Text).Render(a.notification.Message)

	full := iconStr + " " + textStr
	if maxW > 0 && lipgloss.Width(full) > maxW {
		avail := maxW - lipgloss.Width(iconStr) - 2
		if avail > 3 {
			full = iconStr + " " + lipgloss.NewStyle().Foreground(a.cfg.Theme.Text).Render(a.notification.Message[:avail-1]+"…")
		}
	}
	return full
}
