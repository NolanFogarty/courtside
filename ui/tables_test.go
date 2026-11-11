package ui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/NolanFogarty/courtside/backend"
	"github.com/charmbracelet/x/ansi"
)

func TestColumnOrdering(t *testing.T) {
	cols := []dataColumn{textColumn("NAME", "PLAYER", 20), numberColumn("PTS", "PTS", 5, 1), dateColumn("DATE", "DATE", 14)}
	rows := []backend.Row{
		dataRow(map[string]any{"NAME": "Second", "PTS": 9.5, "DATE": "JAN 31, 2026"}),
		dataRow(map[string]any{"NAME": "First", "PTS": 100.0, "DATE": "FEB 01, 2026"}),
		dataRow(map[string]any{"NAME": "Unknown", "PTS": nil, "DATE": nil}),
		dataRow(map[string]any{"NAME": "Tied", "PTS": 9.5, "DATE": "DEC 01, 2025"}),
	}
	for _, tc := range []struct {
		key  string
		desc bool
		want string
	}{
		{"PTS", true, "First,Second,Tied,Unknown"}, {"PTS", false, "Second,Tied,First,Unknown"},
		{"NAME", false, "First,Second,Tied,Unknown"}, {"DATE", false, "Tied,Second,First,Unknown"}, {"DATE", true, "First,Second,Tied,Unknown"},
	} {
		got := []string{}
		for _, r := range orderedRows(rows, cols, columnSort{key: tc.key, desc: tc.desc}) {
			got = append(got, r.Text("NAME"))
		}
		if strings.Join(got, ",") != tc.want {
			t.Errorf("%s desc=%v: %v", tc.key, tc.desc, got)
		}
	}
	if rows[0].Text("NAME") != "Second" {
		t.Fatal("sorting mutated source data")
	}
}

func TestRosterSortControlsKeepPlayerSelection(t *testing.T) {
	r := newRoot(nil)
	r.current = teamview
	r.teams = fixtureTeam()
	r.teams.width, r.teams.height = 80, 24
	for i := range r.teams.data.Roster {
		r.teams.data.Roster[i].Stats = dataRow(map[string]any{"PTS": i, "GP": i + 1})
	}
	original, _ := r.teams.SelectedPlayer()
	r = updateRoot(r, press("s"))
	if !r.teams.sortMode {
		t.Fatal("s should focus the column headers")
	}
	for r.teams.columns()[r.teams.order().cursor].key != "PTS" {
		r = updateRoot(r, press("right"))
	}
	r = updateRoot(r, press("down"))
	if r.teams.order().key != "PTS" || !r.teams.order().desc {
		t.Fatal("down should sort selected column descending")
	}
	if r.teams.players()[0].Stats.Number("PTS") != 29 {
		t.Fatal("points were not sorted numerically")
	}
	selected, _ := r.teams.SelectedPlayer()
	if selected.ID != original.ID {
		t.Fatal("sorting changed the selected player")
	}
	r = updateRoot(r, press("enter"))
	if r.current != teamview || !r.teams.sortMode || r.teams.order().desc {
		t.Fatal("enter should toggle order without opening a player")
	}
	r = updateRoot(r, press("down"))
	r = updateRoot(r, press("esc"))
	if r.current != teamview || r.teams.sortMode {
		t.Fatal("escape should close sort controls and stay on the roster")
	}
	if r.teams.selected < r.teams.offset || r.teams.selected >= r.teams.offset+r.teams.pageSize() {
		t.Fatal("selected player scrolled out of view after leaving sorting")
	}
	r = updateRoot(r, press("enter"))
	if r.current != playerview || r.player.seed.ID != original.ID {
		t.Fatal("sorted row should open the same player's profile")
	}
}

