package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/NolanFogarty/courtside/backend"
)

type schedule struct {
	games                 []backend.SeasonGame
	filter                string
	filtering             bool
	offset, width, height int
	selected              int
	columnOffset          int
	loading               bool
	err                   error
	touched               bool // user has navigated; late loads must not reposition
}
type scheduleLoaded struct {
	games []backend.SeasonGame
	err   error
}

func newSchedule(w, h int) schedule { return schedule{width: w, height: h, loading: true} }
func (m schedule) Init() tea.Cmd {
	return func() tea.Msg { g, e := backend.GetSeasonSchedule(); return scheduleLoaded{g, e} }
}
func (m schedule) Update(msg tea.Msg) (schedule, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
	case scheduleLoaded:
		m.loading = false
		m.err = v.err
		m.games = v.games
		// A late load must not yank a user who already navigated back to
		// today; only auto-position on a fresh, untouched view.
		if !m.touched {
			today := time.Now().Format("2006-01-02")
			found := false
			for i, g := range m.filteredGames() {
				if scheduleLocalDate(g) >= today {
					m.offset = i
					m.selected = i
					found = true
					break
				}
			}
			if !found && len(m.filteredGames()) > 0 {
				// Every game is in the past (e.g. end of season): land on
				// the most recent one instead of the top of the list.
				m.selected = len(m.filteredGames()) - 1
				m.offset = max(0, len(m.filteredGames())-m.pageSize())
			}
		}
	case tea.KeyPressMsg:
		k := v.String()
		m.touched = true
		if m.filtering {
			switch k {
			case "enter":
				m.filtering = false
			case "esc":
				m.filtering = false
				m.filter = ""
			case "backspace":
				if len(m.filter) > 0 {
					m.filter = m.filter[:len(m.filter)-1]
				}
			default:
				if len(k) == 1 && k[0] >= 32 && k[0] < 127 {
					m.filter += k
				}
			}
			m.selected, m.offset = 0, 0
			break
		}
		switch k {
		case "[":
			m.columnOffset = max(0, m.columnOffset-1)
		case "]":
			m.columnOffset++
		case "/":
			m.filtering = true
		case "up", "k":
			m.selected--
		case "down", "j":
			m.selected++
		case "pgup":
			m.selected -= 15
		case "pgdown":
			m.selected += 15
		case "ctrl+t":
			m.jumpToToday()
		case "esc":
			m.filter = ""
		}
	}
	rows := m.filteredGames()
	count := len(rows)
	if count == 0 {
		m.selected, m.offset = 0, 0
		return m, nil
	}
	m.selected = max(0, min(m.selected, count-1))
	m.offset = max(0, min(m.offset, count-1))
	if m.selected < m.offset {
		m.offset = m.selected
	}
	for m.selected >= m.pageEnd(rows, m.offset) {
		m.offset++
	}
	return m, nil
}

func (m schedule) SelectedGame() (backend.Game, bool) {
	rows := m.filteredGames()
	if m.selected < 0 || m.selected >= len(rows) {
		return backend.Game{}, false
	}
	return backend.GameFromSeasonSchedule(rows[m.selected]), true
}

func (m schedule) filteredGames() []backend.SeasonGame {
	rows := []backend.SeasonGame{}
	for _, g := range m.games {
		if m.filter == "" || strings.Contains(strings.ToLower(g.Away+" "+g.Home), strings.ToLower(m.filter)) {
			rows = append(rows, g)
		}
	}
	return rows
}

var liveSty = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))

