package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/NolanFogarty/courtside/backend"
)

var teamTabs = []string{"Roster", "Overview", "Injuries", "Salaries", "Games"}

type teams struct {
	sortMode                      bool
	orders                        [5]columnSort
	columnOffsets                 [5]int
	width, height                 int
	browsing, filtering           bool
	filter                        string
	choice, selected, offset, tab int
	loading                       bool
	request                       int
	data                          backend.TeamProfile
}
type teamLoadedMsg struct {
	id, request int
	data        backend.TeamProfile
}

func newTeams(w, h int) teams { return teams{width: w, height: h, browsing: true} }
func (m teams) choices() []backend.Team {
	out := []backend.Team{}
	for _, t := range backend.Teams {
		if strings.Contains(strings.ToLower(t.Name+" "+t.Code), strings.ToLower(m.filter)) {
			out = append(out, t)
		}
	}
	return out
}
func (m teams) open(t backend.Team) (teams, tea.Cmd) {
	m.browsing = false
	m.loading = true
	m.selected = 0
	m.offset = 0
	m.tab = 0
	m.request++
	m.sortMode = false
	m.orders = [5]columnSort{}
	m.columnOffsets = [5]int{}
	m.data = backend.TeamProfile{Team: t}
	return m, m.fetch()
}
func (m teams) fetch() tea.Cmd {
	t, request := m.data.Team, m.request
	return func() tea.Msg { return teamLoadedMsg{t.ID, request, backend.GetTeamProfile(t)} }
}
func (m teams) order() columnSort {
	s := m.orders[m.tab]
	if s.key == "" {
		s.key = "NAME"
		if m.tab == 3 {
			s.key = "SALARY"
			s.desc = true
		}
		if m.tab == 4 {
			s.key = "GAME_DATE"
			s.desc = true
		}
	}
	return s
}
func (m teams) columns() []dataColumn {
	if m.tab == 4 {
		return []dataColumn{dateColumn("GAME_DATE", "DATE", 13), textColumn("MATCHUP", "MATCHUP", 16), textColumn("WL", "W/L", 5), numberColumn("PTS", "PTS", 5, 0), numberColumn("REB", "REB", 5, 0), numberColumn("AST", "AST", 5, 0)}
	}
	cols := []dataColumn{textColumn("NAME", "PLAYER", 24)}
	if m.tab == 3 {
		return append(cols, textColumn("POSITION", "POS", 5), numberColumn("SALARY", "SALARY", 14, 0), numberColumn("GP", "GP", 4, 0), numberColumn("PTS", "PTS", 5, 1), textColumn("STATUS", "STATUS", 11))
	}
	return append(cols, numberColumn("NUM", "#", 4, 0), textColumn("POSITION", "POS", 5), numberColumn("AGE", "AGE", 4, 0), numberColumn("GP", "GP", 4, 0), numberColumn("MIN", "MIN", 5, 1), numberColumn("PTS", "PTS", 5, 1), numberColumn("REB", "REB", 5, 1), numberColumn("AST", "AST", 5, 1), percentColumn("FG_PCT", "FG%"))
}
func (m teams) rosterRows() []backend.Row {
	rows := []backend.Row{}
	for _, p := range m.data.Roster {
		r := dataRow(map[string]any{"ID": p.ID, "NAME": p.Name, "NUM": p.Number, "POSITION": p.Position})
		if p.Age > 0 {
			r["AGE"] = dataRow(map[string]any{"AGE": p.Age})["AGE"]
		}
		for k, v := range p.Stats {
			r[k] = v
		}
		if p.Salary != nil {
			r["SALARY"] = dataRow(map[string]any{"SALARY": *p.Salary})["SALARY"]
		}
		if p.Injury != nil {
			r["STATUS"] = dataRow(map[string]any{"STATUS": p.Injury.Status})["STATUS"]
		}
		rows = append(rows, r)
	}
	return orderedRows(rows, m.columns(), m.order())
}
func (m teams) players() []backend.RosterPlayer {
	byID := map[int]backend.RosterPlayer{}
	for _, p := range m.data.Roster {
		if p.ID != 0 {
			byID[p.ID] = p
		}
	}
	rows := []backend.RosterPlayer{}
	for _, r := range m.rosterRows() {
		if !r.IsNumeric("ID") {
			continue
		}
		if p, ok := byID[int(r.Number("ID"))]; ok {
			rows = append(rows, p)
		}
	}
	return rows
}
func (m *teams) applySort(direction string) {
	id := 0
	if p, ok := m.SelectedPlayer(); ok {
		id = p.ID
	}
	state := m.order()
	state.apply(m.columns(), direction)
	m.orders[m.tab] = state
	m.offset = 0
	if m.tab == 0 || m.tab == 3 {
		for i, p := range m.players() {
			if p.ID == id {
				m.selected = i
				break
			}
		}
		m.offset = max(0, m.selected-m.pageSize()+1)
	}
}
func (m teams) SelectedPlayer() (backend.RosterPlayer, bool) {
	if m.loading || m.browsing {
		return backend.RosterPlayer{}, false
	}
	if m.tab == 0 || m.tab == 3 {
		rows := m.players()
		if m.selected >= 0 && m.selected < len(rows) {
			return rows[m.selected], true
		}
	}
	return backend.RosterPlayer{}, false
}
func (m teams) Update(msg tea.Msg) (teams, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
	case teamLoadedMsg:
		if v.id == m.data.Team.ID && v.request == m.request {
			m.loading = false
			m.data = v.data
		}
	case tea.KeyPressMsg:
		k := v.String()
		if m.browsing {
			if m.filtering {
				switch k {
				case "enter", "esc":
					m.filtering = false
				case "backspace":
					r := []rune(m.filter)
					if len(r) > 0 {
						m.filter = string(r[:len(r)-1])
					}
				default:
					if v.Text != "" {
						m.filter += v.Text
					}
				}
				m.choice = 0
				return m, nil
			}
			switch k {
			case "/":
				m.filtering = true
			case "esc":
				m.filter = ""
				m.choice = 0
			case "up", "k":
				m.choice--
			case "down", "j":
				m.choice++
			case "pgup":
				m.choice -= 10
			case "pgdown":
				m.choice += 10
			case "enter":
				rows := m.choices()
				if len(rows) > 0 {
					return m.open(rows[m.choice])
				}
			}
			m.choice = max(0, min(m.choice, len(m.choices())-1))
			return m, nil
		}
		if m.sortMode {
			cols := m.columns()
			state := m.order()
			switch k {
			case "esc", "q", "s":
				m.sortMode = false
			case "left", "h":
				state.cursor = (state.cursor + len(cols) - 1) % len(cols)
				m.orders[m.tab] = state
			case "right", "l":
				state.cursor = (state.cursor + 1) % len(cols)
				m.orders[m.tab] = state
			case "enter", "up", "down":
				m.applySort(k)
			}
			state = m.order()
			m.columnOffsets[m.tab] = revealColumn(cols, contentWidth(m.width)-6, m.columnOffsets[m.tab], state.cursor)
			return m.clampViewport(), nil
		}
		switch k {
		case "s":
			if !m.loading && (m.tab == 0 || m.tab == 3 || m.tab == 4) {
				m.sortMode = true
				state := m.order()
				state.cursor = state.cursorFor(m.columns())
				m.orders[m.tab] = state
			}
		case "[":
			m.columnOffsets[m.tab] = max(0, m.columnOffsets[m.tab]-1)
		case "]":
			m.columnOffsets[m.tab] = min(max(0, len(m.columns())-2), m.columnOffsets[m.tab]+1)
		case "tab", "right", "l":
			m.tab = (m.tab + 1) % len(teamTabs)
			m.selected, m.offset = 0, 0
		case "shift+tab", "left", "h":
			m.tab = (m.tab + len(teamTabs) - 1) % len(teamTabs)
			m.selected, m.offset = 0, 0
		case "1", "2", "3", "4", "5":
			m.tab = int(k[0] - '1')
			m.selected, m.offset = 0, 0
		case "r":
			if !m.loading {
				m.loading = true
				m.request++
				return m, m.fetch()
			}
		case "up", "k":
			if m.tab == 0 || m.tab == 3 {
				m.selected--
			}
			m.offset--
		case "down", "j":
			if m.tab == 0 || m.tab == 3 {
				m.selected++
			}
			m.offset++
		case "pgup":
			if m.tab == 0 || m.tab == 3 {
				m.selected -= m.pageSize()
			}
			m.offset -= m.pageSize()
		case "pgdown":
			if m.tab == 0 || m.tab == 3 {
				m.selected += m.pageSize()
			}
			m.offset += m.pageSize()
		case "home":
			m.selected, m.offset = 0, 0
		case "end":
			m.selected = 1 << 20
			m.offset = 1 << 20
		}
	}
	return m.clampViewport(), nil
}
func (m teams) clampViewport() teams {
	limit := m.rowCount()
	if limit == 0 {
		m.selected = -1
		m.offset = 0
		return m
	}
	m.selected = max(0, min(m.selected, limit-1))
	m.offset = max(0, min(m.offset, max(0, limit-m.pageSize())))
	if m.tab == 0 || m.tab == 3 {
		if m.selected < m.offset {
			m.offset = m.selected
		}
		if m.selected >= m.offset+m.pageSize() {
			m.offset = m.selected - m.pageSize() + 1
		}
	}
	return m
}
func (m teams) pageSize() int {
	room := pageBodyHeight(m.width, m.height, m.subtitle(), m.hints()) - 2
	if m.tab == 0 || m.tab == 3 {
		room -= 5 + len(m.rosterIntro()) + m.rosterFooterHeight()
	}
	if m.tab == 4 {
		room -= 5
	}
	return max(1, room)
}
func (m teams) rosterFooterHeight() int {
	if m.height < 30 {
		return 2
	}
	return 3
}
func (m teams) rosterIntro() []string {
	lines := []string{}
	if m.tab == 3 {
		var total int64
		known := 0
		for _, p := range m.data.Roster {
			if p.Salary != nil {
				total += *p.Salary
				known++
			}
		}
		lines = append(lines, fmt.Sprintf("ESPN · %s · USD · coverage %d/%d", m.data.Season, known, len(m.data.Roster)), fmt.Sprintf("Reported roster total: %s", money(reportedTotal(total, known))), mutedSty.Render("Excludes dead money, cap holds and tax."))
		if e := m.sectionError("salaries"); e != "" {
			lines = append(lines, e)
		}
	} else if e := m.sectionError("stats"); e != "" {
		lines = append(lines, e)
	}
	return lines
}
func (m teams) subtitle() string {
	tabs := []string{}
	for i, t := range teamTabs {
		label := fmt.Sprintf("%d %s", i+1, t)
		if i == m.tab {
			label = accentSty.Render("[" + label + "]")
		}
		tabs = append(tabs, label)
	}
	return strings.Join(tabs, "  ")
}
func (m teams) rowCount() int {
	switch m.tab {
	case 0, 3:
		return len(m.data.Roster)
	case 2:
		return len(m.injuryLines())
	case 4:
		return len(m.data.Games)
	default:
		return len(m.overviewLines())
	}
}
func (m teams) hints() string {
	if m.sortMode {
		return renderHints(contentWidth(m.width), [2]string{"←/→", "column"}, [2]string{"↑/↓", "asc/desc"}, [2]string{"enter", "toggle"}, [2]string{"esc", "done"})
	}
	if m.browsing {
		if m.filter != "" || m.filtering {
			return renderHints(contentWidth(m.width), [2]string{"↑/↓", "team"}, [2]string{"enter", "open"}, [2]string{"/", "search"}, [2]string{"esc", "clear"}, [2]string{"q", "back"})
		}
		return renderHints(contentWidth(m.width), [2]string{"↑/↓", "team"}, [2]string{"enter", "open"}, [2]string{"/", "search"}, [2]string{"esc/q", "back"})
	}
	pairs := [][2]string{{"←/→", "section"}, {"1–5", "jump"}, {"↑/↓", "move"}, {"pgup/pgdn", "page"}}
	if m.tab == 0 || m.tab == 3 {
		pairs = append(pairs, [2]string{"enter", "player"})
	}
	if m.tab == 0 || m.tab == 3 || m.tab == 4 {
		pairs = append(pairs, [2]string{"s", "sort"}, [2]string{"[/]", "columns"})
	}
	pairs = append(pairs, [2]string{"a", "schedule"}, [2]string{"r", "refresh"}, [2]string{"t", "teams"}, [2]string{"esc/q", "back"})
	return renderHints(contentWidth(m.width), pairs...)
}
func (m teams) View() tea.View {
	if m.browsing {
		rows := m.choices()
		lines := []string{mutedSty.Render("Choose a team to explore its roster, stats, injuries and salaries."), ""}
		if m.filter != "" || m.filtering {
			lines = append(lines, "Search: "+m.filter+"▏", "")
		}
		count := max(1, m.height-11-lipgloss.Height(m.hints()))
		start := max(0, min(m.choice-count/2, len(rows)-count))
		for i := start; i < min(len(rows), start+count); i++ {
			row := fmt.Sprintf("%-3s  %s", rows[i].Code, rows[i].Name)
			lines = append(lines, selectedRow(row, i == m.choice))
		}
		if len(rows) == 0 {
			lines = append(lines, "No teams match your search.")
		}
		lines = append(lines, "", mutedSty.Render(fmt.Sprintf("%d teams · / to search", len(rows))))
		return pageView(m.width, m.height, "NBA Teams", "", strings.Join(lines, "\n"), m.hints())
	}
	subtitle := m.subtitle()
	var body string
	if m.loading {
		body = "Loading team data…"
	} else {
		lines := []string{mutedSty.Render(m.data.Season + " · updated " + formatTeamUpdated(m.data.Updated)), ""}
		switch m.tab {
		case 0, 3:
			lines = append(lines, m.rosterLines()...)
		case 1:
			lines = append(lines, windowLines(m.overviewLines(), m.offset, m.pageSize())...)
		case 2:
			lines = append(lines, windowLines(m.injuryLines(), m.offset, m.pageSize())...)
		case 4:
			lines = append(lines, m.gameLines()...)
		}
		body = strings.Join(lines, "\n")
	}
	return pageView(m.width, m.height, m.data.Team.Name, subtitle, body, m.hints())
}
func selectedRow(row string, selected bool) string {
	if selected {
		return accentSty.Render("▶ " + row)
	}
	return "  " + row
}
func windowLines[T any](lines []T, start, count int) []T {
	start = max(0, min(start, max(0, len(lines)-count)))
	return lines[start:min(len(lines), start+count)]
}
func unavailable(r backend.Row, key string, precision int) string {
	if !r.Has(key) {
		return "—"
	}
	if precision < 0 {
		return r.Text(key)
	}
	return fmt.Sprintf("%.*f", precision, r.Number(key))
}
func money(s *int64) string {
	if s == nil {
		return "Unavailable"
	}
	digits := fmt.Sprint(*s)
	for i := len(digits) - 3; i > 0; i -= 3 {
		digits = digits[:i] + "," + digits[i:]
	}
	return "$" + digits
}
func formatTeamUpdated(t time.Time) string {
	lt := t.Local()
	abbrev, _ := lt.Zone()
	return lt.Format("3:04 PM") + " " + abbrev
}

