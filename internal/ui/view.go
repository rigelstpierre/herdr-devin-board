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
	defaultWidth    = 100
	gap             = "  "
	statusWidth     = 11
	maxRepoWidth    = 18
	maxReviewWidth  = 17
	updatedWidth    = 7
	minSessionWidth = 10
	detailHeight    = 5
	maxDetailPRs    = detailHeight - 2
	chromeHeight    = 5 + 1 + detailHeight + 2
	none            = "–"
	repoColumn      = 2
	reviewColumn    = 5
)

var (
	styleTitle    = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	styleColumn   = lipgloss.NewStyle().Faint(true).Bold(true)
	styleDim      = lipgloss.NewStyle().Faint(true)
	styleLink     = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Underline(true)
	styleSelected = lipgloss.NewStyle().Reverse(true)
	styleStatus   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	styleGreen    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleYellow   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleRed      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	stylePlain    = lipgloss.NewStyle()
	kindStyles    = map[board.Kind]lipgloss.Style{
		board.Running:   styleGreen,
		board.Waiting:   styleYellow.Bold(true),
		board.Suspended: styleDim,
		board.Finished:  styleDim,
		board.Errored:   styleRed,
	}
	keyHints = [][2]string{
		{"Enter", "open"}, {"↑↓", "select"}, {"p", "PR"}, {"1-9", "nth PR"},
		{"s", "ssh"}, {"x", "archive"}, {"a", "all"}, {"r", "refresh"}, {"q", "quit"},
	}
)

type cell struct {
	text       string
	style      lipgloss.Style
	width      int
	alignRight bool
}

type layout struct {
	session int
	repo    int
	pr      int
	review  int
}

func (l layout) visible(cells []cell) []cell {
	var shown []cell
	for i, c := range cells {
		if (i == repoColumn && l.repo == 0) || (i == reviewColumn && l.review == 0) {
			continue
		}
		shown = append(shown, c)
	}
	return shown
}

func (m Model) View() string {
	width := m.width
	if width == 0 {
		width = defaultWidth
	}
	sessions := m.sessions()
	cols := columnLayout(sessions, width)
	rule := styleDim.Render(strings.Repeat("─", width))

	lines := []string{styleTitle.Render("Devin Sessions"), truncate(summary(sessions), width), ""}
	lines = append(lines, truncate(headerRow(cols), width), rule)
	for _, row := range m.tableRows(sessions, cols) {
		lines = append(lines, truncate(row, width))
	}
	lines = append(lines, rule)
	lines = append(lines, m.detail(width)...)

	footer := []string{truncate(keyHintLine(), width), truncate(m.statusLine(), width)}
	if m.height > 0 {
		for len(lines)+len(footer) < m.height {
			lines = append(lines, "")
		}
	} else {
		lines = append(lines, "")
	}
	return strings.Join(append(lines, footer...), "\n")
}

func columnLayout(sessions []board.Session, width int) layout {
	cols := layout{repo: len("REPO"), pr: len("PR"), review: len("REVIEW")}
	for _, s := range sessions {
		repo, pr, _, review := prCells(s)
		cols.repo = max(cols.repo, ansi.StringWidth(repo.text))
		cols.pr = max(cols.pr, ansi.StringWidth(pr.text))
		cols.review = max(cols.review, ansi.StringWidth(review.text))
	}
	cols.repo = min(cols.repo, maxRepoWidth)
	cols.review = min(cols.review, maxReviewWidth)
	if width-cols.fixedWidth() < minSessionWidth {
		cols.repo = 0
	}
	if width-cols.fixedWidth() < minSessionWidth {
		cols.review = 0
	}
	cols.session = max(width-cols.fixedWidth(), minSessionWidth)
	return cols
}

func (l layout) fixedWidth() int {
	widths := []int{statusWidth, l.pr, 2, updatedWidth}
	if l.repo > 0 {
		widths = append(widths, l.repo)
	}
	if l.review > 0 {
		widths = append(widths, l.review)
	}
	total := len(widths) * len(gap)
	for _, w := range widths {
		total += w
	}
	return total
}

