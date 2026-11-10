package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/NolanFogarty/courtside/backend"
	"github.com/charmbracelet/x/ansi"
)

var docStyle = lipgloss.NewStyle().Margin(1, 2)

// Conference columns on the daily schedule: 0 renders left, 1 renders right.
const (
	colEast = iota
	colWest
)

// minTwoColWidth is the narrowest content width that still fits two game
// columns side by side. Below it only the active conference renders.
const minTwoColWidth = 76

type gamelist struct {
	games         []backend.Game // every game for the day, in fetch order
	filter        string         // team-name substring, empty means unfiltered
	filtering     bool           // typing a filter; owns every keypress
	selCol        int            // active conference column (colEast/colWest)
	eastSel       int            // selected row within the visible east games
	westSel       int            // selected row within the visible west games
	page          int            // shared page across both columns
	day           time.Time      // the date currently being viewed
	todayStamp    time.Time      // local "today" as of the last tick; spots real midnight rollovers
	seq           int            // bumped on each day change; debounces rapid navigation
	loading       bool
	err           error
	entering      bool      // date picker open
	pickDay       time.Time // date picker cursor
	width, height int       // terminal size, for the bottom navigation
}

// daySettledMsg fires after a short pause following a day change; the fetch only
// runs if seq still matches (i.e. no newer keypress superseded it).
type daySettledMsg struct{ seq int }

// gamesLoadedMsg carries the result of an async day fetch.
type gamesLoadedMsg struct {
	day   time.Time
	games []backend.Game
	err   error
}

// fetchDebounce is how long to wait after the last day change before fetching.
const fetchDebounce = 250 * time.Millisecond

func (m gamelist) Init() tea.Cmd {
	// Startup opens instantly with an empty list and fills it in async, so a
	// hanging scoreboard endpoint delays data, not the whole UI.
	if m.loading && len(m.games) == 0 {
		return fetchGames(m.day)
	}
	return nil
}

func (m gamelist) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Keep the size regardless of mode.
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = ws.Width, ws.Height
		m.clamp()
		return m, nil
	}

	// While the date picker is open, it owns every keypress.
	if m.entering {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			if key.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m.updateDatePicker(key)
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// While typing a filter, every key edits the filter text.
		if m.filtering {
			m.updateFilter(msg.String())
			return m, nil
		}
		switch msg.String() {
		case "d":
			return m.startDatePicker()
		case "left", "h":
			return m.gotoDay(-1)
		case "right", "l":
			return m.gotoDay(1)
		case "/":
			m.filtering = true
			m.clamp()
			return m, nil
		case "tab", "shift+tab":
			m.switchColumn()
			return m, nil
		case "up", "k":
			m.moveSelection(-1)
			return m, nil
		case "down", "j":
			m.moveSelection(1)
			return m, nil
		case "pgup":
			m.movePage(-1)
			return m, nil
		case "pgdown":
			m.movePage(1)
			return m, nil
		}
	case daySettledMsg:
		if msg.seq != m.seq {
			return m, nil // superseded by a newer day change; don't fetch
		}
		return m, fetchGames(m.day)
	case gamesLoadedMsg:
		if !sameDay(msg.day, m.day) {
			return m, nil // a newer day was requested; ignore stale result
		}
		wasLoading := m.loading
		m.loading = false
		if msg.err != nil {
			// Only surface an error on a user-initiated load; ignore transient
			// auto-refresh failures and keep showing the current games.
			if wasLoading {
				m.err = msg.err
			}
		} else {
			m.err = nil
			m.applyGames(msg.games)
		}
		return m, nil
	}

	return m, nil
}

// gotoDay moves the viewed date by delta days immediately, but defers the fetch
// behind a debounce so spamming the key doesn't hammer the API.
func (m gamelist) gotoDay(delta int) (gamelist, tea.Cmd) {
	next := m.day.AddDate(0, 0, delta)
	// Don't navigate past today (local) — there are no future results to show.
	if delta > 0 && afterToday(next) {
		return m, nil
	}
	m.day = next
	m.loading = true
	m.err = nil
	m.seq++
	m.page = 0
	m.eastSel, m.westSel = 0, 0
	seq := m.seq
	return m, tea.Tick(fetchDebounce, func(time.Time) tea.Msg {
		return daySettledMsg{seq: seq}
	})
}

// startDatePicker opens the date picker on the currently viewed day.
func (m gamelist) startDatePicker() (gamelist, tea.Cmd) {
	m.entering = true
	m.pickDay = m.day
	return m, nil
}