func (m teams) sectionError(section string) string {
	if e := m.data.Errors[section]; e != "" {
		return "Unavailable: " + e + " · r to retry"
	}
	return ""
}
func (m teams) rosterLines() []string {
	lines := m.rosterIntro()
	if e := m.sectionError("roster"); e != "" {
		return append(lines, e)
	}
	rows := m.rosterRows()
	title := "ROSTER · SEASON AVERAGES"
	if m.tab == 3 {
		title = "SALARIES · HIGHEST REPORTED FIRST"
		if m.orders[3].key != "" {
			title = "ROSTER SALARIES"
		}
	}
	table := renderDataTable(title, m.columns(), rows, dataTableOptions{width: contentWidth(m.width), offset: m.offset, limit: m.pageSize(), selected: m.selected, columnOffset: m.columnOffsets[m.tab], sorting: m.sortMode, order: m.order(), empty: "No roster published for this season."})
	lines = append(lines, strings.Split(table, "\n")...)
	players := m.players()
	if len(players) == 0 {
		return lines
	}
	p := players[clampInt(m.selected, 0, len(players)-1)]
	lines = append(lines, "")
	if m.rosterFooterHeight() == 3 {
		lines = append(lines, mutedSty.Render(fmt.Sprintf("%s · %s · %s lb · %s years · %s", p.Name, p.Height, p.Weight, p.Experience, p.School)))
	}
	lines = append(lines, mutedSty.Render(fmt.Sprintf("Player %d/%d · enter for profile", m.selected+1, len(players))))
	return lines
}
func appendTable(lines []string, table string) []string {
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	return append(lines, strings.Split(table, "\n")...)
}
func (m teams) overviewLines() []string {
	d := m.data
	w := contentWidth(m.width)
	lines := []string{}
	if e := m.sectionError("overview"); e != "" {
		lines = append(lines, e)
	} else {
		fields := [][2]string{{"Conference", d.Info.Text("TEAM_CONFERENCE")}, {"Division", d.Info.Text("TEAM_DIVISION")}, {"Record", unavailable(d.Info, "W", 0) + "–" + unavailable(d.Info, "L", 0)}, {"Win percentage", unavailable(d.Info, "PCT", 3)}, {"Conference rank", unavailable(d.Info, "CONF_RANK", 0)}, {"Division rank", unavailable(d.Info, "DIV_RANK", 0)}, {"Franchise seasons", d.Info.Text("MIN_YEAR") + "–" + d.Info.Text("MAX_YEAR")}}
		lines = append(lines, infoTable("SEASON SNAPSHOT", fields, w)...)
		rows := []backend.Row{}
		for _, f := range [][3]string{{"Points / game", "PTS_PG", "PTS_RANK"}, {"Opponent points / game", "OPP_PTS_PG", "OPP_PTS_RANK"}, {"Rebounds / game", "REB_PG", "REB_RANK"}, {"Assists / game", "AST_PG", "AST_RANK"}} {
			r := dataRow(map[string]any{"STAT": f[0]})
			if d.Ranks.Has(f[1]) {
				r["VALUE"] = d.Ranks[f[1]]
			}
			if d.Ranks.Has(f[2]) {
				r["RANK"] = d.Ranks[f[2]]
			}
			rows = append(rows, r)
		}
		cols := []dataColumn{textColumn("STAT", "STAT", 29), numberColumn("VALUE", "PER GAME", 12, 1), numberColumn("RANK", "NBA RANK", 12, 0)}
		lines = appendTable(lines, renderDataTable("LEAGUE RANKS", cols, rows, dataTableOptions{width: w, selected: -1}))
	}
	if len(d.Totals) > 0 {
		lines = append(lines, "")
		lines = append(lines, statSummary(d.Totals, w, "TEAM AVERAGES")...)
	}
	if e := m.sectionError("franchise"); e != "" {
		lines = append(lines, "", e)
	}
	if rows := d.Franchise["TeamBackground"]; len(rows) > 0 {
		fields := [][2]string{}
		for _, f := range [][2]string{{"Founded", "YEARFOUNDED"}, {"Arena", "ARENA"}, {"Capacity", "ARENACAPACITY"}, {"Owner", "OWNER"}, {"General manager", "GENERALMANAGER"}, {"Head coach", "HEADCOACH"}, {"G League affiliate", "DLEAGUEAFFILIATION"}} {
			fields = append(fields, [2]string{f[0], rows[0].Text(f[1])})
		}
		lines = append(lines, "")
		lines = append(lines, infoTable("FRANCHISE", fields, w)...)
	}
	titles := [][2]string{}
	for _, s := range [][2]string{{"Championships", "TeamAwardsChampionships"}, {"Conference titles", "TeamAwardsConf"}, {"Division titles", "TeamAwardsDiv"}, {"NBA Cup titles", "TeamAwardsCommCup"}} {
		if rows := d.Franchise[s[1]]; rows != nil {
			years := []string{}
			for _, r := range rows {
				years = append(years, r.Text("YEARAWARDED"))
			}
			value := "None listed"
			if len(years) > 0 {
				value = fmt.Sprintf("%d · %s", len(rows), strings.Join(years, ", "))
			}
			titles = append(titles, [2]string{s[0], value})
		}
	}
	if len(titles) > 0 {
		lines = append(lines, "")
		lines = append(lines, infoTable("TITLES & HONORS", titles, w)...)
	}
	for _, s := range [][2]string{{"RETIRED NUMBERS", "TeamRetired"}, {"HALL OF FAME", "TeamHof"}} {
		if rows := d.Franchise[s[1]]; len(rows) > 0 {
			cols := []dataColumn{textColumn("PLAYER", "PLAYER", 25), textColumn("JERSEY", "#", 5), textColumn("POSITION", "POS", 5), textColumn("SEASONSWITHTEAM", "SEASONS", 15), numberColumn("YEAR", "YEAR", 6, 0)}
			lines = appendTable(lines, renderDataTable(s[0], cols, rows, dataTableOptions{width: w, selected: -1}))
		}
	}
	coaches := []backend.Row{}
	for _, c := range d.Coaches {
		role := "Assistant"
		if c.Number("IS_ASSISTANT") == 0 {
			role = "Head coach"
		}
		coaches = append(coaches, dataRow(map[string]any{"NAME": c.Text("COACH_NAME"), "ROLE": role}))
	}
	lines = appendTable(lines, renderDataTable("COACHING STAFF", []dataColumn{textColumn("NAME", "COACH", 28), textColumn("ROLE", "ROLE", 22)}, coaches, dataTableOptions{width: w, selected: -1}))
	moves := [][2]string{}
	for _, p := range d.Roster {
		if p.Acquired != "" {
			moves = append(moves, [2]string{p.Name, p.Acquired})
		}
	}
	if len(moves) > 0 {
		lines = append(lines, "")
		lines = append(lines, infoTable("ROSTER MOVES", moves, w)...)
	}
	return lines
}
func (m teams) injuryLines() []string {
	if e := m.sectionError("injuries"); e != "" {
		return []string{e}
	}
	rows := []backend.Row{}
	notes := [][2]string{}
	for _, i := range m.data.Injuries {
		rows = append(rows, dataRow(map[string]any{"NAME": i.Name, "STATUS": i.Status, "INJURY": i.Kind, "RETURN": i.ReturnDate}))
		note := "Updated: " + i.Date
		if i.Comment != "" {
			note += "\n" + i.Comment
		}
		notes = append(notes, [2]string{i.Name, note})
	}
	cols := []dataColumn{textColumn("NAME", "PLAYER", 24), textColumn("STATUS", "STATUS", 12), textColumn("INJURY", "INJURY", 13), textColumn("RETURN", "EST. RETURN", 13)}
	lines := strings.Split(renderDataTable("INJURY REPORT · ESPN", cols, rows, dataTableOptions{width: contentWidth(m.width), selected: -1, wrap: true, empty: "No injuries listed by the provider."}), "\n")
	if len(notes) > 0 {
		lines = append(lines, "")
		lines = append(lines, infoTable("REPORT NOTES", notes, contentWidth(m.width))...)
	}
	lines = append(lines, "", mutedSty.Render("Estimated returns can change before tip-off."), "Official NBA game-status reports:", fmt.Sprintf("https://official.nba.com/nba-injury-report-%s-season/", m.data.Season))
	return lines
}
func (m teams) gameLines() []string {
	if e := m.sectionError("games"); e != "" {
		return []string{e}
	}
	rows := orderedRows(m.data.Games, m.columns(), m.order())
	return strings.Split(renderDataTable("REGULAR SEASON · RESULTS", m.columns(), rows, dataTableOptions{width: contentWidth(m.width), offset: m.offset, limit: m.pageSize(), selected: -1, columnOffset: m.columnOffsets[m.tab], sorting: m.sortMode, order: m.order(), empty: "No completed regular-season games yet."}), "\n")
}

func reportedTotal(total int64, known int) *int64 {
	if known == 0 {
		return nil
	}
	return &total
}
