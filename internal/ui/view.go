package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	colSize  = 9
	colBar   = 12
	colPct   = 11
	colRate  = 11
	colPeers = 5
	colState = 11
)

func (m Model) View() tea.View {
	sections := []string{m.header(), m.table()}
	if t, ok := m.selected(); ok && m.showDetails {
		sections = append(sections, m.details(t))
	}
	sections = append(sections, m.footer())

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, sections...))
	v.AltScreen = true
	v.WindowTitle = "peerz"
	return v
}

func (m Model) header() string {
	var down, up int64
	for _, t := range m.torrents {
		down += t.DownRate
		up += t.UpRate
	}
	stats := fmt.Sprintf("  %d torrents  ↓ %s  ↑ %s",
		len(m.torrents), humanRate(down), humanRate(up))

	return titleStyle.Render("peerz") + dimStyle.Render(stats) + "\n"
}

func (m Model) nameWidth() int {
	w := m.width
	if w == 0 {
		w = 100
	}
	// cursor(2) + fixed columns + 7 spaces + 1 space
	fixed := 2 + colSize + colBar + colPct + 2*colRate + colPeers + colState + 7 + 1
	return max(10, w-fixed)
}

func (m Model) table() string {
	nw := m.nameWidth()
	var b strings.Builder

	head := fmt.Sprintf("  %s %*s %-*s %*s %*s %*s %*s %-*s",
		fit("NAME", nw), colSize, "SIZE", colBar, "PROGRESS", colPct, "",
		colRate, "DOWN", colRate, "UP", colPeers, "PEERS", colState, "STATE")
	b.WriteString(headerStyle.Render(head) + "\n")

	if len(m.torrents) == 0 {
		b.WriteString(dimStyle.Render("  no torrents") + "\n")
	}

	for i, t := range m.torrents {
		cursor, name := "  ", fit(t.Name, nw)
		if i == m.cursor {
			cursor = selectedStyle.Render("> ")
			name = selectedStyle.Render(name)
		}
		fmt.Fprintf(&b, "%s%s %*s %s %*s %*s %*s %*d %s\n",
			cursor, name,
			colSize, humanBytes(t.Size),
			bar(t.Progress, colBar),
			colPct, fmt.Sprintf("%.1f%%", t.Progress*100),
			colRate, humanRate(t.DownRate),
			colRate, humanRate(t.UpRate),
			colPeers, t.Peers,
			stateStyle(t.State).Render(fit(t.State, colState)))
	}
	return b.String()
}

func (m Model) details(t TorrentView) string {
	eta := "∞"
	if t.DownRate > 0 && t.Progress < 1 {
		left := float64(t.Size) * (1 - t.Progress)
		secs := left / float64(t.DownRate)
		eta = time.Duration(secs * float64(time.Second)).Round(time.Second).String()
	}
	done := int64(float64(t.Size) * t.Progress)

	lines := []string{
		selectedStyle.Render(t.Name),
		"",
		fmt.Sprintf("Info hash  %s", t.ID),
		fmt.Sprintf("Size       %s in %d pieces", humanBytes(t.Size), t.Pieces),
		fmt.Sprintf("Done       %s of %s", humanBytes(done), humanBytes(t.Size)),
		fmt.Sprintf("Peers      %d", t.Peers),
		fmt.Sprintf("ETA        %s", eta),
	}
	return detailBox.Render(strings.Join(lines, "\n"))
}

func (m Model) footer() string {
	return dimStyle.Render("\n  j/k move • enter details • space pause • q quit")
}

func fit(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s + strings.Repeat(" ", w-len(r))
	}
	return string(r[:w-1]) + "…"
}

func bar(p float64, w int) string {
	p = min(max(p, 0), 1)
	filled := int(p * float64(w))
	return barFull.Render(strings.Repeat("█", filled)) +
		barEmpty.Render(strings.Repeat("░", w-filled))
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func humanRate(n int64) string {
	if n == 0 {
		return "-"
	}
	return humanBytes(n) + "/s"
}
