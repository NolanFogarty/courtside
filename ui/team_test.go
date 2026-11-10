package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/NolanFogarty/courtside/backend"
	"github.com/charmbracelet/x/ansi"
)

func press(s string) tea.KeyPressMsg {
	code := rune(0)
	text := ""
	var mod tea.KeyMod
	switch s {
	case "enter":
		code = tea.KeyEnter
	case "esc":
		code = tea.KeyEscape
	case "down":
		code = tea.KeyDown
	case "up":
		code = tea.KeyUp
	case "end":
		code = tea.KeyEnd
	case "home":
		code = tea.KeyHome
	case "left":
		code = tea.KeyLeft
	case "right":
		code = tea.KeyRight
	case "pgdown":
		code = tea.KeyPgDown
	case "pgup":
		code = tea.KeyPgUp
	case "tab":
		code = tea.KeyTab
	case "ctrl+left":
		code, mod = tea.KeyLeft, tea.ModCtrl
	case "ctrl+right":
		code, mod = tea.KeyRight, tea.ModCtrl
	case "ctrl+up":
		code, mod = tea.KeyUp, tea.ModCtrl
	case "ctrl+down":
		code, mod = tea.KeyDown, tea.ModCtrl
	default:
		code = []rune(s)[0]
		text = s
	}
	return tea.KeyPressMsg{Code: code, Text: text, Mod: mod}
}
func updateRoot(r root, msg tea.Msg) root { model, _ := r.Update(msg); return model.(root) }
func profileStatRow(fields map[string]any) backend.Row {
	r := backend.Row{}
	for k, v := range fields {
		b, _ := json.Marshal(v)
		r[k] = b
	}
	return r
}
func fixtureTeam() teams {
	m := newTeams(100, 32)
	m.browsing = false
	m.data = backend.TeamProfile{Team: backend.Teams[13], Season: "2026-27", Updated: time.Now(), Errors: map[string]string{}}
	for i := range 30 {
		salary := int64((i + 1) * 1000000)
		m.data.Roster = append(m.data.Roster, backend.RosterPlayer{ID: i + 1, Name: fmt.Sprintf("Player %02d", i+1), Salary: &salary, Stats: profileStatRow(map[string]any{"GP": 5, "PTS": 20})})
	}
	return m
}
func TestQuitFromTopLevelList(t *testing.T) {
	r := newRoot(nil)
	r.current = listview
	r.list = fixtureList(fixtureDay(), 100, 30)
	r.list.loading = false
	_, cmd := r.Update(press("q"))
	if cmd == nil {
		t.Fatal("q on the scores list must quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q on the scores list must produce a quit command")
	}
	// While filtering, q is text input, not quit.
	r.list.filtering = true
	updated, cmd := r.list.Update(press("q"))
	r.list = updated.(gamelist)
	if r.list.filter != "q" {
		t.Fatal("q while filtering must type, not quit")
	}
	if cmd != nil {
		t.Fatal("typing q in the filter must not quit")
	}
}