func fixturePlayer() player {
	p := newPlayer(backend.RosterPlayer{ID: 1, Name: "Luka Dončić", Height: "6-8", Weight: "230", School: "Slovenia"}, backend.Teams[13], 100, 32, 1)
	p.loading = false
	p.tab = 1
	p.data = backend.PlayerProfile{Season: "2026-27", Errors: map[string]string{}}
	for i := range 35 {
		row := dataRow(map[string]any{"SEASON_ID": fmt.Sprintf("%d-%02d", 1992+i, (1993+i)%100), "TEAM_ABBREVIATION": "LAL", "GP": 35 - i, "MIN": 30.5, "PTS": i, "REB": 8.2, "AST": 7.5, "STL": 1.5, "BLK": 0.5, "TOV": 2.3, "FGM": 8.5, "FGA": 17.0, "FG_PCT": 0.5, "FG3M": 3, "FG3A": 6, "FG3_PCT": 0.5, "FTM": 4, "FTA": 5, "FT_PCT": 0.8, "GAME_DATE": fmt.Sprintf("NOV %02d, 2026", i%28+1), "MATCHUP": "LAL vs. BOS", "WL": "W"})
		p.data.Seasons = append(p.data.Seasons, row)
		p.data.Playoffs = append(p.data.Playoffs, row)
		p.data.Games = append(p.data.Games, row)
	}
	p.data.Info = dataRow(map[string]any{"DISPLAY_FIRST_LAST": "Luka Dončić", "COUNTRY": "Slovenia", "TEAM_NAME": "Lakers", "HEIGHT": "6-8", "WEIGHT": "230"})
	p.data.Career = p.data.Seasons[:1]
	return p
}
func TestPlayerSortPersistsAcrossSectionsAndRefresh(t *testing.T) {
	r := newRoot(nil)
	r.current = playerview
	r.player = fixturePlayer()
	r = updateRoot(r, press("s"))
	for r.player.columns()[r.player.order().cursor].key != "PTS" {
		r = updateRoot(r, press("right"))
	}
	r = updateRoot(r, press("down"))
	if r.player.rows()[0].Number("PTS") != 34 {
		t.Fatal("career rows were not sorted by points")
	}
	r = updateRoot(r, press("esc"))
	if r.current != playerview || r.player.sortMode {
		t.Fatal("escape should keep player page open")
	}
	r = updateRoot(r, press("3"))
	if r.player.order().key != "SEASON_ID" {
		t.Fatal("each section needs its own order")
	}
	r = updateRoot(r, press("2"))
	if r.player.order().key != "PTS" || !r.player.order().desc {
		t.Fatal("career sort was lost on section switch")
	}
	data := r.player.data
	r = updateRoot(r, playerLoadedMsg{id: 1, request: 1, data: data})
	if r.player.order().key != "PTS" {
		t.Fatal("refresh lost the selected sort")
	}
	r = updateRoot(r, press("o"))
	if r.player.rows()[0].Number("PTS") != 34 {
		t.Fatal("switching shooting stats should preserve row order")
	}
}

func TestPlayerTablesPinHeadersAndRevealEveryColumn(t *testing.T) {
	for _, size := range [][2]int{{60, 24}, {80, 24}, {100, 32}, {130, 40}} {
		for _, tab := range []int{1, 2, 3} {
			for _, shooting := range []bool{false, true} {
				p := fixturePlayer()
				p.width, p.height = size[0], size[1]
				p.tab = tab
				p.shooting = shooting
				p, _ = p.Update(press("end"))
				view := ansi.Strip(p.View().Content)
				if !strings.Contains(view, "╭") || !strings.Contains(view, "╰") || !strings.Contains(view, "─") {
					t.Fatal("table borders disappeared while paging")
				}
				header := "SEASON"
				if tab == 3 {
					header = "DATE"
				}
				if !strings.Contains(view, header) {
					t.Fatal("column headers must stay visible while scrolling")
				}
				assertViewFits(t, view, p.width, p.height)
				p, _ = p.Update(press("s"))
				for i := range p.columns() {
					state := p.order()
					state.cursor = i
					p.orders[p.tab] = state
					p.columnOffsets[p.tab] = revealColumn(p.columns(), contentWidth(p.width)-4, p.columnOffsets[p.tab], i)
					if !strings.Contains(ansi.Strip(p.View().Content), p.columns()[i].label) {
						t.Fatalf("cannot reach column %s at width %d", p.columns()[i].key, p.width)
					}
				}
			}
		}
	}
}
func assertViewFits(t *testing.T, view string, width, height int) {
	t.Helper()
	if lipgloss.Height(view) > height {
		t.Fatalf("view height %d exceeds %d", lipgloss.Height(view), height)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > width {
			t.Fatalf("line exceeds width %d: %q", width, line)
		}
	}
}