// updateDatePicker moves the picker cursor: arrows step a day, up/down and
// ctrl+arrows step a week, ctrl+up/down hop a month. The cursor never moves
// past today (local), which has no future games to show. Enter jumps, esc
// cancels.
func (m gamelist) updateDatePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.entering = false
		return m, nil
	case "enter":
		m.entering = false
		return m.jumpToDay(m.pickDay)
	case "left", "h":
		m.movePickDay(-1)
	case "right", "l":
		m.movePickDay(1)
	case "up", "k", "ctrl+left":
		m.movePickDay(-7)
	case "down", "j", "ctrl+right":
		m.movePickDay(7)
	case "ctrl+up":
		m.movePickDay(-30)
	case "ctrl+down":
		m.movePickDay(30)
	}
	return m, nil
}

// movePickDay shifts the picker cursor, clamping it to today (local).
func (m *gamelist) movePickDay(delta int) {
	next := m.pickDay.AddDate(0, 0, delta)
	if afterToday(next) {
		next = localToday()
	}
	m.pickDay = next
}

// jumpToDay switches directly to an absolute date and fetches it immediately
// (no debounce — the user explicitly asked for this date).
func (m gamelist) jumpToDay(day time.Time) (gamelist, tea.Cmd) {
	if afterToday(day) {
		day = localToday()
	}
	m.day = day
	m.loading = true
	m.err = nil
	m.seq++ // invalidate any pending day-navigation debounce ticks
	m.page = 0
	m.eastSel, m.westSel = 0, 0
	return m, fetchGames(m.day)
}

func fetchGames(day time.Time) tea.Cmd {
	return func() tea.Msg {
		games, err := backend.GetGamesForDate(day.Format("2006-01-02"))
		return gamesLoadedMsg{day: day, games: games, err: err}
	}
}

// refreshCmd silently re-fetches the current day's games when viewing today,
// where live scores change. Past/future days are static, so it returns nil.
func (m gamelist) refreshCmd() tea.Cmd {
	if m.entering {
		return nil
	}
	if !sameDay(m.day, localToday()) {
		return nil
	}
	return fetchGames(m.day)
}

// rolloverCmd advances the viewed day to today when local midnight passes while
// the list is open, so scores don't freeze on a stale day. Days the user
// deliberately navigated to are left alone: the stamp records what "today"
// was at the last check, so only a genuine midnight rollover — the viewed
// day matching the old stamp but not the new day — moves the view.
func (m gamelist) rolloverCmd() (gamelist, tea.Cmd) {
	if m.entering {
		return m, nil
	}
	now := localToday()
	if !m.todayStamp.IsZero() && sameDay(m.day, m.todayStamp) && !sameDay(m.day, now) {
		m.day = now
		m.loading = true
		m.err = nil
		m.seq++
		m.page = 0
		m.eastSel, m.westSel = 0, 0
		m.todayStamp = now
		return m, fetchGames(m.day)
	}
	m.todayStamp = now
	if sameDay(m.day, now) {
		return m, m.refreshCmd()
	}
	return m, nil
}

// visibleGames returns the filtered games split into east (home team in the
// Eastern Conference) and west columns, each in fetch order. Cross-conference
// matchups sort by the home team; unrecognized teams balance into the shorter
// column so no game is dropped.
func (m gamelist) visibleGames() (east, west []backend.Game) {
	for _, g := range m.games {
		if m.filter != "" && !strings.Contains(strings.ToLower(g.AwayTeam+" "+g.HomeTeam), strings.ToLower(m.filter)) {
			continue
		}
		switch backend.GameConference(g) {
		case "east":
			east = append(east, g)
		case "west":
			west = append(west, g)
		default:
			if len(east) <= len(west) {
				east = append(east, g)
			} else {
				west = append(west, g)
			}
		}
	}
	return east, west
}

// columnGames returns the visible games for one conference column.
func (m gamelist) columnGames(col int) []backend.Game {
	east, west := m.visibleGames()
	if col == colWest {
		return west
	}
	return east
}

// selectedRow returns the clamped selection within the active column.
func (m gamelist) selectedRow() int {
	rows := m.columnGames(m.selCol)
	if len(rows) == 0 {
		return 0
	}
	sel := m.eastSel
	if m.selCol == colWest {
		sel = m.westSel
	}
	return max(0, min(sel, len(rows)-1))
}

