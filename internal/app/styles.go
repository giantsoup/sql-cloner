package app

import lipgloss "charm.land/lipgloss/v2"

type styles struct {
	frame        lipgloss.Style
	center       lipgloss.Style
	sidebar      lipgloss.Style
	statusLeft   lipgloss.Style
	statusMiddle lipgloss.Style
	statusRight  lipgloss.Style
	helpBar      lipgloss.Style
	title        lipgloss.Style
	subtle       lipgloss.Style
	panel        lipgloss.Style
	panelTitle   lipgloss.Style
	filter       lipgloss.Style
	modal        lipgloss.Style
	statusLine   lipgloss.Style
	error        lipgloss.Style
	success      lipgloss.Style
}

func newStyles() styles {
	cream := lipgloss.Color("#F8F4E3")
	ink := lipgloss.Color("#101820")
	copper := lipgloss.Color("#C86B3C")
	teal := lipgloss.Color("#1F6C68")
	gold := lipgloss.Color("#D0A85C")
	slate := lipgloss.Color("#31404A")
	rose := lipgloss.Color("#8C3B3B")

	return styles{
		frame:        lipgloss.NewStyle().Background(ink).Foreground(cream).Padding(1, 1),
		center:       lipgloss.NewStyle().Width(88).PaddingRight(1),
		sidebar:      lipgloss.NewStyle().Width(34),
		statusLeft:   lipgloss.NewStyle().Background(copper).Foreground(cream).Padding(0, 1).Bold(true),
		statusMiddle: lipgloss.NewStyle().Background(slate).Foreground(cream).Padding(0, 1),
		statusRight:  lipgloss.NewStyle().Background(teal).Foreground(cream).Padding(0, 1).Bold(true),
		helpBar:      lipgloss.NewStyle().Foreground(gold).PaddingTop(1),
		title:        lipgloss.NewStyle().Foreground(gold).Bold(true),
		subtle:       lipgloss.NewStyle().Foreground(lipgloss.Color("#B9C2C9")),
		panel:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(gold).Padding(1, 1).MarginTop(1),
		panelTitle:   lipgloss.NewStyle().Foreground(copper).Bold(true),
		filter:       lipgloss.NewStyle().Foreground(cream).Background(slate).Padding(0, 1).MarginTop(1),
		modal:        lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(copper).Padding(1, 2).MarginTop(2),
		statusLine:   lipgloss.NewStyle().Foreground(teal),
		error:        lipgloss.NewStyle().Foreground(rose).Bold(true),
		success:      lipgloss.NewStyle().Foreground(teal).Bold(true),
	}
}
