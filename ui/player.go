package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/NolanFogarty/courtside/backend"
)

var playerTabs = []string{"Profile", "Career", "Playoffs", "Game log"}

type player struct {
	width, height        int
	seed                 backend.RosterPlayer
	team                 backend.Team
	data                 backend.PlayerProfile
	loading, shooting    bool
	sortMode             bool
	orders               [4]columnSort
	columnOffsets        [4]int
	tab, offset, request int
}
type playerLoadedMsg struct {
	id, request int
	data        backend.PlayerProfile
}

func newPlayer(p backend.RosterPlayer, t backend.Team, w, h, request int) player {
	return player{seed: p, team: t, width: w, height: h, loading: true, request: request}
}
func (m player) Init() tea.Cmd {
	id, request := m.seed.ID, m.request
	return func() tea.Msg { return playerLoadedMsg{id, request, backend.GetPlayerProfile(id)} }
}
func (m player) Update(msg tea.Msg) (player, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
	case playerLoadedMsg:
		if v.id == m.seed.ID && v.request == m.request {
			m.loading = false
			m.data = v.data
		}
	case tea.KeyPressMsg:
		if m.sortMode {
			cols := m.columns()
			state := m.order()
			switch v.String() {
			case "esc", "q", "s":
				m.sortMode = false
			case "left", "h":
				state.cursor = (state.cursor + len(cols) - 1) % len(cols)
			case "right", "l":
				state.cursor = (state.cursor + 1) % len(cols)
			case "enter", "up", "down":
				state.apply(cols, v.String())
				m.offset = 0
			}
			m.orders[m.tab] = state
			m.columnOffsets[m.tab] = revealColumn(cols, contentWidth(m.width)-4, m.columnOffsets[m.tab], state.cursor)
			m.offset = max(0, min(m.offset, max(0, m.rowCount()-m.pageSize())))
			return m, nil
		}
		switch v.String() {
		case "tab", "right", "l":
			m.tab = (m.tab + 1) % len(playerTabs)
			m.offset = 0
		case "shift+tab", "left", "h":
			m.tab = (m.tab + len(playerTabs) - 1) % len(playerTabs)
			m.offset = 0
		case "1", "2", "3", "4":
			m.tab = int(v.String()[0] - '1')
			m.offset = 0
		case "s":
			if !m.loading && m.tab > 0 && m.data.Errors[m.section()] == "" {
				m.sortMode = true
				state := m.order()
				state.cursor = state.cursorFor(m.columns())
				m.orders[m.tab] = state
			}
		case "[":
			m.columnOffsets[m.tab] = max(0, m.columnOffsets[m.tab]-1)
		case "]":
			m.columnOffsets[m.tab] = min(max(0, len(m.columns())-2), m.columnOffsets[m.tab]+1)
		case "o":
			m.shooting = !m.shooting
		case "up", "k":
			m.offset--
		case "down", "j":
			m.offset++
		case "pgup":
			m.offset -= m.pageSize()
		case "pgdown":
			m.offset += m.pageSize()
		case "home":
			m.offset = 0
		case "end":
			m.offset = 1 << 20
		case "r":
			if !m.loading {
				m.loading = true
				m.request++
				return m, m.Init()
			}
		}
	}
	m.offset = max(0, min(m.offset, max(0, m.rowCount()-m.pageSize())))
	return m, nil
}
func (m player) hints() string {
	if m.sortMode {
		return renderHints(contentWidth(m.width), [2]string{"←/→", "column"}, [2]string{"↑/↓", "asc/desc"}, [2]string{"enter", "toggle"}, [2]string{"esc", "done"})
	}
	pairs := [][2]string{{"←/→", "section"}, {"↑/↓", "scroll"}, {"pgup/pgdn", "page"}}
	if m.tab > 0 {
		pairs = append(pairs, [2]string{"s", "sort"}, [2]string{"o", "stats"}, [2]string{"[/]", "columns"})
	}
	pairs = append(pairs, [2]string{"r", "refresh"}, [2]string{"t", "teams"}, [2]string{"esc/q", "back"})
	return renderHints(contentWidth(m.width), pairs...)
}
func (m player) pageSize() int {
	room := pageBodyHeight(m.width, m.height, m.subtitle(), m.hints())
	if m.tab > 0 {
		room -= 7
	}
	return max(1, room)
}
func (m player) rowCount() int {
	if m.tab == 0 {
		return len(m.bioLines())
	}
	return len(m.rows())
}
func (m player) section() string {
	if m.tab == 3 {
		return "games"
	}
	return "career"
}
func (m player) order() columnSort {
	s := m.orders[m.tab]
	if s.key == "" {
		s.key = "SEASON_ID"
		s.desc = true
		if m.tab == 3 {
			s.key = "GAME_DATE"
		}
	}
	return s
}
func (m player) columns() []dataColumn {
	cols := []dataColumn{textColumn("SEASON_ID", "SEASON", 9), textColumn("TEAM_ABBREVIATION", "TEAM", 6), numberColumn("GP", "GP", 4, 0)}
	if m.tab == 3 {
		cols = []dataColumn{dateColumn("GAME_DATE", "DATE", 13), textColumn("MATCHUP", "MATCHUP", 13), textColumn("WL", "W/L", 5)}
	}
	if m.shooting {
		return append(cols, shootingColumns()...)
	}
	return append(cols, countingColumns()...)
}
func countingColumns() []dataColumn {
	cols := []dataColumn{}
	for _, k := range []string{"MIN", "PTS", "REB", "AST", "STL", "BLK", "TOV"} {
		cols = append(cols, numberColumn(k, k, 5, 1))
	}
	return cols
}
func shootingColumns() []dataColumn {
	return []dataColumn{numberColumn("FGM", "FGM", 5, 1), numberColumn("FGA", "FGA", 5, 1), percentColumn("FG_PCT", "FG%"), numberColumn("FG3M", "3PM", 5, 1), numberColumn("FG3A", "3PA", 5, 1), percentColumn("FG3_PCT", "3P%"), numberColumn("FTM", "FTM", 5, 1), numberColumn("FTA", "FTA", 5, 1), percentColumn("FT_PCT", "FT%")}
}
func (m player) rows() []backend.Row {
	rows := m.data.Seasons
	if m.tab == 2 {
		rows = m.data.Playoffs
	} else if m.tab == 3 {
		rows = m.data.Games
	}
	cols := append(append(m.columns(), countingColumns()...), shootingColumns()...)
	return orderedRows(rows, cols, m.order())
}
func (m player) subtitle() string {
	tabs := []string{}
	for i, t := range playerTabs {
		label := fmt.Sprintf("%d %s", i+1, t)
		if i == m.tab {
			label = accentSty.Render("[" + label + "]")
		}
		tabs = append(tabs, label)
	}
	return strings.Join(tabs, "  ")
}
func (m player) View() tea.View {
	lines := []string{"Loading player data…"}
	if !m.loading {
		lines = m.lines()
		if m.tab == 0 {
			lines = windowLines(lines, m.offset, m.pageSize())
		}
	}
	return pageView(m.width, m.height, m.seed.Name+" · "+m.team.Code, m.subtitle(), strings.Join(lines, "\n"), m.hints())
}
func (m player) lines() []string {
	if m.tab == 0 {
		return m.bioLines()
	}
	if e := m.data.Errors[m.section()]; e != "" {
		return []string{"Unavailable: " + e + " · r to retry"}
	}
	title := []string{"", "REGULAR SEASON · PER GAME", "PLAYOFFS · PER GAME", "GAME LOG"}[m.tab]
	body := renderDataTable(title, m.columns(), m.rows(), dataTableOptions{width: contentWidth(m.width), offset: m.offset, limit: m.pageSize(), selected: -1, columnOffset: m.columnOffsets[m.tab], sorting: m.sortMode, order: m.order(), empty: "No statistics published for this section."})
	return append([]string{mutedSty.Render(m.data.Season + " · NBA"), ""}, strings.Split(body, "\n")...)
}
func statSummary(r backend.Row, width int, title string) []string {
	keys := []string{"GP", "GS", "MIN", "PTS", "REB", "AST", "STL", "BLK", "TOV", "FG_PCT", "FG3_PCT", "FT_PCT"}
	cols := []dataColumn{}
	for i := range 3 {
		value := textColumn(fmt.Sprintf("V%d", i), "VALUE", 8)
		value.right = true
		cols = append(cols, textColumn(fmt.Sprintf("K%d", i), "STAT", 8), value)
	}
	rows := []backend.Row{}
	for i := 0; i < len(keys); i += 3 {
		values := map[string]any{}
		for j := range 3 {
			k := keys[i+j]
			value := unavailable(r, k, 1)
			label := k
			if k == "GP" || k == "GS" {
				value = unavailable(r, k, 0)
			}
			if strings.HasSuffix(k, "PCT") {
				label = map[string]string{"FG_PCT": "FG%", "FG3_PCT": "3P%", "FT_PCT": "FT%"}[k]
				if r.Has(k) {
					value = fmt.Sprintf("%.1f%%", 100*r.Number(k))
				}
			}
			values[fmt.Sprintf("K%d", j)], values[fmt.Sprintf("V%d", j)] = label, value
		}
		rows = append(rows, dataRow(values))
	}
	return strings.Split(renderDataTable(title, cols, rows, dataTableOptions{width: width, selected: -1}), "\n")
}
func (m player) bioLines() []string {
	p, r := m.seed, m.data.Info
	lines := []string{}
	if e := m.data.Errors["bio"]; e != "" {
		lines = append(lines, "Bio unavailable: "+e)
	}
	fields := [][2]string{{"Team", strings.TrimSpace(r.Text("TEAM_CITY") + " " + r.Text("TEAM_NAME"))}, {"Position", r.Text("POSITION")}, {"Jersey", r.Text("JERSEY")}, {"Height", r.Text("HEIGHT")}, {"Weight (lb)", r.Text("WEIGHT")}, {"Birth date", r.Text("BIRTHDATE")}, {"Country", r.Text("COUNTRY")}, {"School", r.Text("SCHOOL")}, {"NBA experience", r.Text("SEASON_EXP")}, {"NBA seasons", seasonSpan(r)}, {"Roster status", r.Text("ROSTERSTATUS")}, {"Draft", draftInfo(r)}}
	if len(r) == 0 {
		fields = [][2]string{{"Team", m.team.Name}, {"Position", p.Position}, {"Jersey", p.Number}, {"Height", p.Height}, {"Weight (lb)", p.Weight}, {"Birth date", p.BirthDate}, {"School", p.School}, {"NBA experience", p.Experience}}
	}
	lines = append(lines, infoTable("BIO & DRAFT", fields, contentWidth(m.width))...)
	details := [][2]string{{"Acquired", p.Acquired}, {m.data.Season + " salary", money(p.Salary) + " · ESPN"}}
	if p.Injury != nil {
		details = append(details, [2]string{"Injury · ESPN", p.Injury.Status + " · " + p.Injury.Kind}, [2]string{"Updated", p.Injury.Date}, [2]string{"Estimated return", p.Injury.ReturnDate}, [2]string{"Report", p.Injury.Comment})
	}
	lines = append(lines, "")
	lines = append(lines, infoTable("CONTRACT & AVAILABILITY", details, contentWidth(m.width))...)
	lines = append(lines, "")
	if len(p.Stats) > 0 {
		lines = append(lines, statSummary(p.Stats, contentWidth(m.width), "CURRENT TEAM · SEASON AVERAGES")...)
	} else {
		lines = append(lines, "No season averages published.")
	}
	if len(m.data.Career) > 0 {
		lines = append(lines, "")
		lines = append(lines, statSummary(m.data.Career[0], contentWidth(m.width), "CAREER · REGULAR SEASON")...)
	}
	if len(m.data.Highs) > 0 {
		cols := []dataColumn{textColumn("STAT", "STAT", 12), numberColumn("STAT_VALUE", "HIGH", 8, 0), dateColumn("GAME_DATE", "DATE", 18)}
		lines = append(lines, "")
		lines = append(lines, strings.Split(renderDataTable("CAREER HIGHS", cols, m.data.Highs, dataTableOptions{width: contentWidth(m.width), selected: -1}), "\n")...)
	}
	return lines
}

func seasonSpan(r backend.Row) string {
	from, to := r.Text("FROM_YEAR"), r.Text("TO_YEAR")
	if from == "" {
		return to
	}
	if to == "" || from == to {
		return from
	}
	return from + "–" + to
}
func draftInfo(r backend.Row) string {
	year := r.Text("DRAFT_YEAR")
	if strings.EqualFold(year, "Undrafted") {
		return "Undrafted"
	}
	parts := []string{}
	if year != "" {
		parts = append(parts, year)
	}
	if round := r.Text("DRAFT_ROUND"); round != "" {
		parts = append(parts, "Round "+round)
	}
	if pick := r.Text("DRAFT_NUMBER"); pick != "" {
		parts = append(parts, "Pick "+pick)
	}
	return strings.Join(parts, " · ")
}
