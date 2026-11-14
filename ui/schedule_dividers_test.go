package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/NolanFogarty/courtside/backend"
	"github.com/charmbracelet/x/ansi"
)

func TestScheduleDividersAndSelection(t *testing.T) {
	m := schedule{width: 120, height: 35, games: []backend.SeasonGame{
		{GameID: "1", Date: "2025-01-01", Away: "Bulls", Home: "Knicks"},
		{GameID: "2", Date: "2025-01-01", Away: "Heat", Home: "Celtics"},
		{GameID: "3", Date: "2025-01-02", Away: "Magic", Home: "Pacers"},
	}, selected: 2}
	view := ansi.Strip(m.View().Content)
	rules := 0
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, strings.Repeat("─", 30)) && strings.Contains(line, "│") {
			rules++
		}
		if strings.Contains(line, "▶") && !strings.Contains(line, "Magic") {
			t.Fatalf("wrong row selected: %s", line)
		}
	}
	// One header rule and one day divider; no rule between same-day games.
	if rules != 2 {
		t.Fatalf("got %d horizontal rules, want 2:\n%s", rules, view)
	}
	if !strings.Contains(view, "▶") {
		t.Fatal("selected game has no marker")
	}
}

func TestScheduleScrollKeepsSelectedGameVisibleWithDividers(t *testing.T) {
	var games []backend.SeasonGame
	for i := 0; i < 24; i++ {
		games = append(games, backend.SeasonGame{GameID: fmt.Sprint(i), Date: fmt.Sprintf("2025-01-%02d", i+1), Away: fmt.Sprintf("Away%d", i), Home: "Home"})
	}
	for _, h := range []int{14, 22, 35} {
		m := schedule{width: 100, height: h, games: games}
		for _, key := range []string{"down", "down", "pgdown", "down", "pgdown", "up", "pgup", "up"} {
			m, _ = m.Update(tea.KeyPressMsg{Code: map[string]rune{"down": tea.KeyDown, "up": tea.KeyUp, "pgdown": tea.KeyPgDown, "pgup": tea.KeyPgUp}[key]})
			rows := m.filteredGames()
			end := m.pageEnd(rows, m.offset)
			if m.selected < m.offset || m.selected >= end {
				t.Fatalf("height %d key %s: selected %d outside [%d,%d)", h, key, m.selected, m.offset, end)
			}
			cost := end - m.offset
			for i := m.offset + 1; i < end; i++ {
				if scheduleLocalDate(rows[i]) != scheduleLocalDate(rows[i-1]) {
					cost++
				}
			}
			if cost > m.pageSize() {
				t.Fatalf("height %d: used %d rows, budget %d", h, cost, m.pageSize())
			}
			selected, ok := m.SelectedGame()
			if !ok || selected.GameId != rows[m.selected].GameID {
				t.Fatal("divider changed game selection")
			}
		}
	}
}
