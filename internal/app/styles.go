package app

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

type styles struct {
	frame        lipgloss.Style
	center       lipgloss.Style
	sidebar      lipgloss.Style
	statusLeft   lipgloss.Style
	statusMiddle lipgloss.Style
	statusRight  lipgloss.Style
	helpBar      lipgloss.Style
	title        lipgloss.Style
	titleAccent  lipgloss.Style
	subtle       lipgloss.Style
	panel        lipgloss.Style
	panelActive  lipgloss.Style
	panelTitle   lipgloss.Style
	label        lipgloss.Style
	value        lipgloss.Style
	badge        lipgloss.Style
	badgeStrong  lipgloss.Style
	badgeOK      lipgloss.Style
	badgeWarn    lipgloss.Style
	badgeError   lipgloss.Style
	filter       lipgloss.Style
	modal        lipgloss.Style
	statusLine   lipgloss.Style
	error        lipgloss.Style
	success      lipgloss.Style
	code         lipgloss.Style
}

func newStyles() styles {
	text := lipgloss.Color("#E5ECF4")
	muted := lipgloss.Color("#94A3B8")
	bg := lipgloss.Color("#09111C")
	surface := lipgloss.Color("#0F1A2B")
	surfaceAlt := lipgloss.Color("#132235")
	border := lipgloss.Color("#2A3A50")
	accent := lipgloss.Color("#D97706")
	accentSoft := lipgloss.Color("#F59E0B")
	success := lipgloss.Color("#2FA58A")
	danger := lipgloss.Color("#EF6F6C")

	return styles{
		frame:        lipgloss.NewStyle().Background(bg).Foreground(text).Padding(0, 1, 1, 1),
		center:       lipgloss.NewStyle().MarginRight(1),
		sidebar:      lipgloss.NewStyle(),
		statusLeft:   lipgloss.NewStyle().Background(accent).Foreground(bg).Padding(0, 1).Bold(true),
		statusMiddle: lipgloss.NewStyle().Background(surfaceAlt).Foreground(text).Padding(0, 1),
		statusRight:  lipgloss.NewStyle().Background(success).Foreground(bg).Padding(0, 1).Bold(true),
		helpBar:      lipgloss.NewStyle().Foreground(muted).BorderTop(true).BorderForeground(border).PaddingTop(0),
		title:        lipgloss.NewStyle().Foreground(text).Bold(true),
		titleAccent:  lipgloss.NewStyle().Foreground(accentSoft).Bold(true),
		subtle:       lipgloss.NewStyle().Foreground(muted),
		panel:        lipgloss.NewStyle().Background(surface).Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1),
		panelActive:  lipgloss.NewStyle().Background(surface).Border(lipgloss.RoundedBorder()).BorderForeground(accentSoft).Padding(0, 1),
		panelTitle:   lipgloss.NewStyle().Foreground(accentSoft).Bold(true),
		label: lipgloss.NewStyle().
			Foreground(muted).
			Bold(true).
			Transform(func(value string) string { return strings.ToUpper(value) }),
		value:       lipgloss.NewStyle().Foreground(text),
		badge:       lipgloss.NewStyle().Foreground(text).Background(surfaceAlt).Padding(0, 1),
		badgeStrong: lipgloss.NewStyle().Foreground(bg).Background(accentSoft).Padding(0, 1).Bold(true),
		badgeOK:     lipgloss.NewStyle().Foreground(bg).Background(success).Padding(0, 1).Bold(true),
		badgeWarn:   lipgloss.NewStyle().Foreground(bg).Background(accent).Padding(0, 1).Bold(true),
		badgeError:  lipgloss.NewStyle().Foreground(text).Background(danger).Padding(0, 1).Bold(true),
		filter:      lipgloss.NewStyle().Foreground(text).Background(surfaceAlt).Padding(0, 1),
		modal:       lipgloss.NewStyle().Background(surface).Border(lipgloss.DoubleBorder()).BorderForeground(accent).Padding(1, 2).MarginTop(2),
		statusLine:  lipgloss.NewStyle().Foreground(success).Bold(true),
		error:       lipgloss.NewStyle().Foreground(danger).Bold(true),
		success:     lipgloss.NewStyle().Foreground(success).Bold(true),
		code:        lipgloss.NewStyle().Foreground(text).Background(surfaceAlt).Padding(0, 1),
	}
}
