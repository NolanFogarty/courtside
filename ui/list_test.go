package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/NolanFogarty/courtside/backend"
	"github.com/charmbracelet/x/ansi"
)

// listGame builds a display-only game; status defaults to scheduled so the
// clock renders as-is without needing live box-score data.
func listGame(id, away, home, clock string) backend.Game {
	return backend.Game{GameId: id, AwayTeam: away, HomeTeam: home, GameClock: clock}
}

// fixtureDay mixes intra- and cross-conference matchups. Columns sort by the
// home team: Boston hosting LA is an EASTERN game, LA hosting Boston WESTERN.
func fixtureDay() []backend.Game {
	return []backend.Game{
		listGame("1", "Lakers", "Celtics", "Final"),
		listGame("2", "Bulls", "Knicks", "Final"),
		listGame("3", "Celtics", "Lakers", "Q2 05:00"),
		listGame("4", "Suns", "Warriors", "7:30 PM ET"),
		listGame("5", "Heat", "Bucks", "Final"),
		listGame("6", "Nuggets", "Thunder", "8:00 PM ET"),
	}
}

// fullSlate builds the busiest realistic day: 15 games, 8 hosted in the east.
func fullSlate() []backend.Game {
	eastHosts := []string{"Celtics", "Knicks", "Bucks", "Cavaliers", "Heat", "Pacers", "Magic", "76ers"}
	westHosts := []string{"Lakers", "Warriors", "Suns", "Nuggets", "Thunder", "Timberwolves", "Spurs"}
	aways := []string{"Bulls", "Raptors", "Wizards", "Hawks", "Nets", "Hornets", "Pistons", "Mavericks", "Rockets", "Clippers", "Grizzlies", "Pelicans", "Kings", "Jazz", "Trail Blazers"}
	games := []backend.Game{}
	for i, home := range eastHosts {
		games = append(games, listGame(fmt.Sprintf("e%d", i), aways[i], home, "Final"))
	}
	for i, home := range westHosts {
		games = append(games, listGame(fmt.Sprintf("w%d", i), aways[i+len(eastHosts)], home, "Final"))
	}
	return games
}

func fixtureList(games []backend.Game, w, h int) gamelist {
	m := gamelist{games: games, width: w, height: h}
	m.clamp()
	return m
}

func TestConferenceColumnsSplitByHomeTeam(t *testing.T) {
	m := fixtureList(fixtureDay(), 100, 30)
	east, west := m.visibleGames()
	if len(east) != 3 || len(west) != 3 {
		t.Fatalf("want 3 east and 3 west games, got %d and %d", len(east), len(west))
	}
	for _, g := range east {
		if got := backend.GameConference(g); got != "east" {
			t.Errorf("east column holds %s hosting %s", g.AwayTeam, g.HomeTeam)
		}
	}
	// Cross-conference games follow the host: game 1 (Lakers @ Celtics) is
	// east, game 3 (Celtics @ Lakers) is west.
	if east[0].GameId != "1" || west[0].GameId != "3" {
		t.Errorf("cross-conference games mis-sorted: east[0]=%s west[0]=%s", east[0].GameId, west[0].GameId)
	}
}

func TestUnknownTeamsStillShowUp(t *testing.T) {
	m := fixtureList([]backend.Game{listGame("x", "Unknown", "Unknown", "Final")}, 100, 30)
	east, west := m.visibleGames()
	if len(east)+len(west) != 1 {
		t.Fatal("unrecognized teams must still appear in exactly one column")
	}
	if _, ok := m.selectedGame(); !ok {
		t.Fatal("the fallback game must be selectable")
	}
}

func TestScheduleTwoColumnsFitTypicalDays(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 40}} {
		m := fixtureList(fixtureDay(), size[0], size[1])
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "EASTERN") || !strings.Contains(view, "WESTERN") {
			t.Fatalf("%dx%d: both conference headers must render", size[0], size[1])
		}
		if !strings.Contains(view, "Celtics") || !strings.Contains(view, "Warriors") {
			t.Fatalf("%dx%d: games from both columns must render", size[0], size[1])
		}
		assertViewFits(t, view, size[0], size[1])
	}
}

