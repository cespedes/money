package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

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

	// confirmCursorStyle renders a solid block, standing in for a
	// terminal cursor on a "(y/n)" confirmation prompt — which, since
	// it's plain rendered text rather than a focused text field, gets no
	// real cursor of its own — so the prompt reads as something waiting
	// on a keypress rather than a static status line (see
	// confirmDeletePrompt).
	confirmCursorStyle = lipgloss.NewStyle().Reverse(true)
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

// confirmDeletePrompt renders "Delete selected <kind>? (y/n)" followed by
// a cursor-like block (see confirmCursorStyle), shared by every tab's
// delete confirmation.
func confirmDeletePrompt(kind string) string {
	return errorStyle.Render(fmt.Sprintf("Delete selected %s? (y/n)", kind)) + " " + confirmCursorStyle.Render(" ")
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

// stripPickerHeader drops a table.Model's own column header line from its
// rendered View() — used for the blank-titled ({Title: ""}) dropdown
// pickers throughout the TUI (Parent, Currency, other-account), whose
// header exists only so table.Model reserves a row for it internally
// (see e.g. syncParentPickerHeight), not to show any text — so a
// dropdown sits flush under whatever field precedes it instead of
// leaving a blank line above its first row.
func stripPickerHeader(view string) string {
	if _, rest, ok := strings.Cut(view, "\n"); ok {
		return rest
	}
	return view
}

// indentLines prefixes every line of view with n spaces, so a dropdown
// picker lines up under whichever field column it belongs to (see
// fieldPickerOffset) instead of always sitting flush against the pop-up's
// left edge — which is only correct for a picker in the first column.
func indentLines(view string, n int) string {
	if n <= 0 {
		return view
	}
	prefix := strings.Repeat(" ", n)
	lines := strings.Split(view, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
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