func TestSelectedTableRowHighlightsUniformly(t *testing.T) {
	cols := []dataColumn{textColumn("TEAM", "TEAM", 16), numberColumn("W", "W", 3, 0)}
	// A plain selected row gets the white selection style.
	plain := renderDataTable("T", cols, []backend.Row{
		dataRow(map[string]any{"TEAM": "Lakers", "W": 1}),
		dataRow(map[string]any{"TEAM": "Celtics", "W": 2}),
	}, dataTableOptions{width: 60, selected: 0})
	var selected string
	for _, line := range strings.Split(plain, "\n") {
		if strings.Contains(line, "▶") {
			selected = line
		}
	}
	if selected == "" {
		t.Fatal("selected row must carry the gutter marker")
	}
	if !strings.Contains(selected, "\x1b[") {
		t.Fatalf("plain selected row must be highlighted, got %q", selected)
	}
	// A caller-prestyled selected row (schedule winner/loser colors) stands
	// on its own: wrapping it would bury its colors under an outer reset.
	styled := renderDataTable("T", cols, []backend.Row{
		dataRow(map[string]any{"TEAM": lipgloss.NewStyle().Background(lipgloss.Color("22")).Render("Lakers"), "W": 1}),
	}, dataTableOptions{width: 60, selected: 0})
	selected = ""
	for _, line := range strings.Split(styled, "\n") {
		if strings.Contains(line, "▶") {
			selected = line
		}
	}
	if !strings.Contains(selected, "48;5;22") {
		t.Fatalf("prestyled selected row must keep its colors, got %q", selected)
	}
}

func TestTableUnicodeAndWrappedReports(t *testing.T) {
	fields := [][2]string{{"Luka Dončić", "An update with enough text to wrap across several lines, with no loss of data or broken alignment."}, {"八村 塁", "東京 · Healthy"}}
	lines := infoTable("REPORT NOTES", fields, 76)
	view := ansi.Strip(strings.Join(lines, "\n"))
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) != 76 {
			t.Fatalf("misaligned table border: %q", line)
		}
	}
	if !strings.Contains(view, "八村 塁") || !strings.Contains(view, "alignment.") {
		t.Fatal("wrapped table lost report content")
	}
}

func TestScheduleShowsTBDWithoutTipTime(t *testing.T) {
	s := newSchedule(100, 24)
	s.loading = false
	s.games = []backend.SeasonGame{
		{GameID: "1", Away: "Lakers", Home: "Celtics", GameStatus: 1},
	}
	view := ansi.Strip(s.View().Content)
	if !strings.Contains(view, "TBD") {
		t.Fatalf("schedule with no tip time must show TBD, got %q", view)
	}
	if strings.Contains(view, "0 - 0") {
		t.Fatalf("schedule must never show 0-0 for unplayed games, got %q", view)
	}
}

func TestScheduleLoadLandsOnLastGameWhenSeasonOver(t *testing.T) {
	s := newSchedule(100, 24)
	s, _ = s.Update(scheduleLoaded{games: []backend.SeasonGame{
		{GameID: "1", Date: "2020-01-01", Away: "Bulls", Home: "Knicks", Status: "Final", GameStatus: 3, AwayScore: 100, HomeScore: 90},
		{GameID: "2", Date: "2020-01-02", Away: "Heat", Home: "Bucks", Status: "Final", GameStatus: 3, AwayScore: 101, HomeScore: 99},
	}})
	if s.selected != 1 {
		t.Fatalf("all-past schedule must land on the last game, got selected=%d", s.selected)
	}
	if game, ok := s.SelectedGame(); !ok || game.GameId != "2" {
		t.Fatalf("all-past schedule selected %+v", game)
	}
}