func TestFullSlatePagesButStaysBounded(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 40}, {160, 50}} {
		m := fixtureList(fullSlate(), size[0], size[1])
		east, west := m.visibleGames()
		if len(east) != 8 || len(west) != 7 {
			t.Fatalf("full slate must split 8/7, got %d/%d", len(east), len(west))
		}
		if m.pageCount() > 2 {
			t.Fatalf("%dx%d: 15 games must fit in at most 2 pages, got %d", size[0], size[1], m.pageCount())
		}
		assertViewFits(t, ansi.Strip(m.View().Content), size[0], size[1])
		// Walk to the last page: every game must be reachable and visible.
		for m.page < m.pageCount()-1 {
			m.movePage(1)
			assertViewFits(t, ansi.Strip(m.View().Content), size[0], size[1])
		}
		seen := map[string]bool{}
		for _, col := range []int{colEast, colWest} {
			for row := range m.columnGames(col) {
				m.setSelection(col, row)
				g, ok := m.selectedGame()
				if !ok {
					t.Fatal("every row must resolve to a game")
				}
				seen[g.GameId] = true
				assertViewFits(t, ansi.Strip(m.View().Content), size[0], size[1])
			}
		}
		if len(seen) != 15 {
			t.Fatalf("only %d of 15 games reachable", len(seen))
		}
	}
}

func TestTabSwitchesConferenceAndEnterOpensIt(t *testing.T) {
	r := newRoot(nil)
	r.current = listview
	r.list = fixtureList(fixtureDay(), 100, 30)
	r.list.loading = false
	r.width, r.height = 100, 30

	if got, _ := r.list.selectedGame(); got.GameId != "1" {
		t.Fatalf("initial selection = %s, want game 1 (east top)", got.GameId)
	}
	r = updateRoot(r, press("tab"))
	if r.list.selCol != colWest {
		t.Fatal("tab must jump to the western column")
	}
	if got, _ := r.list.selectedGame(); got.GameId != "3" {
		t.Fatalf("western selection = %s, want game 3", got.GameId)
	}
	r = updateRoot(r, press("down"))
	if got, _ := r.list.selectedGame(); got.GameId != "4" {
		t.Fatalf("down in west = %s, want game 4", got.GameId)
	}
	r = updateRoot(r, press("tab"))
	if got, _ := r.list.selectedGame(); got.GameId != "1" {
		t.Fatalf("tab back must remember the east selection, got %s", got.GameId)
	}
	r = updateRoot(r, press("enter"))
	if r.current != detailview {
		t.Fatal("enter must open the selected game")
	}
	if r.detail.gameID != "1" {
		t.Fatalf("detail opened game %s, want game 1", r.detail.gameID)
	}
}

func TestListFilterNarrowsBothColumns(t *testing.T) {
	m := fixtureList(fixtureDay(), 100, 30)
	updated, _ := m.Update(press("/"))
	m = updated.(gamelist)
	if !m.filtering {
		t.Fatal("/ must start filtering")
	}
	for _, k := range []string{"l", "a", "k", "e", "r", "s"} {
		updated, _ := m.Update(press(k))
		m = updated.(gamelist)
	}
	east, west := m.visibleGames()
	if len(east) != 1 || len(west) != 1 {
		t.Fatalf("filter 'lakers' must match 1 east and 1 west game, got %d/%d", len(east), len(west))
	}
	updated, _ = m.Update(press("esc"))
	m = updated.(gamelist)
	if m.filter != "" || m.filtering {
		t.Fatal("esc must clear the filter")
	}
	if east, west := m.visibleGames(); len(east)+len(west) != 6 {
		t.Fatal("clearing the filter must restore all games")
	}
}

func TestListRefreshKeepsHighlightOnSameGame(t *testing.T) {
	m := fixtureList(fixtureDay(), 100, 30)
	m.setSelection(colWest, 1)
	before, _ := m.selectedGame()
	before.HomeScore = 99 // simulate a live score update on refresh
	games := fixtureDay()
	for i := range games {
		if games[i].GameId == before.GameId {
			games[i].HomeScore = 99
		}
	}
	m.applyGames(games)
	after, ok := m.selectedGame()
	if !ok || after.GameId != before.GameId || after.HomeScore != 99 {
		t.Fatalf("refresh moved the highlight: %+v", after)
	}
}

