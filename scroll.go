package agenttui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Scroll model
//
// The transcript follows the bottom by default (scrollAway == false): every
// content change jumps to the last line. As soon as the user scrolls up
// (PgUp, wheel, arrows, Home ...) scrollAway turns on and new content no longer
// moves the view; unseen records that content arrived below. Scrolling back to
// the last line (End, PgDn, wheel) or sending a message turns following on again.

// syncView re-renders the transcript into the viewport, honouring follow mode.
func (a *App) syncView() { a.syncContent(a.renderMessages()) }

// syncContent installs content into the viewport, honouring follow mode.
func (a *App) syncContent(s string) {
	before := a.viewport.TotalLineCount()
	a.viewport.SetContent(s)
	if !a.scrollAway {
		a.viewport.GotoBottom()
		return
	}
	if a.viewport.TotalLineCount() > before {
		a.unseen = true
	}
	a.afterScroll()
}

// followBottom resumes follow mode and jumps to the last line.
func (a *App) followBottom() {
	a.scrollAway = false
	a.unseen = false
	a.viewport.GotoBottom()
}

// keepFollow re-pins the view to the bottom after a height change, but only in
// follow mode.
func (a *App) keepFollow() {
	if !a.scrollAway {
		a.viewport.GotoBottom()
	}
}

// afterScroll derives follow mode from the position after a scroll operation.
func (a *App) afterScroll() {
	if a.viewport.AtBottom() {
		a.scrollAway = false
		a.unseen = false
		return
	}
	a.scrollAway = true
}

// scrollBy scrolls n lines (negative = up) and updates follow mode.
func (a *App) scrollBy(n int) {
	if n < 0 {
		a.viewport.ScrollUp(-n)
	} else {
		a.viewport.ScrollDown(n)
	}
	a.afterScroll()
}

// handleScrollKey implements the transcript scrolling keys. It reports whether
// the key was consumed.
func (a *App) handleScrollKey(key string) bool {
	switch key {
	case "pgup":
		a.viewport.PageUp()
	case "pgdown", "ctrl+f":
		a.viewport.PageDown()
	case "ctrl+home":
		a.viewport.GotoTop()
	case "ctrl+end":
		a.viewport.GotoBottom()
	case "alt+up":
		a.viewport.ScrollUp(1)
	case "alt+down":
		a.viewport.ScrollDown(1)
	case "home", "end", "up", "down":
		// Plain navigation keys belong to the input box unless it is empty,
		// and (for up/down) to history/popups unless a response is running.
		if strings.TrimSpace(a.textarea.Value()) != "" || a.autoActive {
			return false
		}
		switch key {
		case "home":
			a.viewport.GotoTop()
		case "end":
			a.viewport.GotoBottom()
		case "up", "down":
			if !a.busy() {
				return false
			}
			if key == "up" {
				a.viewport.ScrollUp(1)
			} else {
				a.viewport.ScrollDown(1)
			}
		}
	default:
		return false
	}
	a.afterScroll()
	return true
}

// scrollBadge is the indicator drawn on the last viewport row while the view
// is not following the bottom ("" when following).
func (a *App) scrollBadge() string {
	if !a.scrollAway {
		return ""
	}
	if a.unseen {
		return " ↓ new content · End "
	}
	return " ↓ End to follow "
}

// overlayBadge right-aligns the badge on a rendered viewport row of width w.
func (a *App) overlayBadge(line, badge string, w int) string {
	bw := lipgloss.Width(badge)
	if bw >= w {
		return line
	}
	st := lipgloss.NewStyle().Bold(true).Foreground(a.cfg.Theme.BgDark).Background(a.cfg.Theme.Primary)
	return ansi.Truncate(line, w-bw, "") + strings.Repeat(" ", max(0, w-bw-ansi.StringWidth(ansi.Truncate(line, w-bw, "")))) + st.Render(badge)
}

// hardWrapLines wraps every line of s wider than w cells (ANSI and wide
// characters aware) so that nothing is silently cut off by the viewport.
func hardWrapLines(s string, w int) string {
	if w < 8 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if ansi.StringWidth(l) > w {
			lines[i] = ansi.Hardwrap(l, w, true)
		}
	}
	return strings.Join(lines, "\n")
}

// renderStreamIfDue redraws the streaming tail when chunks are pending and the
// throttle allows it. The minimum gap adapts to the cost of the last render
// (about 2x), so very long answers cannot starve the event loop.
func (a *App) renderStreamIfDue() {
	if !a.streamDirty || time.Since(a.lastStreamDraw) < a.streamDrawGap {
		return
	}
	t0 := time.Now()
	a.syncContent(a.renderMessagesfast())
	d := time.Since(t0)
	a.lastStreamDraw = time.Now()
	a.streamDrawGap = min(max(2*d, 30*time.Millisecond), time.Second)
	a.streamDirty = false
}