// selectedGame returns the highlighted game for the detail view.
func (m gamelist) selectedGame() (backend.Game, bool) {
	rows := m.columnGames(m.selCol)
	if len(rows) == 0 {
		return backend.Game{}, false
	}
	return rows[m.selectedRow()], true
}

// setSelection records a row selection and pulls the shared page along so the
// highlighted game is always on screen.
func (m *gamelist) setSelection(col, row int) {
	if col == colWest {
		m.westSel = max(0, row)
	} else {
		m.eastSel = max(0, row)
	}
	m.selCol = col
	m.page = m.selectedRow() / m.pageRows()
}

// moveSelection steps the highlight within the active column, turning the page
// when it walks past the visible rows.
func (m *gamelist) moveSelection(delta int) {
	rows := m.columnGames(m.selCol)
	if len(rows) == 0 {
		m.switchColumn()
		return
	}
	m.setSelection(m.selCol, m.selectedRow()+delta)
}

// switchColumn jumps the highlight to the other conference, landing on the
// other column's remembered game and its page.
func (m *gamelist) switchColumn() {
	other := colEast
	if m.selCol == colEast {
		other = colWest
	}
	if len(m.columnGames(other)) == 0 {
		return // nothing to jump to; stay put
	}
	m.selCol = other
	m.page = m.selectedRow() / m.pageRows()
}

// movePage flips the shared page and lands the highlight on the first game of
// the new page within the active column.
func (m *gamelist) movePage(delta int) {
	if m.pageCount() <= 1 {
		return
	}
	m.page = max(0, min(m.page+delta, m.pageCount()-1))
	rows := m.columnGames(m.selCol)
	if len(rows) == 0 {
		return
	}
	top := min(m.page*m.pageRows(), len(rows)-1)
	m.setSelection(m.selCol, top)
}

// updateFilter edits the team-name filter; the selection resets to the top of
// the first page on every change.
func (m *gamelist) updateFilter(k string) {
	switch k {
	case "enter":
		m.filtering = false
	case "esc":
		m.filtering = false
		m.filter = ""
	case "backspace":
		if m.filter != "" {
			m.filter = m.filter[:len(m.filter)-1]
		}
	default:
		if len(k) == 1 && k[0] >= 32 && k[0] < 127 {
			m.filter += k
		}
	}
	m.eastSel, m.westSel, m.page = 0, 0, 0
	m.clamp()
}

// applyGames replaces the day's games, keeping the highlight on the same game
// across live auto-refreshes when it is still on the board.
func (m *gamelist) applyGames(games []backend.Game) {
	var id string
	if sel, ok := m.selectedGame(); ok {
		id = sel.GameId
	}
	m.games = games
	m.eastSel, m.westSel, m.page = 0, 0, 0
	if id != "" {
		east, west := m.visibleGames()
		for i, g := range east {
			if g.GameId == id {
				m.selCol, m.eastSel = colEast, i
				break
			}
		}
		for i, g := range west {
			if g.GameId == id {
				m.selCol, m.westSel = colWest, i
				break
			}
		}
	}
	m.clamp()
}

// clamp keeps the column, selections, and page inside the visible games.
func (m *gamelist) clamp() {
	east, west := m.visibleGames()
	m.eastSel = clampInt(m.eastSel, 0, max(0, len(east)-1))
	m.westSel = clampInt(m.westSel, 0, max(0, len(west)-1))
	if m.selCol == colWest && len(west) == 0 && len(east) > 0 {
		m.selCol = colEast
	}
	if m.selCol == colEast && len(east) == 0 && len(west) > 0 {
		m.selCol = colWest
	}
	m.page = clampInt(m.page, 0, max(0, m.pageCount()-1))
	// Keep the highlight on the current page after a resize or reload.
	rows := m.pageRows()
	sel := m.selectedRow()
	if sel < m.page*rows || sel >= (m.page+1)*rows {
		m.page = sel / rows
	}
}

// pageCount is how many pages the taller column needs at the current height.
func (m gamelist) pageCount() int {
	east, west := m.visibleGames()
	tallest := max(len(east), len(west))
	if tallest == 0 {
		return 1
	}
	return max(1, (tallest+m.pageRows()-1)/m.pageRows())
}

// pageRows is how many games fit vertically in one column.
// Each game takes one line (matchup plus status column); each panel costs
// its border (2) plus a conference header and divider (2).
func (m gamelist) pageRows() int {
	_, vFrame := docStyle.GetFrameSize()
	titleH := lipgloss.Height(headerBarSty.Render("X"))
	room := m.height - vFrame - titleH - 1 /* blank */ - lipgloss.Height(m.hints()) - 1 /* spacer */
	if m.filtering {
		room-- // the filter help line under the columns
	}
	return max(1, room-4)
}