func TestNavigationWrapsAndCenters(t *testing.T) {
	m := newGamesList(nil)
	m.width = 80
	hints := ansi.Strip(m.hints())
	lines := strings.Split(hints, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 help rows at 80 columns, got %d: %q", len(lines), hints)
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		left := lipgloss.Width(line) - lipgloss.Width(strings.TrimLeft(line, " "))
		right := lipgloss.Width(line) - lipgloss.Width(strings.TrimRight(line, " "))
		if lipgloss.Width(line) > 76 || left-right > 1 || right-left > 1 || trimmed == "" {
			t.Fatalf("help is not centered: %q", line)
		}
	}
	for _, width := range []int{20, 40, 76, 116} {
		h := renderHints(width, [2]string{"t", "teams"}, [2]string{"a", "full schedule"}, [2]string{"s", "standings"})
		for _, line := range strings.Split(h, "\n") {
			if lipgloss.Width(line) > width {
				t.Fatalf("help overflows %d: %q", width, line)
			}
		}
		for _, word := range []string{"teams", "standings"} {
			if !strings.Contains(ansi.Strip(h), word) {
				t.Fatalf("missing shortcut: %q", h)
			}
		}
	}
}
func TestTeamToPlayerAndBack(t *testing.T) {
	r := newRoot(nil)
	r = updateRoot(r, tea.WindowSizeMsg{Width: 100, Height: 32})
	r = updateRoot(r, press("t"))
	if r.current != teamview || !r.teams.browsing {
		t.Fatal("t must open teams")
	}
	r.teams = fixtureTeam()
	r = updateRoot(r, press("enter"))
	if r.current != playerview || r.player.seed.ID != 1 {
		t.Fatal("roster enter must open the selected player")
	}
	r = updateRoot(r, press("esc"))
	if r.current != teamview || r.teams.browsing {
		t.Fatal("back must preserve the team profile")
	}
	r = updateRoot(r, press("esc"))
	if !r.teams.browsing {
		t.Fatal("second back must return to team browser")
	}
	r = updateRoot(r, press("esc"))
	if r.current != listview {
		t.Fatal("browser back must return to scores")
	}
	// Typed letters belong to filters and date inputs.
	r.list.entering = true
	r = updateRoot(r, press("t"))
	if r.current != listview {
		t.Fatal("shortcut stole a text input key")
	}
	r.list.entering = false
	r = updateRoot(r, press("t"))
	r.teams.filtering = true
	r = updateRoot(r, press("t"))
	if r.teams.filter != "t" {
		t.Fatal("team search must accept the shortcut letter")
	}
}
func TestProfileResponsesStayWithTheirOwner(t *testing.T) {
	r := newRoot(nil)
	r.teams = fixtureTeam()
	r.teams.loading = true
	r.teams.request = 2
	stale := teamLoadedMsg{id: r.teams.data.Team.ID, request: 1, data: backend.TeamProfile{Season: "stale"}}
	r = updateRoot(r, stale)
	if !r.teams.loading || r.teams.data.Season == "stale" {
		t.Fatal("stale team response applied")
	}
	data := r.teams.data
	data.Season = "loaded"
	r = updateRoot(r, teamLoadedMsg{id: data.Team.ID, request: 2, data: data})
	if r.teams.loading || r.teams.data.Season != "loaded" || r.current != listview {
		t.Fatal("background team load was lost or changed active view")
	}
	r.player = newPlayer(data.Roster[0], data.Team, 100, 32, 7)
	r = updateRoot(r, playerLoadedMsg{id: 1, request: 6, data: backend.PlayerProfile{ID: 1}})
	if !r.player.loading {
		t.Fatal("stale player response applied")
	}
	r = updateRoot(r, playerLoadedMsg{id: 1, request: 7, data: backend.PlayerProfile{ID: 1}})
	if r.player.loading {
		t.Fatal("player response lost while another page was active")
	}
}
func TestRosterSelectionAndResize(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {100, 32}, {130, 40}} {
		for _, tab := range []int{0, 3} {
			m := fixtureTeam()
			m.width, m.height = size[0], size[1]
			m.tab = tab
			m, _ = m.Update(press("end"))
			p, ok := m.SelectedPlayer()
			if !ok {
				t.Fatal("last player must be selectable")
			}
			view := ansi.Strip(m.View().Content)
			if !strings.Contains(view, p.Name) {
				t.Fatalf("selected player was clipped at %v tab %d: %s", size, tab, view)
			}
			if !strings.Contains(view, "esc/q back") {
				t.Fatalf("footer clipped: %s", view)
			}
			if lipgloss.Height(view) > m.height {
				t.Fatalf("view height %d exceeds %d", lipgloss.Height(view), m.height)
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > m.width {
					t.Fatalf("view overflows width %d", m.width)
				}
			}
		}
	}
}
func TestEmptyAndFailedTeamSections(t *testing.T) {
	m := newTeams(80, 24)
	m.browsing = false
	m.data = backend.TeamProfile{Season: "2026-27", Errors: map[string]string{"injuries": "timeout", "salaries": "timeout"}}
	for tab := range len(teamTabs) {
		m.tab = tab
		view := ansi.Strip(m.View().Content)
		if tab == 2 && (!strings.Contains(view, "timeout") || strings.Contains(view, "No injuries")) {
			t.Fatal("failed injury request was shown as healthy roster")
		}
	}
	m.data.Errors = map[string]string{}
	m.tab = 3
	if strings.Contains(strings.Join(m.rosterLines(), "\n"), "total: $0") {
		t.Fatal("unknown payroll must not appear as zero")
	}
}

