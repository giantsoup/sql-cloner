package app

import lipgloss "charm.land/lipgloss/v2"

type styles struct {
	frame         lipgloss.Style
	center        lipgloss.Style
	sidebar       lipgloss.Style
	statusCard    lipgloss.Style
	statusLabel   lipgloss.Style
	statusPath    lipgloss.Style
	hero          lipgloss.Style
	heroEyebrow   lipgloss.Style
	heroTitle     lipgloss.Style
	heroSubtitle  lipgloss.Style
	helpBar       lipgloss.Style
	helpLabel     lipgloss.Style
	title         lipgloss.Style
	titleAccent   lipgloss.Style
	subtle        lipgloss.Style
	panel         lipgloss.Style
	panelActive   lipgloss.Style
	panelTitle    lipgloss.Style
	panelSubtitle lipgloss.Style
	label         lipgloss.Style
	value         lipgloss.Style
	badge         lipgloss.Style
	badgeStrong   lipgloss.Style
	badgeOK       lipgloss.Style
	badgeWarn     lipgloss.Style
	badgeError    lipgloss.Style
	badgeGhost    lipgloss.Style
	filter        lipgloss.Style
	modal         lipgloss.Style
	statusLine    lipgloss.Style
	error         lipgloss.Style
	success       lipgloss.Style
	code          lipgloss.Style
}

func newStyles() styles {
	text := lipgloss.Color("#E8F1F8")
	muted := lipgloss.Color("#93A9BD")
	bg := lipgloss.Color("#07131F")
	surface := lipgloss.Color("#0E1D2D")
	surfaceAlt := lipgloss.Color("#13273B")
	surfaceSoft := lipgloss.Color("#19344B")
	border := lipgloss.Color("#27455F")
	borderStrong := lipgloss.Color("#4C6F8E")
	accent := lipgloss.Color("#D88A1D")
	accentSoft := lipgloss.Color("#F7B955")
	success := lipgloss.Color("#53C39D")
	info := lipgloss.Color("#67C7D8")
	danger := lipgloss.Color("#F27F75")

	return styles{
		frame:         lipgloss.NewStyle().Background(bg).Foreground(text).Padding(1, 1),
		center:        lipgloss.NewStyle().MarginRight(1),
		sidebar:       lipgloss.NewStyle(),
		statusCard:    lipgloss.NewStyle().Background(surface).Border(lipgloss.RoundedBorder()).BorderForeground(borderStrong).Padding(1, 2),
		statusLabel:   lipgloss.NewStyle().Foreground(info).Bold(true),
		statusPath:    lipgloss.NewStyle().Foreground(text),
		hero:          lipgloss.NewStyle().Background(surfaceAlt).Border(lipgloss.RoundedBorder()).BorderForeground(borderStrong).Padding(1, 2),
		heroEyebrow:   lipgloss.NewStyle().Foreground(info).Bold(true),
		heroTitle:     lipgloss.NewStyle().Foreground(text).Bold(true),
		heroSubtitle:  lipgloss.NewStyle().Foreground(muted),
		helpBar:       lipgloss.NewStyle().Foreground(muted).Background(surface).Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1),
		helpLabel:     lipgloss.NewStyle().Foreground(accentSoft).Bold(true),
		title:         lipgloss.NewStyle().Foreground(text).Bold(true),
		titleAccent:   lipgloss.NewStyle().Foreground(accentSoft).Bold(true),
		subtle:        lipgloss.NewStyle().Foreground(muted),
		panel:         lipgloss.NewStyle().Background(surface).Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(1, 2),
		panelActive:   lipgloss.NewStyle().Background(surface).Border(lipgloss.RoundedBorder()).BorderForeground(accentSoft).Padding(1, 2),
		panelTitle:    lipgloss.NewStyle().Foreground(text).Bold(true),
		panelSubtitle: lipgloss.NewStyle().Foreground(muted),
		label: lipgloss.NewStyle().
			Foreground(info).
			Bold(true),
		value: lipgloss.NewStyle().Foreground(text),
		badge: lipgloss.NewStyle().Foreground(text).Background(surfaceSoft).Padding(0, 1),
		badgeStrong: lipgloss.NewStyle().
			Foreground(bg).
			Background(accentSoft).
			Padding(0, 1).
			Bold(true),
		badgeOK: lipgloss.NewStyle().
			Foreground(bg).
			Background(success).
			Padding(0, 1).
			Bold(true),
		badgeWarn: lipgloss.NewStyle().
			Foreground(bg).
			Background(accent).
			Padding(0, 1).
			Bold(true),
		badgeError: lipgloss.NewStyle().
			Foreground(text).
			Background(danger).
			Padding(0, 1).
			Bold(true),
		badgeGhost: lipgloss.NewStyle().
			Foreground(muted).
			Background(surface).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			Padding(0, 1),
		filter:     lipgloss.NewStyle().Foreground(text).Background(surfaceAlt).Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1),
		modal:      lipgloss.NewStyle().Background(surfaceAlt).Border(lipgloss.RoundedBorder()).BorderForeground(accentSoft).Padding(1, 2).MarginTop(1),
		statusLine: lipgloss.NewStyle().Foreground(success).Bold(true),
		error:      lipgloss.NewStyle().Foreground(danger).Bold(true),
		success:    lipgloss.NewStyle().Foreground(success).Bold(true),
		code:       lipgloss.NewStyle().Foreground(text).Background(surfaceSoft).Padding(0, 1),
	}
}