func (m gamelist) title() string {
	label := m.day.Format("Mon Jan 2")
	if sameDay(m.day, localToday()) {
		label = "Today · " + label
	}
	t := "NBA Scores — " + label
	switch {
	case m.loading:
		t += "  (loading…)"
	case m.err != nil:
		t += "  (failed to load)"
	}
	if m.filtering {
		t += "  /" + m.filter
	} else if m.filter != "" {
		t += "  · /" + m.filter
	}
	if m.pageCount() > 1 {
		t += fmt.Sprintf("  · page %d/%d", m.page+1, m.pageCount())
	}
	if contentWidth(m.width) < minTwoColWidth {
		conf := "EASTERN"
		if m.selCol == colWest {
			conf = "WESTERN"
		}
		t += "  · " + conf
	}
	return t
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

// afterToday reports whether t's calendar date is after today (local). Both
// sides are normalized to their calendar date so the comparison ignores time
// of day and location differences.
func afterToday(t time.Time) bool {
	today := localToday()
	ty, tm, td := t.Date()
	ny, nm, nd := today.Date()
	return time.Date(ty, tm, td, 0, 0, 0, 0, time.UTC).
		After(time.Date(ny, nm, nd, 0, 0, 0, 0, time.UTC))
}

// localToday returns the current date in the viewer's local time zone.
func localToday() time.Time {
	return time.Now().Local()
}

func (m gamelist) View() tea.View {
	// While picking a date, show the month calendar instead of the schedule.
	if m.entering {
		return m.datePickerView()
	}

	title := lipgloss.PlaceHorizontal(contentWidth(m.width), lipgloss.Center,
		headerBarSty.Render(ansi.Truncate(m.title(), max(1, contentWidth(m.width)-4), "…")))

	var main string
	east, west := m.visibleGames()
	switch {
	case len(m.games) == 0:
		// Mirror the old empty list: just the title and a single message.
		note := "No games on this date."
		if m.loading {
			note = "Loading…"
		} else if m.err != nil {
			note = "Failed to load: " + m.err.Error()
		}
		main = lipgloss.PlaceHorizontal(contentWidth(m.width), lipgloss.Center,
			lipgloss.NewStyle().Faint(true).Render(note))
	case len(east) == 0 && len(west) == 0:
		main = lipgloss.PlaceHorizontal(contentWidth(m.width), lipgloss.Center,
			mutedSty.Render("No games match this filter."))
	default:
		main = m.columnsView()
	}

	if m.filtering {
		main += "\n" + lipgloss.PlaceHorizontal(contentWidth(m.width), lipgloss.Center,
			mutedSty.Render("Type a team name • enter applies • esc clears"))
	}

	body := lipgloss.JoinVertical(lipgloss.Left, title, "", main)
	hint := m.hints()

	_, vFrame := docStyle.GetFrameSize()
	spacerH := m.height - vFrame - lipgloss.Height(body) - lipgloss.Height(hint)
	if spacerH < 1 {
		spacerH = 1
	}
	spacer := lipgloss.NewStyle().Height(spacerH).Render("")
	content := lipgloss.JoinVertical(lipgloss.Left, body, spacer, hint)

	v := tea.NewView(docStyle.Render(content))
	v.AltScreen = true
	return v
}

// datePickerView renders the month calendar for "go to date" in place of the
// schedule. The cursor day is bright, today is underlined, the currently
// viewed day is blue, and days past today are dimmed (unreachable).
func (m gamelist) datePickerView() tea.View {
	body := lipgloss.PlaceHorizontal(contentWidth(m.width), lipgloss.Center,
		panelSty.Render(lipgloss.JoinVertical(lipgloss.Left, m.calendarLines()...)))
	subtitle := m.pickDay.Format("Mon Jan 2, 2006")
	if sameDay(m.pickDay, localToday()) {
		subtitle += " · today"
	}
	return pageView(m.width, m.height, "Go to date", subtitle, body, m.pickerHints())
}

func (m gamelist) pickerHints() string {
	return renderHints(contentWidth(m.width),
		[2]string{"←/→", "day"}, [2]string{"↑/↓", "week"},
		[2]string{"ctrl+←/→", "week"}, [2]string{"ctrl+↑/↓", "month"},
		[2]string{"enter", "go"}, [2]string{"esc", "cancel"})
}

// calendarLines renders the picker's month grid: a title, weekday headers,
// and one 4-column cell per day.
func (m gamelist) calendarLines() []string {
	y, mo, _ := m.pickDay.Date()
	first := time.Date(y, mo, 1, 0, 0, 0, 0, time.UTC)
	daysInMonth := time.Date(y, mo+1, 0, 0, 0, 0, 0, time.UTC).Day()
	today := localToday()

	title := lipgloss.PlaceHorizontal(28, lipgloss.Center,
		lipgloss.NewStyle().Bold(true).Render(first.Format("January 2006")))
	head := ""
	for i, wd := range []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"} {
		if i > 0 {
			head += " "
		}
		head += colHeaderSty.Render(fmt.Sprintf(" %-2s ", wd))
	}
	lines := []string{title, head}
	week := strings.Repeat("    ", int(first.Weekday()))
	for d := 1; d <= daysInMonth; d++ {
		day := time.Date(y, mo, d, 0, 0, 0, 0, time.UTC)
		cell := fmt.Sprintf(" %2d ", d)
		switch {
		case sameDay(day, m.pickDay):
			cell = accentSty.Render(cell)
		case sameDay(day, today):
			cell = lipgloss.NewStyle().Bold(true).Underline(true).Render(cell)
		case sameDay(day, m.day):
			cell = lipgloss.NewStyle().Foreground(awayColor).Render(cell)
		case afterToday(day):
			cell = mutedSty.Render(cell)
		}
		week += cell
		if (int(first.Weekday())+d)%7 == 0 {
			lines = append(lines, week)
			week = ""
		} else {
			week += " "
		}
	}
	if strings.TrimSpace(week) != "" {
		lines = append(lines, week)
	}
	return lines
}

