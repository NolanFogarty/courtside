package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/NolanFogarty/courtside/backend"
	"github.com/charmbracelet/x/ansi"
)

func TestConferencePanelsHaveMatchingBottomBorders(t *testing.T) {
	for _, counts := range [][2]int{{3, 1}, {1, 3}, {3, 0}, {0, 3}, {0, 0}, {8, 7}} {
		var games []backend.Game
		for i := 0; i < counts[0]; i++ {
			games = append(games, listGame("east", "Bulls", "Celtics", "Final"))
		}
		for i := 0; i < counts[1]; i++ {
			games = append(games, listGame("west", "Suns", "Lakers", "Final"))
		}
		m := fixtureList(games, 120, 40)
		w := (contentWidth(m.width) - 2) / 2
		wantHeight := max(lipgloss.Height(m.conferencePanel(colEast, w)), lipgloss.Height(m.conferencePanel(colWest, w)))
		rendered := ansi.Strip(m.columnsView())
		lines := strings.Split(rendered, "\n")
		if len(lines) != wantHeight {
			t.Errorf("counts %v: height=%d, want %d", counts, len(lines), wantHeight)
		}
		// Both lower-left border corners must share the last row, rather than
		// padding being added outside an already closed shorter panel.
		if strings.Count(lines[len(lines)-1], "╰") != 2 {
			t.Errorf("counts %v: bottom borders not aligned: %q", counts, lines[len(lines)-1])
		}
	}
}
