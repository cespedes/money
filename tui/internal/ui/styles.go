package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 2)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Padding(0, 2)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	formLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	focusedFieldStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212"))

	popupStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)
)

// padOrTruncate left-aligns s within width (by rune count), for a pop-up
// form field's plain-text value where — unlike a textinput.Model — there's
// no built-in fixed-width rendering.
func padOrTruncate(s string, width int) string {
	r := []rune(s)
	if len(r) > width {
		return string(r[:width])
	}
	return s + strings.Repeat(" ", width-len(r))
}

// columnHeader styles label as a fixed-width, table-style column header
// for a pop-up form (see createFieldWidth), highlighted if it's the
// currently focused field.
func columnHeader(label string, focused bool) string {
	style := formLabelStyle
	if focused {
		style = focusedFieldStyle
	}
	return style.Width(createFieldWidth).Render(label)
}

// overlayCentered composites popup on top of background, centered within
// a width x height viewport (e.g. the terminal size), so it reads as a
// pop-up floating over whatever's currently on screen. If width or height
// isn't known yet, background is returned unchanged.
func overlayCentered(background, popup string, width, height int) string {
	if width <= 0 || height <= 0 {
		return background
	}
	x := max(0, (width-lipgloss.Width(popup))/2)
	y := max(0, (height-lipgloss.Height(popup))/2)

	bg := lipgloss.NewLayer(background)
	bg.AddLayers(lipgloss.NewLayer(popup).X(x).Y(y).Z(1))
	return lipgloss.NewCompositor(bg).Render()
}