func headerRow(cols layout) string {
	return renderCells(cols.visible([]cell{
		{text: "STATUS", style: styleColumn, width: statusWidth},
		{text: "SESSION", style: styleColumn, width: cols.session},
		{text: "REPO", style: styleColumn, width: cols.repo},
		{text: "PR", style: styleColumn, width: cols.pr},
		{text: "CI", style: styleColumn, width: 2},
		{text: "REVIEW", style: styleColumn, width: cols.review},
		{text: "UPDATED", style: styleColumn, width: updatedWidth, alignRight: true},
	}), false)
}

func (m Model) tableRows(sessions []board.Session, cols layout) []string {
	switch {
	case !m.loaded && m.err == nil:
		return []string{styleDim.Render("Loading sessions…")}
	case m.loaded && len(sessions) == 0:
		return []string{styleDim.Render(emptyText(m.showAll))}
	}
	end := min(len(sessions), m.offset+m.visibleRows())
	var rows []string
	for i := m.offset; i < end; i++ {
		rows = append(rows, m.sessionRow(sessions[i], cols, i == m.cursor))
	}
	return rows
}

func (m Model) sessionRow(s board.Session, cols layout, selected bool) string {
	repo, pr, ci, review := prCells(s)
	repo.width, pr.width, ci.width, review.width = cols.repo, cols.pr, 2, cols.review
	return renderCells(cols.visible([]cell{
		{text: kindLabel(s.Kind), style: kindStyles[s.Kind], width: statusWidth},
		{text: s.Title, style: stylePlain, width: cols.session},
		repo, pr, ci, review,
		{text: shortAge(m.deps.Now(), s.UpdatedAt), style: styleDim, width: updatedWidth, alignRight: true},
	}), selected)
}

func renderCells(cells []cell, selected bool) string {
	parts := make([]string, len(cells))
	for i, c := range cells {
		text := pad(truncate(c.text, c.width), c.width, c.alignRight)
		if selected {
			parts[i] = text
		} else {
			parts[i] = c.style.Render(text)
		}
	}
	row := strings.Join(parts, gap)
	if selected {
		return styleSelected.Render(row)
	}
	return row
}

func prCells(s board.Session) (repo, number, ci, review cell) {
	if len(s.PRs) == 0 {
		dash := cell{text: none, style: styleDim}
		return dash, dash, dash, dash
	}
	pr := s.PRs[0]
	label := fmt.Sprintf("#%d", pr.Number)
	if extra := len(s.PRs) - 1; extra > 0 {
		label += fmt.Sprintf(" +%d", extra)
	}
	return cell{text: pr.Repo, style: stylePlain},
		cell{text: label, style: stylePlain},
		ciCell(pr),
		reviewCell(pr)
}

func ciCell(pr board.PR) cell {
	if pr.Unknown {
		return cell{text: "?", style: styleDim}
	}
	if pr.State != "open" && pr.State != "draft" {
		return cell{text: none, style: styleDim}
	}
	switch pr.CI {
	case github.CIPassing:
		return cell{text: "✓", style: styleGreen}
	case github.CIFailing:
		return cell{text: "✕", style: styleRed}
	case github.CIPending:
		return cell{text: "●", style: styleYellow}
	}
	return cell{text: none, style: styleDim}
}

func reviewCell(pr board.PR) cell {
	switch {
	case pr.Unknown:
		return cell{text: "?", style: styleDim}
	case pr.State == "merged" || pr.State == "closed":
		return cell{text: pr.State, style: styleDim}
	}
	switch pr.Review {
	case "APPROVED":
		return cell{text: "approved", style: styleGreen}
	case "CHANGES_REQUESTED":
		return cell{text: "changes requested", style: styleRed}
	case "REVIEW_REQUIRED":
		return cell{text: "review required", style: stylePlain}
	}
	if pr.State == "draft" {
		return cell{text: "draft", style: styleDim}
	}
	return cell{text: none, style: styleDim}
}

