package ui

import "charm.land/lipgloss/v2"

var (
	accent = lipgloss.Color("#7D56F4")
	subtle = lipgloss.Color("#6C6C6C")
	green  = lipgloss.Color("#04B575")
	red    = lipgloss.Color("#E06C75")
	yellow = lipgloss.Color("#E5C07B")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(accent).
			Padding(0, 1)

	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(subtle)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	dimStyle      = lipgloss.NewStyle().Foreground(subtle)

	barFull  = lipgloss.NewStyle().Foreground(green)
	barEmpty = lipgloss.NewStyle().Foreground(subtle)

	detailBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Padding(0, 1)
)

func stateStyle(state string) lipgloss.Style {
	switch state {
	case "downloading":
		return lipgloss.NewStyle().Foreground(green)
	case "seeding":
		return lipgloss.NewStyle().Foreground(accent)
	case "checking":
		return lipgloss.NewStyle().Foreground(yellow)
	case "error":
		return lipgloss.NewStyle().Foreground(red)
	default:
		return dimStyle
	}
}
