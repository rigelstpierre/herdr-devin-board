package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/rigelstpierre/herdr-devin-board/internal/board"
	"github.com/rigelstpierre/herdr-devin-board/internal/github"
)

const (
	defaultWidth  = 80
	statusWidth   = 11
	minTitleWidth = 10
	footer        = "[enter] open  [p] open PR  [s] ssh  [a] show all  [r] refresh  [q] quit"
)

var (
	styleHeader = lipgloss.NewStyle().Bold(true)
	styleError  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	styleDim    = lipgloss.NewStyle().Faint(true)
	kindStyles  = map[board.Kind]lipgloss.Style{
		board.Running:   lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		board.Waiting:   lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true),
		board.Suspended: styleDim,
		board.Finished:  styleDim,
		board.Errored:   lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
	}
)

func (m Model) View() string {
	sessions := m.sessions()
	var b strings.Builder
	b.WriteString(styleHeader.Render(m.header(len(sessions))) + "\n")
	if m.err != nil {
		b.WriteString(styleError.Render(m.err.Error()) + "\n")
	}
	b.WriteString("\n")
	switch {
	case !m.loaded && m.err == nil:
		b.WriteString(styleDim.Render("Loading sessions…") + "\n")
	case m.loaded && len(sessions) == 0:
		b.WriteString(styleDim.Render(emptyText(m.showAll)) + "\n")
	}
	lines := flatten(sessions)
	end := min(len(lines), m.offset+m.visibleRows())
	for i := m.offset; i < end; i++ {
		l := lines[i]
		prefix := "  "
		if i == m.cursor {
			prefix = "› "
		}
		session := sessions[l.session]
		if l.pr < 0 {
			b.WriteString(prefix + m.sessionLine(session) + "\n")
		} else {
			b.WriteString(prefix + prLine(session.PRs[l.pr]) + "\n")
		}
	}
	b.WriteString("\n" + styleDim.Render(footer) + "\n")
	return b.String()
}

func (m Model) header(count int) string {
	parts := []string{"Devin Cloud", pluralize(count, "session")}
	if m.loaded {
		parts = append(parts, "refreshed "+relative(m.deps.Now(), m.refreshedAt))
	}
	if m.loading {
		parts = append(parts, "refreshing…")
	}
	if m.showAll {
		parts = append(parts, "showing all")
	}
	return strings.Join(parts, " · ")
}

func (m Model) sessionLine(s board.Session) string {
	width := m.width
	if width == 0 {
		width = defaultWidth
	}
	ago := relative(m.deps.Now(), s.UpdatedAt)
	titleWidth := max(width-2-statusWidth-2-2-len(ago), minTitleWidth)
	status := kindStyles[s.Kind].Render(fmt.Sprintf("%-*s", statusWidth, kindLabel(s.Kind)))
	return fmt.Sprintf("%s  %-*s  %s", status, titleWidth, truncate(s.Title, titleWidth), ago)
}

func prLine(pr board.PR) string {
	detail := strings.TrimSpace(ciText(pr.CI) + "  " + reviewText(pr.Review))
	if pr.Unknown {
		detail = styleDim.Render("? ?")
	}
	return strings.TrimRight(fmt.Sprintf("    #%d %s  %-6s %s", pr.Number, pr.Repo, pr.State, detail), " ")
}

func kindLabel(k board.Kind) string {
	switch k {
	case board.Waiting:
		return "◐ waiting"
	case board.Suspended:
		return "◌ suspended"
	case board.Finished:
		return "○ finished"
	case board.Errored:
		return "✗ error"
	}
	return "● running"
}

func ciText(ci github.CI) string {
	switch ci {
	case github.CIPassing:
		return "✓ CI"
	case github.CIFailing:
		return "✗ CI"
	case github.CIPending:
		return "⧗ CI"
	}
	return ""
}

func reviewText(review string) string {
	switch review {
	case "APPROVED":
		return "approved"
	case "CHANGES_REQUESTED":
		return "changes requested"
	case "REVIEW_REQUIRED":
		return "review required"
	}
	return ""
}

func emptyText(showAll bool) string {
	if showAll {
		return "No sessions."
	}
	return "No sessions in the last 7 days. [a] show all"
}

func relative(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func truncate(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	return string(runes[:width-1]) + "…"
}

func pluralize(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}