// columnsView renders the two conference panels side by side (or just the
// active one on narrow terminals), paged to the terminal height.
func (m gamelist) columnsView() string {
	w := contentWidth(m.width)
	if w < minTwoColWidth {
		col := colEast
		if m.selCol == colWest {
			col = colWest
		}
		panel := m.conferencePanel(col, w)
		return lipgloss.PlaceHorizontal(w, lipgloss.Center, panel)
	}
	colW := (w - 2) / 2 // two panels plus a two-space gap
	east := m.conferencePanel(colEast, colW)
	west := m.conferencePanel(colWest, colW)
	tables := lipgloss.JoinHorizontal(lipgloss.Top, east, "  ", west)
	return lipgloss.PlaceHorizontal(w, lipgloss.Center, tables)
}

// conferencePanel renders one conference's games for the current page. width
// is the panel's total width including its border; every content line is a
// two-cell gutter plus text so panels line up with or without a highlight.
func (m gamelist) conferencePanel(col, width int) string {
	games := m.columnGames(col)
	name := "EASTERN"
	color := awayColor
	if col == colWest {
		name = "WESTERN"
		color = homeColor
	}
	inner := max(22, width-4) // inside the border plus padding
	text := inner - 2         // inside the selection gutter
	lines := []string{lipgloss.PlaceHorizontal(inner, lipgloss.Center,
		lipgloss.NewStyle().Bold(true).Foreground(color).Render(name))}
	lines = append(lines, dimRowSty.Render(strings.Repeat("─", inner)))

	rows := m.pageRows()
	start := min(m.page*rows, max(0, len(games)-1))
	if len(games) == 0 {
		start = 0
	}
	end := min(start+rows, len(games))
	sel := -1
	if col == m.selCol {
		sel = m.selectedRow()
	}
	for i := start; i < end; i++ {
		line := formatGameLine(games[i], text, i == sel)
		if i == sel {
			line = "▶ " + line
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	if len(games) == 0 {
		lines = append(lines, "  "+mutedSty.Render(pad("No games.", text, false)))
	}

	sty := panelSty
	if col == m.selCol && len(games) > 0 {
		sty = focusedPanelSty
	}
	return sty.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// Winner/loser colors for final games: bright green for the winner's team
// and score, bright red for the loser's. No background blocks.
var (
	winTextSty  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	lossTextSty = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

// formatGameLine renders one game as a single line: the matchup on the left
// and the status (Final, live clock, tip time, or TBD) in its own column on
// the right. Played games read "AWAY @ HOME  110 - 105". Finals highlight
// the winner green and the loser red: unselected rows put the team names on
// background blocks (scores go green/red text), while the selected row uses
// green/red text for teams and scores with the rest in white.
func formatGameLine(g backend.Game, width int, selected bool) string {
	// Base style for the row's ordinary text: plain normally, white when
	// selected. Winner/loser parts keep their green/red on both.
	base := func(s string) string {
		if selected {
			return accentSty.Render(s)
		}
		return s
	}
	status := gameStatusText(g)
	switch {
	case g.IsLive():
		status = liveSty.Render(status)
	case selected:
		status = accentSty.Render(status)
	default:
		status = mutedSty.Render(status)
	}
	statusW := lipgloss.Width(status)
	room := max(8, width-statusW-1)
	var matchup string
	if g.NotStarted() {
		teams := room - len(" @ ")
		away := ansi.Truncate(g.AwayTeam, (teams+1)/2, "…")
		home := ansi.Truncate(g.HomeTeam, teams-lipgloss.Width(away), "…")
		matchup = base(away) + base(" @ ") + base(home)
	} else {
		awayScore := fmt.Sprintf("%3d", g.AwayScore)
		homeScore := fmt.Sprintf("%-3d", g.HomeScore)
		teams := room - lipgloss.Width(" @ "+"  "+awayScore+" - "+homeScore)
		away := ansi.Truncate(g.AwayTeam, (teams+1)/2, "…")
		home := ansi.Truncate(g.HomeTeam, teams-lipgloss.Width(away), "…")
		at, pad2, dash := base(" @ "), base("  "), base(" - ")
		// Winner/loser styles wrap the plain strings directly: wrapping an
		// already-styled span buries the color under an inner reset and the
		// selected row renders all white.
		if g.IsFinal() {
			switch {
			case g.AwayScore > g.HomeScore:
				awayScore = winTextSty.Render(awayScore)
				homeScore = lossTextSty.Render(homeScore)
				if selected {
					away, home = winTextSty.Render(away), lossTextSty.Render(home)
				} else {
					away, home = winBg.Render(away), lossBg.Render(home)
				}
			case g.HomeScore > g.AwayScore:
				awayScore = lossTextSty.Render(awayScore)
				homeScore = winTextSty.Render(homeScore)
				if selected {
					away, home = lossTextSty.Render(away), winTextSty.Render(home)
				} else {
					away, home = lossBg.Render(away), winBg.Render(home)
				}
			default:
				away, home = base(away), base(home)
				awayScore, homeScore = base(awayScore), base(homeScore)
			}
		} else {
			away, home = base(away), base(home)
			awayScore, homeScore = base(awayScore), base(homeScore)
		}
		matchup = away + at + home + pad2 + awayScore + dash + homeScore
	}
	gap := max(0, width-lipgloss.Width(matchup)-statusW)
	return matchup + strings.Repeat(" ", gap) + status
}

// gameStatusText is the plain status for the game line's right column: the
// tip-off time (or TBD) for scheduled games, the quarter clock for live
// games, and "Final" for finished ones.
func gameStatusText(g backend.Game) string {
	switch {
	case g.IsFinal():
		return "Final"
	case g.IsLive():
		return g.GameClock
	default:
		if strings.TrimSpace(g.GameClock) == "" {
			return "TBD"
		}
		return g.GameClock
	}
}

func (m gamelist) hints() string {
	if m.filtering {
		return renderHints(contentWidth(m.width), [2]string{"enter", "apply"}, [2]string{"esc", "clear"})
	}
	back := [2]string{"↑/↓", "move"}
	pairs := [][2]string{back, [2]string{"tab", "conference"}, [2]string{"pgup/pgdn", "page"}, [2]string{"←/→", "day"}, [2]string{"d", "date"}, [2]string{"s", "standings"}, [2]string{"a", "schedule"}, [2]string{"t", "teams"}, [2]string{"/", "filter"}, [2]string{"enter", "open"}, [2]string{"q", "quit"}}
	if m.filter != "" {
		pairs = append(pairs, [2]string{"esc", "clear filter"})
	}
	return renderHints(contentWidth(m.width), pairs...)
}

func newGamesList(games []backend.Game) gamelist {
	m := gamelist{
		day:        localToday(),
		todayStamp: localToday(),
		// With no seeded games (offline or slow startup fetch), show the
		// spinner and let Init fetch async instead of rendering an empty day.
		loading: len(games) == 0,
	}
	m.applyGames(games)
	m.loading = len(games) == 0
	return m
}

func Run(games []backend.Game) error {
	m := newRoot(games)
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
