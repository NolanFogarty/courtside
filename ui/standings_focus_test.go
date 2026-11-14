package ui

import (
	"strings"
	"testing"

	"github.com/NolanFogarty/courtside/backend"
)

func TestStandingsConferenceFocusUsesFocusedPanelStyle(t *testing.T) {
	rows := []backend.Standing{{Rank: 1, Team: "Celtics"}}
	focused := renderConferenceFocused("EASTERN", awayColor, rows, false, true)
	unfocused := renderConferenceFocused("WESTERN", homeColor, rows, false, false)

	if focused == unfocused {
		t.Fatal("focused and unfocused conference panels should use different border styles")
	}
	if !strings.Contains(focused, "Celtics") || !strings.Contains(unfocused, "WESTERN") {
		t.Fatalf("conference content missing from rendered panels")
	}
	if want := panelSty.Render("panel"); renderConference("EASTERN", awayColor, rows, false) == want {
		t.Fatal("renderConference should retain its normal conference content")
	}
}

func TestStandingsViewFocusesSelectedConference(t *testing.T) {
	m := fixtureStandings()
	m.expanded = false

	if got, want := m.showWest, false; got != want {
		t.Fatalf("initial conference = %v, want east", got)
	}
	m, _ = m.Update(press("tab"))
	if !m.showWest {
		t.Fatal("tab should select west")
	}
	view := m.View().Content
	if !strings.Contains(view, "WESTERN") || strings.Contains(view, "EASTERN") {
		t.Fatalf("narrow standings view should show the selected western conference, got %q", view)
	}
}
