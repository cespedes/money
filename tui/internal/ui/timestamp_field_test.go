package ui

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestTimestampField_LeftRightMovesSegmentWithinBounds(t *testing.T) {
	f := newTimestampField(time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC))
	f.Focus()

	if f.segment != timestampFieldYear {
		t.Fatalf("segment = %v, want timestampFieldYear on focus", f.segment)
	}

	// left at the leftmost segment stays put.
	f = f.Update(keyPress("left"))
	if f.segment != timestampFieldYear {
		t.Fatalf("segment = %v, want timestampFieldYear (left at the start is a no-op)", f.segment)
	}

	order := []timestampFieldSegment{
		timestampFieldMonth, timestampFieldDay, timestampFieldHour, timestampFieldMinute,
	}
	for _, want := range order {
		f = f.Update(keyPress("right"))
		if f.segment != want {
			t.Fatalf("segment = %v, want %v", f.segment, want)
		}
	}
	// right at the rightmost segment stays put.
	f = f.Update(keyPress("right"))
	if f.segment != timestampFieldMinute {
		t.Fatalf("segment = %v, want timestampFieldMinute (right at the end is a no-op)", f.segment)
	}

	for _, want := range []timestampFieldSegment{
		timestampFieldHour, timestampFieldDay, timestampFieldMonth, timestampFieldYear,
	} {
		f = f.Update(keyPress("left"))
		if f.segment != want {
			t.Fatalf("segment = %v, want %v", f.segment, want)
		}
	}
}

func TestTimestampField_UpDownIgnoredWhenBlurred(t *testing.T) {
	original := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	f := newTimestampField(original)

	f = f.Update(keyPress("up"))
	if !f.Value().Equal(original) {
		t.Fatalf("Value() = %v, want unchanged %v (field isn't focused)", f.Value(), original)
	}
}

func TestTimestampField_MonthWrapsWithoutChangingYear(t *testing.T) {
	f := newTimestampField(time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC))
	f.Focus()
	f = f.Update(keyPress("right")) // -> month

	f = f.Update(keyPress("up"))
	got := f.Value()
	if got.Year() != 2026 || got.Month() != time.January {
		t.Fatalf("after wrapping past December: got %v, want January 2026 (year untouched)", got)
	}

	f = f.Update(keyPress("down"))
	got = f.Value()
	if got.Year() != 2026 || got.Month() != time.December {
		t.Fatalf("after wrapping back below January: got %v, want December 2026 (year untouched)", got)
	}
}

func TestTimestampField_DayWrapsWithinMonthWithoutRollingOver(t *testing.T) {
	// January has 31 days: incrementing past it should wrap to day 1 of
	// the same month, not roll into February.
	f := newTimestampField(time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC))
	f.Focus()
	f = f.Update(keyPress("right")) // -> month
	f = f.Update(keyPress("right")) // -> day

	f = f.Update(keyPress("up"))
	got := f.Value()
	if got.Month() != time.January || got.Day() != 1 {
		t.Fatalf("after wrapping past day 31: got %v, want January 1 (same month)", got)
	}

	f = f.Update(keyPress("down"))
	got = f.Value()
	if got.Month() != time.January || got.Day() != 31 {
		t.Fatalf("after wrapping back below day 1: got %v, want January 31 (last day of the same month)", got)
	}
}

func TestTimestampField_DayClampsWhenMonthShrinks(t *testing.T) {
	// January 31 -> February must clamp the day rather than overflow
	// into March, the way time.Time.AddDate would.
	f := newTimestampField(time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC))
	f.Focus()
	f = f.Update(keyPress("right")) // -> month
	f = f.Update(keyPress("up"))

	got := f.Value()
	if got.Month() != time.February || got.Day() != 28 {
		t.Fatalf("got %v, want February 28 (clamped from 31, 2026 isn't a leap year)", got)
	}
}

func TestTimestampField_YearClampsFeb29InNonLeapYear(t *testing.T) {
	f := newTimestampField(time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)) // 2024 is a leap year
	f.Focus()
	f = f.Update(keyPress("up")) // year -> 2025, not a leap year

	got := f.Value()
	if got.Year() != 2025 || got.Month() != time.February || got.Day() != 28 {
		t.Fatalf("got %v, want February 28, 2025 (clamped from 29)", got)
	}
}

func TestTimestampField_HourWraps(t *testing.T) {
	f := newTimestampField(time.Date(2026, 6, 15, 23, 30, 0, 0, time.UTC))
	f.Focus()
	f = f.Update(keyPress("right")) // month
	f = f.Update(keyPress("right")) // day
	f = f.Update(keyPress("right")) // hour

	f = f.Update(keyPress("up"))
	if got := f.Value(); got.Hour() != 0 || got.Day() != 15 {
		t.Fatalf("after wrapping past hour 23: got %v, want hour 0, same day (no roll-over)", got)
	}

	f = f.Update(keyPress("down"))
	if got := f.Value(); got.Hour() != 23 {
		t.Fatalf("after wrapping back below hour 0: got %v, want hour 23", got)
	}
}

func TestTimestampField_MinuteWraps(t *testing.T) {
	f := newTimestampField(time.Date(2026, 6, 15, 23, 0, 0, 0, time.UTC))
	f.Focus()
	f = f.Update(keyPress("right")) // month
	f = f.Update(keyPress("right")) // day
	f = f.Update(keyPress("right")) // hour
	f = f.Update(keyPress("right")) // minute

	f = f.Update(keyPress("down"))
	if got := f.Value(); got.Minute() != 59 || got.Hour() != 23 {
		t.Fatalf("after wrapping below minute 0: got %v, want minute 59, same hour (no roll-over)", got)
	}

	f = f.Update(keyPress("up"))
	if got := f.Value(); got.Minute() != 0 || got.Hour() != 23 {
		t.Fatalf("after wrapping past minute 59: got %v, want minute 0, same hour", got)
	}
}

func TestTimestampField_ViewShowsFixedLayout(t *testing.T) {
	f := newTimestampField(time.Date(2026, 1, 5, 9, 3, 0, 0, time.UTC))
	if got, want := len([]rune(ansi.Strip(f.View()))), len(timestampLayout); got != want {
		t.Fatalf("View() length = %d, want %d (matching timestampLayout)", got, want)
	}
}
