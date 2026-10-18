package agenttui

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ExportSession writes the conversation history to both Markdown (.md) and standalone HTML (.html).
// Returns the absolute or relative paths to both generated files.
func (a *App) ExportSession(outDir, baseName string) (mdPath, htmlPath string, err error) {
	a.mu.RLock()
	msgs := make([]Message, len(a.messages))
	copy(msgs, a.messages)
	title := a.cfg.Title
	if title == "" {
		title = "Devran AI Chat Session"
	}
	model := a.modelName
	a.mu.RUnlock()

	if outDir == "" {
		if a.workDir != "" {
			outDir = a.workDir
		} else {
			outDir = "."
		}
	}
	if baseName == "" {
		baseName = fmt.Sprintf("session-%s", time.Now().Format("20060102-150405"))
	}
	baseName = strings.TrimSuffix(baseName, ".md")
	baseName = strings.TrimSuffix(baseName, ".html")

	_ = os.MkdirAll(outDir, 0755)
	mdPath = filepath.Join(outDir, baseName+".md")
	htmlPath = filepath.Join(outDir, baseName+".html")

	// 1. Generate Markdown
	var md strings.Builder
	md.WriteString(fmt.Sprintf("# %s\n\n", title))
	md.WriteString(fmt.Sprintf("- **Model:** `%s`\n", model))
	md.WriteString(fmt.Sprintf("- **Date:** %s\n", time.Now().Format("2006-01-02 15:04:05")))
	md.WriteString(fmt.Sprintf("- **Messages:** %d\n\n---\n\n", len(msgs)))

	for _, m := range msgs {
		switch m.Role {
		case RoleUser:
			md.WriteString(fmt.Sprintf("## ❯ You · %s\n\n%s\n\n", m.Timestamp.Format("15:04"), m.Content))
		case RoleAssistant:
			md.WriteString(fmt.Sprintf("## ◈ %s · %s\n\n", title, m.Timestamp.Format("15:04")))
			if m.Thinking != "" {
				md.WriteString("<details>\n<summary>Thinking Process (Thought)</summary>\n\n")
				md.WriteString(m.Thinking)
				md.WriteString("\n\n</details>\n\n")
			}
			md.WriteString(m.Content)
			md.WriteString("\n\n")
		case RoleTool:
			if m.Tool != nil {
				md.WriteString(fmt.Sprintf("> **Tool:** `%s(%s)`\n>\n", m.Tool.Name, m.Tool.Args))
				if m.Tool.Output != "" {
					md.WriteString(fmt.Sprintf("```\n%s\n```\n\n", m.Tool.Output))
				}
			}
		case RoleError:
			md.WriteString(fmt.Sprintf("> ⚠️ **Error:** %s\n\n", m.Content))
		}
		md.WriteString("---\n\n")
	}

	if err := os.WriteFile(mdPath, []byte(md.String()), 0644); err != nil {
		return "", "", fmt.Errorf("write markdown: %w", err)
	}

	// 2. Generate Standalone Beautiful Dark HTML
	var h strings.Builder
	h.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>` + html.EscapeString(title) + `</title>
<style>
  :root {
    --bg: #0f172a;
    --card-bg: #1e293b;
    --text: #f1f5f9;
    --muted: #94a3b8;
    --border: #334155;
    --primary: #8b5cf6;
    --accent: #f59e0b;
    --user-bg: #1e1e38;
    --code-bg: #0b0f19;
  }
  body {
    background: var(--bg);
    color: var(--text);
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    line-height: 1.6;
    margin: 0;
    padding: 2.5rem 1rem;
  }
  .container { max-width: 880px; margin: 0 auto; }
  header { border-bottom: 1px solid var(--border); padding-bottom: 1.25rem; margin-bottom: 2rem; }
  h1 { margin: 0 0 0.5rem 0; font-size: 1.75rem; color: var(--primary); }
  .meta { color: var(--muted); font-size: 0.9rem; }
  .turn { margin-bottom: 1.75rem; padding: 1.25rem; border-radius: 10px; border: 1px solid var(--border); }
  .user { background: var(--user-bg); border-left: 4px solid #38bdf8; }
  .assistant { background: var(--card-bg); border-left: 4px solid var(--primary); }
  .turn-header { font-weight: bold; margin-bottom: 0.75rem; display: flex; justify-content: space-between; font-size: 0.95rem; }
  .turn-title { color: var(--text); }
  .time { color: var(--muted); font-weight: normal; font-size: 0.85rem; }
  pre { background: var(--code-bg); padding: 1rem; border-radius: 6px; overflow-x: auto; border: 1px solid var(--border); font-size: 0.9em; }
  code { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
  details { background: rgba(0,0,0,0.3); border: 1px dashed var(--border); border-radius: 8px; padding: 0.6rem 1rem; margin-bottom: 1rem; }
  summary { cursor: pointer; color: var(--accent); font-weight: 600; outline: none; }
  .thinking-body { font-style: italic; color: #cbd5e1; margin-top: 0.5rem; white-space: pre-wrap; font-size: 0.92rem; }
  .content { white-space: pre-wrap; word-break: break-word; }
  .tool-pill { display: inline-block; background: #334155; color: #f8fafc; font-size: 0.8rem; padding: 0.2rem 0.55rem; border-radius: 4px; margin-bottom: 0.5rem; font-weight: bold; }
</style>
</head>
<body>
<div class="container">
<header>
  <h1>` + html.EscapeString(title) + `</h1>
  <div class="meta">Model: <code>` + html.EscapeString(model) + `</code> · Exported: ` + time.Now().Format("2006-01-02 15:04:05") + ` · ` + fmt.Sprintf("%d turns", len(msgs)) + `</div>
</header>
<main>
`)

	for _, m := range msgs {
		switch m.Role {
		case RoleUser:
			h.WriteString(fmt.Sprintf(`<div class="turn user">
  <div class="turn-header"><span class="turn-title">❯ You</span><span class="time">%s</span></div>
  <div class="content">%s</div>
</div>
`, m.Timestamp.Format("15:04"), html.EscapeString(m.Content)))

		case RoleAssistant:
			thinkingHTML := ""
			if m.Thinking != "" {
				timeStr := ""
				if m.ThinkingTime > 0 {
					timeStr = fmt.Sprintf(" (%s)", m.ThinkingTime.Round(100*time.Millisecond))
				}
				thinkingHTML = fmt.Sprintf(`<details>
  <summary>▶ Thought%s</summary>
  <div class="thinking-body">%s</div>
</details>
`, timeStr, html.EscapeString(m.Thinking))
			}
			h.WriteString(fmt.Sprintf(`<div class="turn assistant">
  <div class="turn-header"><span class="turn-title">◈ %s</span><span class="time">%s</span></div>
  %s
  <div class="content">%s</div>
</div>
`, html.EscapeString(title), m.Timestamp.Format("15:04"), thinkingHTML, html.EscapeString(m.Content)))

		case RoleTool:
			if m.Tool != nil {
				h.WriteString(fmt.Sprintf(`<div class="turn tool" style="background:#131822;border:1px solid #1e293b;">
  <span class="tool-pill">Tool: %s</span>
  <pre><code>%s</code></pre>
</div>
`, html.EscapeString(m.Tool.Name), html.EscapeString(m.Tool.Output)))
			}
		}
	}

	h.WriteString(`</main>
</div>
</body>
</html>`)

	if err := os.WriteFile(htmlPath, []byte(h.String()), 0644); err != nil {
		return mdPath, "", fmt.Errorf("write html: %w", err)
	}

	return mdPath, htmlPath, nil
}