func TestUnplayedGamesShowTBDInsteadOfZeroZero(t *testing.T) {
	upcoming := backend.GameFromSeasonSchedule(backend.SeasonGame{
		GameID: "u1", Away: "Lakers", Home: "Celtics", GameStatus: 1,
		DateTimeUTC: "2026-11-03T00:30:00Z", Status: "7:30 PM ET",
	})
	if !upcoming.NotStarted() {
		t.Fatal("upcoming game must report NotStarted")
	}
	line := ansi.Strip(formatGameLine(upcoming, 40, false))
	if !strings.Contains(line, "Lakers @ Celtics") || !strings.Contains(line, "PM") {
		t.Fatalf("unplayed game must show matchup with tip time, got %q", line)
	}
	if strings.Contains(line, "0 @ 0") {
		t.Fatalf("unplayed game must not show zero scores, got %q", line)
	}

	final := backend.GameFromSeasonSchedule(backend.SeasonGame{
		GameID: "f1", Away: "Lakers", Home: "Celtics", GameStatus: 3,
		Status: "Final", AwayScore: 110, HomeScore: 105,
	})
	if line := ansi.Strip(formatGameLine(final, 40, false)); !strings.Contains(line, "Lakers @ Celtics  110 - 105") || !strings.Contains(line, "Final") {
		t.Fatalf("final must show teams with @, scores, and a Final column, got %q", line)
	}
	// All finals use green/red foregrounds for teams and scores.
	rawFinal := formatGameLine(final, 40, false)
	for _, code := range []string{"1;92m", "[91m"} {
		if !strings.Contains(rawFinal, code) {
			t.Fatalf("unselected final must foreground-highlight teams, got %q", rawFinal)
		}
	}
	homeWin := backend.GameFromSeasonSchedule(backend.SeasonGame{
		GameID: "f2", Away: "Lakers", Home: "Celtics", GameStatus: 3,
		Status: "Final", AwayScore: 90, HomeScore: 100,
	})
	if got, want := ansi.Strip(rawFinal), ansi.Strip(formatGameLine(homeWin, 40, false)); !strings.Contains(got, "110") || !strings.Contains(want, "100") {
		t.Fatalf("both final directions must keep scores, got %q / %q", got, want)
	}
	// Only the selected row colors the winner green and the loser red (team
	// names and scores), with no background blocks anywhere. The color codes
	// must sit directly against the text: an inner style between the code
	// and the name would win and wash the row white.
	rawSel := formatGameLine(final, 40, true)
	if ansi.Strip(rawSel) != ansi.Strip(rawFinal) {
		t.Fatalf("selection must not change layout:\n%q\n%q", ansi.Strip(rawSel), ansi.Strip(rawFinal))
	}
	for _, code := range []string{"48;5;22", "48;5;88"} {
		if strings.Contains(rawSel, code) {
			t.Fatalf("selected row must not use background highlights, got %q", rawSel)
		}
	}
	for _, want := range []string{"1;92mLakers", "[91mCeltics", "1;92m110", "[91m105"} {
		if !strings.Contains(rawSel, want) {
			t.Fatalf("selected row must color winner/loser text directly, got %q", rawSel)
		}
	}

	live := backend.GameFromSeasonSchedule(backend.SeasonGame{
		GameID: "l1", Away: "Bulls", Home: "Knicks", GameStatus: 2,
		Status: "Q2 05:00", AwayScore: 50, HomeScore: 48,
	})
	if line := ansi.Strip(formatGameLine(live, 40, false)); !strings.Contains(line, "Bulls @ Knicks") || !strings.Contains(line, "50 - 48") || !strings.Contains(line, "Q2 05:00") {
		t.Fatalf("live game must show teams with @, scores, and the clock column, got %q", line)
	}

	// A scheduled game with no tip-off text published yet gets TBD in the
	// status column; a published tip time renders as-is.
	bare := backend.GameFromSeasonSchedule(backend.SeasonGame{
		GameID: "u2", Away: "Suns", Home: "Warriors", GameStatus: 1,
	})
	if line := ansi.Strip(formatGameLine(bare, 40, false)); !strings.Contains(line, "Suns @ Warriors") || !strings.Contains(line, "TBD") {
		t.Fatalf("game with no tip time must show TBD status, got %q", line)
	}
	if line := ansi.Strip(formatGameLine(upcoming, 40, false)); !strings.Contains(line, "PM") {
		t.Fatalf("published tip time must render as-is, got %q", line)
	}
}

