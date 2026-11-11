package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestDatePickerSelectionTracksDisplayedDate(t *testing.T) {
	day := time.Date(2000, time.January, 10, 0, 0, 0, 0, time.Local)
	m := gamelist{day: day, pickDay: day, entering: true, width: 80, height: 24}

	assertSelection := func(want time.Time) {
		t.Helper()
		view := m.datePickerView().Content
		plain := ansi.Strip(view)
		if !strings.Contains(plain, want.Format("Mon Jan 2, 2006")) {
			t.Fatalf("picker subtitle does not show selected date %s:\n%s", want.Format("Mon Jan 2, 2006"), plain)
		}
		// Selected cells carry an underline escape; unlike the old style, this
		// indicator follows the cursor rather than remaining on today's date.
		if !strings.Contains(view, accentSty.Underline(true).Render(fmt.Sprintf(" %2d ", want.Day()))) {
			t.Fatalf("calendar has no underlined selection for %s", want.Format("2006-01-02"))
		}
	}

	assertSelection(day)
	updated, _ := m.updateDatePicker(tea.KeyPressMsg{Code: tea.KeyRight})
	m = updated.(gamelist)
	if !sameDay(m.pickDay, day.AddDate(0, 0, 1)) {
		t.Fatalf("right navigation selected %s, want %s", m.pickDay, day.AddDate(0, 0, 1))
	}
	assertSelection(m.pickDay)
}

func TestDatePickerOnlyUnderlinesSelectedDate(t *testing.T) {
	today := localToday()
	pick := today.AddDate(0, 0, -1)
	m := gamelist{day: today, pickDay: pick}
	calendar := strings.Join(m.calendarLines(), "\n")
	selected := accentSty.Underline(true).Render(fmt.Sprintf(" %2d ", pick.Day()))
	if !strings.Contains(calendar, selected) {
		t.Fatal("selected day missing underline")
	}
	// After removing the selected cell, no other calendar cell is underlined.
	rest := strings.Replace(calendar, selected, "", 1)
	if strings.Contains(rest, "\x1b[4m") || strings.Contains(rest, ";4m") {
		t.Fatal("a non-selected date is also underlined")
	}
}
