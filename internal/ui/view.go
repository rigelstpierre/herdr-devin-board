package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/rigelstpierre/herdr-devin-board/internal/board"
	"github.com/rigelstpierre/herdr-devin-board/internal/github"
)

const (
	defaultWidth  = 80
	cursorWidth   = 2
	statusWidth   = 11
	ageWidth      = 3
	gap           = "  "
	minTitleWidth = 10
	prNumberWidth = 7
	prStateWidth  = 6
	footer        = "enter open · p PR · s ssh · a all · r refresh · q quit"
)

var titleColumn = cursorWidth + statusWidth + len(gap) + ageWidth + len(gap)

var (
	styleHeader = lipgloss.NewStyle().Bold(true)
	styleError  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	styleDim    = lipgloss.NewStyle().Faint(true)
	styleGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleYellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleRed    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	kindStyles  = map[board.Kind]lipgloss.Style{
		board.Running:   styleGreen,
		board.Waiting:   styleYellow.Bold(true),
		board.Suspended: styleDim,
		board.Finished:  styleDim,
		board.Errored:   styleRed,
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
	repoWidth := widestRepo(sessions)
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
			b.WriteString(prefix + prLine(session.PRs[l.pr], repoWidth) + "\n")
		}
	}
	b.WriteString("\n" + styleDim.Render(footer) + "\n")
	return b.String()
}

func (m Model) header(count int) string {
	parts := []string{"Devin Sessions", fmt.Sprint(count)}
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
	titleWidth := max(width-titleColumn, minTitleWidth)
	status := kindStyles[s.Kind].Render(fmt.Sprintf("%-*s", statusWidth, kindLabel(s.Kind)))
	age := styleDim.Render(fmt.Sprintf("%*s", ageWidth, shortAge(m.deps.Now(), s.UpdatedAt)))
	return status + gap + age + gap + ansi.Truncate(s.Title, titleWidth, "…")
}

func prLine(pr board.PR, repoWidth int) string {
	columns := []string{
		fmt.Sprintf("%-*s", prNumberWidth, fmt.Sprintf("#%d", pr.Number)),
		fmt.Sprintf("%-*s", repoWidth, pr.Repo),
		stateStyle(pr.State).Render(fmt.Sprintf("%-*s", prStateWidth, pr.State)),
	}
	switch {
	case pr.Unknown:
		columns = append(columns, styleDim.Render("? ?"))
	case pr.State == "open" || pr.State == "draft":
		columns = append(columns, ciText(pr.CI), reviewText(pr.Review))
	}
	indent := strings.Repeat(" ", titleColumn-cursorWidth)
	return strings.TrimRight(indent+strings.Join(columns, gap), " ")
}

func widestRepo(sessions []board.Session) int {
	widest := 0
	for _, s := range sessions {
		for _, pr := range s.PRs {
			widest = max(widest, ansi.StringWidth(pr.Repo))
		}
	}
	return widest
}

func stateStyle(state string) lipgloss.Style {
	switch state {
	case "merged", "closed":
		return styleDim
	}
	return lipgloss.NewStyle()
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
		return styleGreen.Render("✓") + " CI"
	case github.CIFailing:
		return styleRed.Render("✗") + " CI"
	case github.CIPending:
		return styleYellow.Render("•") + " CI"
	}
	return "    "
}

func reviewText(review string) string {
	switch review {
	case "APPROVED":
		return styleGreen.Render("approved")
	case "CHANGES_REQUESTED":
		return styleRed.Render("changes requested")
	case "REVIEW_REQUIRED":
		return styleDim.Render("review required")
	}
	return ""
}

func emptyText(showAll bool) string {
	if showAll {
		return "No sessions."
	}
	return "No sessions in the last 7 days · press a to show all"
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

func shortAge(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