func TestTeamScheduleNavigationAndEmptyFilter(t *testing.T) {
	r := newRoot(nil)
	r.width, r.height = 100, 32
	r.current = teamview
	r.teams = fixtureTeam()
	r = updateRoot(r, press("a"))
	if r.current != scheduleview || r.schedule.filter != "Lakers" {
		t.Fatal("team schedule must be filtered to the selected team")
	}
	r = updateRoot(r, scheduleLoaded{games: []backend.SeasonGame{{Home: "Lakers", Away: "Celtics", Date: "2026-11-01"}, {Home: "Other", Away: "Other", Date: "2026-11-03"}, {Home: "Lakers", Away: "Bulls", Date: "2026-11-04"}}})
	if len(r.schedule.filteredGames()) != 2 {
		t.Fatal("wrong team schedule")
	}
	r = updateRoot(r, press("q"))
	if r.current != teamview || r.teams.browsing {
		t.Fatal("schedule must return to team profile")
	}
	s := newSchedule(80, 24)
	s.games = []backend.SeasonGame{{Home: "Lakers", Away: "Bulls"}}
	s.filter = "no matches"
	s, _ = s.Update(press("down"))
	if s.offset != 0 || s.selected != 0 {
		t.Fatal("empty schedule must not get a negative index")
	}
	_ = s.View()
}

func TestFootersStayVisibleInExistingViews(t *testing.T) {
	for _, width := range []int{80, 120} {
		r := newRoot([]backend.Game{{GameId: "001", HomeTeam: "Lakers", AwayTeam: "Bulls", GameClock: "Final"}})
		r = updateRoot(r, tea.WindowSizeMsg{Width: width, Height: 32})
		r.standings = newStandings(width, 32)
		r.standings.loading = false
		for range 15 {
			r.standings.data.East = append(r.standings.data.East, backend.Standing{Team: "East Team"})
			r.standings.data.West = append(r.standings.data.West, backend.Standing{Team: "West Team"})
		}
		r.schedule = newSchedule(width, 32)
		r.detail = newDetail(backend.Game{GameId: "001", HomeTeam: "Lakers", AwayTeam: "Bulls"}, width, 32)
		r.detail.loading = false
		for i := range 15 {
			p := playerStat{name: fmt.Sprintf("Player %d", i), min: "20", pts: i}
			r.detail.game.away.players = append(r.detail.game.away.players, p)
			r.detail.game.home.players = append(r.detail.game.home.players, p)
		}
		for _, view := range []int{listview, standingsview, scheduleview, detailview} {
			r.current = view
			rendered := ansi.Strip(r.View().Content)
			if !strings.Contains(rendered, "t teams") {
				t.Fatalf("missing teams shortcut in view %d", view)
			}
			if lipgloss.Height(rendered) > 32 {
				t.Fatalf("footer exceeds terminal height: view %d height %d", view, lipgloss.Height(rendered))
			}
			for _, line := range strings.Split(rendered, "\n") {
				if lipgloss.Width(line) > width {
					t.Fatalf("view %d overflows terminal width %d", view, width)
				}
			}
		}
	}
}

func TestGameResponsesAfterTeamNavigation(t *testing.T) {
	r := newRoot(nil)
	r.current = teamview
	// A live game so the new detail is still waiting on its box score.
	liveGame := backend.GameFromSeasonSchedule(backend.SeasonGame{GameID: "new-game", Away: "Bulls", Home: "Knicks", GameStatus: 2, Status: "Q2 05:00"})
	r.detail = newDetail(liveGame, 100, 32)
	r = updateRoot(r, gameDetailMsg{gameID: "old-game", detail: backend.GameDetail{}})
	if !r.detail.loading {
		t.Fatal("response for a previous game replaced the new game")
	}
	r = updateRoot(r, gameDetailMsg{gameID: "new-game", detail: backend.GameDetail{}})
	if r.detail.loading || r.current != teamview {
		t.Fatal("background game response was lost or changed active view")
	}
}

func TestFormatTeamUpdatedUsesLocalZone(t *testing.T) {
	ts := time.Date(2026, 11, 3, 0, 30, 0, 0, time.UTC)
	abbrev, _ := time.Now().Zone()
	if got := formatTeamUpdated(ts); !strings.HasSuffix(got, abbrev) {
		t.Fatalf("team updated stamp must carry local zone %q, got %q", abbrev, got)
	}
}

func TestLocalTodayMatchesSystemDate(t *testing.T) {
	now := time.Now().Local()
	today := localToday()
	if now.Year() != today.Year() || now.YearDay() != today.YearDay() {
		t.Fatalf("localToday %v does not match system date %v", today, now)
	}
}