func (m schedule) View() tea.View {
	title := "NBA Full Schedule"
	if m.filter != "" {
		title += " · Team: " + m.filter
	}
	if m.filtering {
		title += "  /"
	}
	lines := []string{}
	if m.loading {
		lines = append(lines, "Loading schedule…")
	} else if m.err != nil {
		lines = append(lines, errSty.Render(m.err.Error()))
	} else {
		rows := m.filteredGames()
		tableRows := []backend.Row{}
		separators := map[int]bool{}
		tableSelected := -1
		end := m.pageEnd(rows, m.offset)
		start := m.offset
		if start > len(rows) {
			start = len(rows)
		}
		sel := m.selected - start
		if len(rows) == 0 {
			sel = -1
		}
		for idx, g := range rows[start:end] {
			if idx > 0 && scheduleLocalDate(g) != scheduleLocalDate(rows[start+idx-1]) {
				separators[len(tableRows)] = true
				tableRows = append(tableRows, backend.Row{})
			}
			isSel := idx == sel
			if isSel {
				tableSelected = len(tableRows)
			}
			// Finals use readable green/red foregrounds on every row.
			// Styles always wrap plain strings directly: wrapping a styled
			// span buries the color under an inner reset.
			base := func(s string) string {
				if isSel {
					return accentSty.Render(s)
				}
				return s
			}
			date := base(scheduleLocalDate(g))
			awayText := fmt.Sprintf("%s (%d-%d)", g.Away, g.AwayWins, g.AwayLosses)
			homeText := fmt.Sprintf("%s (%d-%d)", g.Home, g.HomeWins, g.HomeLosses)
			away := awayText
			home := homeText
			final := isFinalScheduleGame(g)
			live := isLiveScheduleGame(g)
			score := g.Status
			if final {
				score = fmt.Sprintf("%3d - %-3d", g.AwayScore, g.HomeScore)
			} else if !live {
				if start, err := time.Parse(time.RFC3339, g.DateTimeUTC); err == nil {
					score = formatScheduleTip(start)
				} else if score == "" {
					score = g.Time
				}
			}
			if !final && !live && strings.TrimSpace(score) == "" {
				// No tip-off text published yet: show TBD, never a bare
				// "0 - 0" or a blank cell.
				score = "TBD"
			}
			awayScore := fmt.Sprintf("%3d", g.AwayScore)
			homeScore := fmt.Sprintf("%-3d", g.HomeScore)
			switch {
			case final && g.AwayScore > g.HomeScore:
				awayScore = winTextSty.Render(awayScore)
				homeScore = lossTextSty.Render(homeScore)
				away, home = winTextSty.Render(away), lossTextSty.Render(home)
				score = awayScore + base(" - ") + homeScore
			case final && g.HomeScore > g.AwayScore:
				awayScore = lossTextSty.Render(awayScore)
				homeScore = winTextSty.Render(homeScore)
				away, home = lossTextSty.Render(away), winTextSty.Render(home)
				score = awayScore + base(" - ") + homeScore
			case final:
				score, away, home = base(score), base(away), base(home)
			case live:
				score = liveSty.Render(fmt.Sprintf("%d - %d  %s", g.AwayScore, g.HomeScore, g.Status))
				away, home = base(away), base(home)
			default:
				score, away, home = base(score), base(away), base(home)
			}
			tableRows = append(tableRows, dataRow(map[string]any{"DATE": date, "AWAY": away, "HOME": home, "SCORE": score}))
		}
		tableTitle := "SEASON SCHEDULE"
		if len(rows) > 0 {
			tableTitle += fmt.Sprintf(" · %d–%d / %d", start+1, end, len(rows))
		}
		cols := []dataColumn{textColumn("DATE", "DATE", 10), textColumn("AWAY", "AWAY · RECORD", 28), textColumn("HOME", "HOME · RECORD", 28), textColumn("SCORE", "SCORE / TIP-OFF", 21)}
		lines = strings.Split(renderDataTable(tableTitle, cols, tableRows, dataTableOptions{width: contentWidth(m.width), selected: tableSelected, separatorRows: separators, columnOffset: m.columnOffset, empty: "No games match this filter."}), "\n")
	}
	if m.filtering {
		lines = append(lines, "Type team name • enter apply • esc clear")
	}
	return pageView(m.width, m.height, title, "", strings.Join(lines, "\n"), m.hints())
}
func (m *schedule) jumpToToday() {
	today := time.Now().Format("2006-01-02")
	rows := m.filteredGames()
	for i, g := range rows {
		if scheduleLocalDate(g) >= today {
			m.offset = i
			m.selected = i
			return
		}
	}
	if len(rows) > 0 {
		m.selected = len(rows) - 1
		m.offset = max(0, len(rows)-m.pageSize())
	}
}

func (m schedule) hints() string {
	if m.filtering {
		return renderHints(contentWidth(m.width), [2]string{"enter", "apply"}, [2]string{"esc", "clear"})
	}
	back := [2]string{"q", "back"}
	if m.filter != "" {
		back = [2]string{"esc", "clear filter"}
	}
	return renderHints(contentWidth(m.width), [2]string{"/", "filter"}, [2]string{"ctrl+t", "today"}, [2]string{"↑/↓", "move"}, [2]string{"pgup/pgdn", "page"}, [2]string{"[/]", "columns"}, [2]string{"enter", "box score"}, [2]string{"t", "teams"}, back)
}
func (m schedule) pageSize() int {
	room := pageBodyHeight(m.width, m.height, "", m.hints()) - 5
	if m.filtering {
		room--
	}
	return max(1, room)
}

// pageEnd limits rendered table rows, including day separators, to the same
// body-height budget used by pageSize. The offset and selection remain game indices.
func (m schedule) pageEnd(rows []backend.SeasonGame, start int) int {
	start = clampInt(start, 0, len(rows))
	budget := m.pageSize()
	used := 0
	end := start
	for end < len(rows) {
		cost := 1
		if end > start && scheduleLocalDate(rows[end]) != scheduleLocalDate(rows[end-1]) {
			cost++
		}
		if used+cost > budget {
			break
		}
		used += cost
		end++
	}
	return end
}

func isLiveScheduleGame(g backend.SeasonGame) bool {
	if g.GameStatus == 2 {
		return true
	}
	s := strings.ToLower(g.Status)
	for _, token := range []string{"q1", "q2", "q3", "q4", "ot", "live", "half", "halftime", "end of"} {
		if strings.Contains(s, token) {
			return true
		}
	}
	return false
}

func isFinalScheduleGame(g backend.SeasonGame) bool {
	if g.GameStatus == 3 {
		return true
	}
	return strings.Contains(strings.ToLower(g.Status), "final")
}

func isLiveScheduleStatus(status string) bool {
	s := strings.ToLower(status)
	for _, token := range []string{"q1", "q2", "q3", "q4", "ot", "live", "half", "halftime", "end of"} {
		if strings.Contains(s, token) {
			return true
		}
	}
	return false
}

func isFinalScheduleStatus(status string) bool {
	return strings.Contains(strings.ToLower(status), "final")
}

func formatScheduleTip(t time.Time) string {
	lt := t.Local()
	abbrev, _ := lt.Zone()
	return lt.Format("3:04 PM") + " " + abbrev
}

func scheduleLocalDate(g backend.SeasonGame) string {
	if t, err := time.Parse(time.RFC3339, g.DateTimeUTC); err == nil {
		return t.Local().Format("2006-01-02")
	}
	if len(g.Date) >= 10 {
		return g.Date[:10]
	}
	return g.Date
}

func padColored(s string, width int) string {
	gap := width - lipgloss.Width(s)
	if gap < 0 {
		gap = 0
	}
	return s + strings.Repeat(" ", gap)
}