func (m Model) detail(width int) []string {
	var lines []string
	if session, ok := m.selected(); ok {
		lines = append(lines, styleLink.Render(session.URL))
		lines = append(lines, truncate(statusSummary(session, m.deps.Now()), width))
		for i, pr := range session.PRs {
			if i == maxDetailPRs-1 && len(session.PRs) > maxDetailPRs {
				lines = append(lines, styleDim.Render(fmt.Sprintf("   … %d more", len(session.PRs)-i)))
				break
			}
			lines = append(lines, prDetail(i+1, pr))
		}
	}
	if m.height > 0 {
		for len(lines) < detailHeight {
			lines = append(lines, "")
		}
	}
	return lines
}

func statusSummary(s board.Session, now time.Time) string {
	parts := []string{kindStyles[s.Kind].Render(kindLabel(s.Kind))}
	if s.Detail != "" {
		parts = append(parts, strings.ReplaceAll(s.Detail, "_", " "))
	}
	parts = append(parts, "updated "+relative(now, s.UpdatedAt))
	return strings.Join(parts, styleDim.Render(" · "))
}

func prDetail(n int, pr board.PR) string {
	parts := []string{fmt.Sprintf("%s #%d", pr.Repo, pr.Number), pr.State}
	if ci := ciWords(pr); ci != "" {
		parts = append(parts, ci)
	}
	if review := reviewCell(pr); review.text != none && review.text != pr.State && review.text != "?" {
		parts = append(parts, review.style.Render(review.text))
	}
	return styleDim.Render(fmt.Sprintf("%d  ", n)) + strings.Join(parts, styleDim.Render(" · ")) + "  " + styleLink.Render(pr.URL)
}

func ciWords(pr board.PR) string {
	if pr.Unknown {
		return styleDim.Render("CI unknown")
	}
	if pr.State != "open" && pr.State != "draft" {
		return ""
	}
	switch pr.CI {
	case github.CIPassing:
		return styleGreen.Render("CI passing")
	case github.CIFailing:
		return styleRed.Render("CI failing")
	case github.CIPending:
		return styleYellow.Render("CI running")
	}
	return styleDim.Render("no checks")
}

func summary(sessions []board.Session) string {
	counts := map[board.Kind]int{}
	for _, s := range sessions {
		counts[s.Kind]++
	}
	parts := []string{styleDim.Render(fmt.Sprintf("%d sessions", len(sessions)))}
	for _, k := range []board.Kind{board.Waiting, board.Running, board.Errored, board.Suspended, board.Finished} {
		if counts[k] == 0 {
			continue
		}
		style := styleDim
		if k == board.Waiting {
			style = kindStyles[k]
		}
		parts = append(parts, style.Render(fmt.Sprintf("%d %s", counts[k], kindWord(k))))
	}
	return strings.Join(parts, styleDim.Render(" · "))
}

func keyHintLine() string {
	parts := make([]string, len(keyHints))
	for i, h := range keyHints {
		parts[i] = styleDim.Render(h[0]) + " " + h[1]
	}
	return strings.Join(parts, styleDim.Render(" · "))
}

func (m Model) statusLine() string {
	if m.confirming != nil {
		return styleYellow.Bold(true).Render(fmt.Sprintf("Archive %q? y to confirm · any other key cancels", m.confirming.Title))
	}
	if m.err != nil {
		return styleError.Render(m.err.Error())
	}
	var parts []string
	if m.loaded {
		parts = append(parts, "refreshed "+relative(m.deps.Now(), m.refreshedAt))
	}
	if m.loading {
		parts = append(parts, "refreshing…")
	}
	if m.showAll {
		parts = append(parts, "showing all")
	} else {
		parts = append(parts, "last 7 days")
	}
	return styleStatus.Render(strings.Join(parts, " · "))
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

func kindWord(k board.Kind) string {
	return strings.Fields(kindLabel(k))[1]
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

func truncate(s string, width int) string {
	return ansi.Truncate(s, width, "…")
}

func pad(s string, width int, alignRight bool) string {
	fill := strings.Repeat(" ", max(width-ansi.StringWidth(s), 0))
	if alignRight {
		return fill + s
	}
	return s + fill
}