func pickerFixture(daysAgo int) gamelist {
	m := fixtureList(fixtureDay(), 100, 30)
	m.day = localToday().AddDate(0, 0, -daysAgo)
	m.loading = false
	return m
}

func openPicker(t *testing.T, m gamelist) gamelist {
	t.Helper()
	updated, _ := m.Update(press("d"))
	m = updated.(gamelist)
	if !m.entering {
		t.Fatal("d must open the date picker")
	}
	if !sameDay(m.pickDay, m.day) {
		t.Fatal("picker must start on the viewed day")
	}
	return m
}

func TestDatePickerMovesByDayWeekAndMonth(t *testing.T) {
	m := openPicker(t, pickerFixture(60))
	start := m.pickDay

	updated, _ := m.Update(press("left"))
	m = updated.(gamelist)
	if !sameDay(m.pickDay, start.AddDate(0, 0, -1)) {
		t.Fatalf("left must step a day back, got %v", m.pickDay)
	}
	updated, _ = m.Update(press("right"))
	updated, _ = updated.(gamelist).Update(press("right"))
	m = updated.(gamelist)
	if !sameDay(m.pickDay, start.AddDate(0, 0, 1)) {
		t.Fatalf("right must step a day forward, got %v", m.pickDay)
	}
	updated, _ = m.Update(press("down"))
	m = updated.(gamelist)
	if !sameDay(m.pickDay, start.AddDate(0, 0, 8)) {
		t.Fatalf("down must step a week forward, got %v", m.pickDay)
	}
	updated, _ = m.Update(press("ctrl+left"))
	m = updated.(gamelist)
	if !sameDay(m.pickDay, start.AddDate(0, 0, 1)) {
		t.Fatalf("ctrl+left must step a week back, got %v", m.pickDay)
	}
	updated, _ = m.Update(press("ctrl+down"))
	m = updated.(gamelist)
	if !sameDay(m.pickDay, start.AddDate(0, 0, 31)) {
		t.Fatalf("ctrl+down must hop a month forward, got %v", m.pickDay)
	}
	updated, _ = m.Update(press("ctrl+up"))
	m = updated.(gamelist)
	if !sameDay(m.pickDay, start.AddDate(0, 0, 1)) {
		t.Fatalf("ctrl+up must hop a month back, got %v", m.pickDay)
	}
}

func TestDatePickerClampsAtTodayAndCancelsCleanly(t *testing.T) {
	m := pickerFixture(0)
	m = openPicker(t, m)
	for _, k := range []string{"right", "ctrl+right", "down", "ctrl+down"} {
		updated, _ := m.Update(press(k))
		m = updated.(gamelist)
		if !sameDay(m.pickDay, localToday()) {
			t.Fatalf("%s must not move past today, got %v", k, m.pickDay)
		}
	}
	updated, _ := m.Update(press("esc"))
	m = updated.(gamelist)
	if m.entering {
		t.Fatal("esc must close the picker")
	}
	if !sameDay(m.day, localToday()) {
		t.Fatal("esc must leave the viewed day alone")
	}
}

func TestDatePickerEnterJumpsToPickedDay(t *testing.T) {
	m := openPicker(t, pickerFixture(60))
	updated, _ := m.Update(press("left"))
	m = updated.(gamelist)
	want := m.pickDay
	updated, cmd := m.Update(press("enter"))
	m = updated.(gamelist)
	if m.entering {
		t.Fatal("enter must close the picker")
	}
	if !sameDay(m.day, want) {
		t.Fatalf("enter must jump to the picked day, got %v", m.day)
	}
	if cmd == nil {
		t.Fatal("enter must trigger a fetch for the picked day")
	}
}

