package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// timestampFieldSegment identifies which of a timestampField's five
// segments (year, month, day, hour, minute) currently has focus.
type timestampFieldSegment int

const (
	timestampFieldYear timestampFieldSegment = iota
	timestampFieldMonth
	timestampFieldDay
	timestampFieldHour
	timestampFieldMinute
	numTimestampFieldSegments
)

// timestampField is a date/time entry widget with five independently
// adjustable segments (year, month, day, hour, minute), replacing a
// free-text field that would otherwise need parsing and validation
// after the fact (see timestampLayout) — left/right move focus between
// segments, and up/down increment/decrement whichever segment currently
// has focus, wrapping within that segment's own valid range (e.g. month
// wraps from 12 back to 1 without touching the year; day wraps within
// however many days the current month/year actually has, so a value
// this field holds is always a real calendar date, never invalid or in
// need of a validation error at submit time).
type timestampField struct {
	value   time.Time
	segment timestampFieldSegment
	focused bool
}

func newTimestampField(value time.Time) timestampField {
	return timestampField{value: value}
}

func (f *timestampField) Focus() {
	f.focused = true
	f.segment = timestampFieldYear
}

func (f *timestampField) Blur() {
	f.focused = false
}

func (f timestampField) Value() time.Time {
	return f.value
}

// Update handles left/right (move focus between segments) and up/down
// (adjust the focused segment) while focused; any other message,
// including every message while blurred, is left alone.
func (f timestampField) Update(msg tea.Msg) timestampField {
	if !f.focused {
		return f
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return f
	}
	switch key.String() {
	case "left":
		if f.segment > timestampFieldYear {
			f.segment--
		}
	case "right":
		if f.segment < timestampFieldMinute {
			f.segment++
		}
	case "up":
		f.value = f.adjust(1)
	case "down":
		f.value = f.adjust(-1)
	}
	return f
}

// adjust returns value with the focused segment moved by delta,
// wrapping within that segment's own min/max (year has no upper/lower
// bound, so it just moves by delta) — day, month, hour and minute never
// carry into an adjacent segment, so e.g. incrementing day past the end
// of the month wraps back to day 1 of the same month rather than
// rolling into the next one. Changing the year or month can leave the
// current day past the end of the new month (e.g. Jan 31 -> February),
// in which case day is clamped to that month's last day, matching how
// a calendar-style picker commonly handles this.
func (f timestampField) adjust(delta int) time.Time {
	year, month, day := f.value.Date()
	hour, minute, sec := f.value.Clock()

	switch f.segment {
	case timestampFieldYear:
		year += delta
	case timestampFieldMonth:
		month = time.Month(wrapInRange(int(month)+delta, 1, 12))
	case timestampFieldDay:
		day = wrapInRange(day+delta, 1, daysInMonth(year, month))
	case timestampFieldHour:
		hour = wrapInRange(hour+delta, 0, 23)
	case timestampFieldMinute:
		minute = wrapInRange(minute+delta, 0, 59)
	}

	if max := daysInMonth(year, month); day > max {
		day = max
	}
	return time.Date(year, month, day, hour, minute, sec, 0, f.value.Location())
}

// wrapInRange wraps value into [min, max] (inclusive), cycling back to
// the other end rather than clamping — e.g. wrapInRange(13, 1, 12) == 1
// and wrapInRange(0, 1, 12) == 12.
func wrapInRange(value, min, max int) int {
	span := max - min + 1
	return ((value-min)%span+span)%span + min
}

// daysInMonth is how many days month has in year (accounting for leap
// years), found by asking for "day 0" of the following month.
func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// View renders the field as "YYYY-MM-DD HH:MM" (matching timestampLayout
// exactly, including its total width — see createFieldWidth), with the
// focused segment highlighted, matching a text field's own cursor-style
// affordance for showing exactly what will change on the next
// up/down/left/right.
func (f timestampField) View() string {
	segments := [numTimestampFieldSegments]string{
		timestampFieldYear:   fmt.Sprintf("%04d", f.value.Year()),
		timestampFieldMonth:  fmt.Sprintf("%02d", int(f.value.Month())),
		timestampFieldDay:    fmt.Sprintf("%02d", f.value.Day()),
		timestampFieldHour:   fmt.Sprintf("%02d", f.value.Hour()),
		timestampFieldMinute: fmt.Sprintf("%02d", f.value.Minute()),
	}
	separators := [numTimestampFieldSegments]string{
		timestampFieldYear:  "-",
		timestampFieldMonth: "-",
		timestampFieldDay:   " ",
		timestampFieldHour:  ":",
	}

	var b strings.Builder
	for seg := timestampFieldYear; seg < numTimestampFieldSegments; seg++ {
		text := segments[seg]
		if f.focused && seg == f.segment {
			b.WriteString(confirmCursorStyle.Render(text))
		} else {
			b.WriteString(text)
		}
		b.WriteString(separators[seg])
	}
	return b.String()
}
