package ui

import (
	"strings"
	"testing"

	"github.com/NolanFogarty/courtside/backend"
	"github.com/charmbracelet/x/ansi"
)

func fixtureStandings() standings {
	m := newStandings(100, 30)
	m.loading = false
	m.expanded = true
	m.data.East = []backend.Standing{{Rank: 1, Team: "Celtics"}}
	m.data.West = []backend.Standing{{Rank: 1, Team: "Lakers"}}
	return m
}

func TestExpandedStandingsToggleConferenceWithArrowsAndTab(t *testing.T) {
	m := fixtureStandings()
	if m.showWest {
		t.Fatal("standings must start on the eastern conference")
	}

	m, _ = m.Update(press("right"))
	if !m.showWest {
		t.Fatal("right arrow must switch to the western conference")
	}
	m, _ = m.Update(press("left"))
	if m.showWest {
		t.Fatal("left arrow must switch back to the eastern conference")
	}

	m, _ = m.Update(press("tab"))
	if !m.showWest {
		t.Fatal("tab must toggle to the western conference")
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "WESTERN") {
		t.Fatalf("expanded view must render the western conference, got %q", view)
	}
	m, _ = m.Update(press("shift+tab"))
	if m.showWest {
		t.Fatal("shift+tab must toggle back to the eastern conference")
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "EASTERN") {
		t.Fatalf("expanded view must render the eastern conference, got %q", view)
	}
}