func TestSelectedFinalRowHighlightsUniformly(t *testing.T) {
	games := []backend.Game{
		backend.GameFromSeasonSchedule(backend.SeasonGame{GameID: "1", Away: "Lakers", Home: "Celtics", GameStatus: 3, Status: "Final", AwayScore: 110, HomeScore: 105}),
		backend.GameFromSeasonSchedule(backend.SeasonGame{GameID: "2", Away: "Bulls", Home: "Knicks", GameStatus: 3, Status: "Final", AwayScore: 90, HomeScore: 100}),
	}
	m := fixtureList(games, 100, 30)
	m.setSelection(colEast, 1)
	var selected, plain string
	for _, line := range strings.Split(m.conferencePanel(colEast, 60), "\n") {
		if strings.Contains(line, "▶") {
			selected = line
		} else if strings.Contains(line, "Lakers") {
			plain = line
		}
	}
	if selected == "" || plain == "" {
		t.Fatal("panel must render selected and unselected rows")
	}
	// The selected row uses text colors only: a background reset inside it
	// would cancel the selection style partway through the row.
	for _, bg := range []string{"48;5;22", "48;5;88"} {
		if strings.Contains(selected, bg) {
			t.Fatalf("selected row must use text highlights, got %q", selected)
		}
	}
	// Unselected rows use the same readable foreground colors.
	if strings.Contains(plain, "48;5;") || !strings.Contains(plain, "1;92mLakers") {
		t.Fatalf("unselected row must use foreground colors without backgrounds, got %q", plain)
	}
	if got := ansi.Strip(selected); !strings.Contains(got, "Bulls @ Knicks") || !strings.Contains(got, "90 - 100") {
		t.Fatalf("selected row must keep the full matchup, got %q", got)
	}
}

func TestRolloverLeavesDeliberateDayAlone(t *testing.T) {
	m := pickerFixture(60)
	m.day = localToday().AddDate(0, 0, -1)
	m.todayStamp = localToday()
	m, cmd := m.rolloverCmd()
	if !sameDay(m.day, localToday().AddDate(0, 0, -1)) {
		t.Fatalf("tick must not yank yesterday back to today, got %v", m.day)
	}
	if cmd != nil {
		t.Fatal("tick on a deliberate day must not refetch")
	}
	// Sitting on today still auto-refreshes.
	m.day = localToday()
	m, _ = m.rolloverCmd()
	if m.todayStamp.IsZero() || !sameDay(m.todayStamp, localToday()) {
		t.Fatal("tick must keep the stamp current")
	}
}

func TestRolloverFollowsRealMidnight(t *testing.T) {
	m := pickerFixture(60)
	// Stamp matches the viewed day but "today" has moved on: local midnight
	// passed while the list was open.
	m.day = localToday().AddDate(0, 0, -1)
	m.todayStamp = localToday().AddDate(0, 0, -1)
	m, cmd := m.rolloverCmd()
	if !sameDay(m.day, localToday()) {
		t.Fatalf("tick must follow a real midnight rollover, got %v", m.day)
	}
	if cmd == nil {
		t.Fatal("midnight rollover must fetch the new day")
	}
}

func TestDatePickerRendersMonthCalendar(t *testing.T) {
	m := openPicker(t, pickerFixture(60))
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, m.pickDay.Format("January 2006")) {
		t.Fatalf("picker must show the cursor month, got %q", view)
	}
	for _, wd := range []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"} {
		if !strings.Contains(view, wd) {
			t.Fatalf("picker must show weekday headers, got %q", view)
		}
	}
	if !strings.Contains(view, m.pickDay.Format("Jan 2, 2006")) {
		t.Fatalf("picker must show the picked date, got %q", view)
	}
	assertViewFits(t, view, 100, 30)
}

func TestNarrowTerminalShowsActiveConference(t *testing.T) {
	m := fixtureList(fixtureDay(), 60, 24)
	view := ansi.Strip(m.View().Content)
	if strings.Contains(view, "WESTERN") && strings.Contains(view, "EASTERN") {
		t.Fatal("narrow terminal must show only one conference at a time")
	}
	assertViewFits(t, view, 60, 24)
	updated, _ := m.Update(press("tab"))
	m = updated.(gamelist)
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Warriors") {
		t.Fatal("tab must reveal the western conference on narrow terminals")
	}
	assertViewFits(t, view, 60, 24)
}