func TestLateScheduleLoadKeepsUserPosition(t *testing.T) {
	games := []backend.SeasonGame{
		{GameID: "1", Date: "2020-01-01", Away: "Bulls", Home: "Knicks", Status: "Final", GameStatus: 3, AwayScore: 100, HomeScore: 90},
		{GameID: "2", Date: "2999-01-01", Away: "Heat", Home: "Bucks", GameStatus: 1},
	}
	// Fresh view with no navigation lands on the first upcoming game.
	fresh := newSchedule(100, 24)
	fresh, _ = fresh.Update(scheduleLoaded{games: games})
	if fresh.selected != 1 {
		t.Fatalf("fresh load must land on today/future, got selected=%d", fresh.selected)
	}
	// A user who navigated while the fetch was in flight must not be yanked.
	moved := newSchedule(100, 24)
	moved, _ = moved.Update(press("up"))
	moved, _ = moved.Update(scheduleLoaded{games: games})
	if moved.selected != 0 || moved.offset != 0 {
		t.Fatalf("late load must keep user position, got selected=%d offset=%d", moved.selected, moved.offset)
	}
}

func TestScheduleSelectedRowUsesTextColors(t *testing.T) {
	s := newSchedule(100, 24)
	s.loading = false
	s.games = []backend.SeasonGame{
		{GameID: "1", Date: "2026-11-02", Away: "Lakers", Home: "Celtics", Status: "Final", GameStatus: 3, AwayScore: 110, HomeScore: 105},
		{GameID: "2", Date: "2026-11-02", Away: "Bulls", Home: "Knicks", Status: "Final", GameStatus: 3, AwayScore: 90, HomeScore: 100},
	}
	s.selected, s.offset = 0, 0
	var selected, plain string
	for _, line := range strings.Split(s.View().Content, "\n") {
		switch {
		case strings.Contains(line, "▶"):
			selected = line
		case strings.Contains(line, "Bulls"):
			plain = line
		}
	}
	if selected == "" || plain == "" {
		t.Fatal("schedule must render selected and unselected rows")
	}
	for _, code := range []string{"48;5;22", "48;5;88"} {
		if strings.Contains(selected, code) {
			t.Fatalf("schedule selected row must use text colors, got %q", selected)
		}
	}
	if !strings.Contains(selected, "1;92mLakers") {
		t.Fatalf("schedule selected row must color the winner directly, got %q", selected)
	}
	if strings.Contains(plain, "48;5;") || !strings.Contains(plain, "1;92mKnicks") {
		t.Fatalf("schedule unselected row must use foreground colors without backgrounds, got %q", plain)
	}
}

func TestBrightWhiteAccent(t *testing.T) {
	if accentColor != lipgloss.Color("15") {
		t.Fatal("focus accents must be bright white")
	}
}

func TestScheduleTableKeepsSelectionAndScoresVisible(t *testing.T) {
	for _, width := range []int{60, 80, 120} {
		s := newSchedule(width, 24)
		s.loading = false
		for i := range 30 {
			s.games = append(s.games, backend.SeasonGame{GameID: fmt.Sprint(i + 1), Date: "2026-11-02", Away: "Lakers", Home: "Celtics", Status: "Final", AwayScore: 110 + i, HomeScore: 105, AwayWins: 4, AwayLosses: 2, HomeWins: 3, HomeLosses: 3})
		}
		s, _ = s.Update(press("pgdown"))
		selected, ok := s.SelectedGame()
		if !ok || selected.GameId != "16" {
			t.Fatal("table layout changed the selected game")
		}
		view := ansi.Strip(s.View().Content)
		if !strings.Contains(view, "AWAY") || !strings.Contains(view, "╰") {
			t.Fatal("schedule must retain table headers and borders")
		}
		assertViewFits(t, view, width, 24)
		if width == 60 {
			s, _ = s.Update(press("]"))
		}
		view = ansi.Strip(s.View().Content)
		if !strings.Contains(view, "SCORE") {
			t.Fatal("schedule score column is unreachable")
		}
	}
}
