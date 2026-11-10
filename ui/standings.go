package ui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/NolanFogarty/courtside/backend"
)

// ---- model ---------------------------------------------------------------

type standings struct {
	loading       bool
	err           error
	data          backend.Standings
	expanded      bool
	showWest      bool
	width, height int
}

func newStandings(width, height int) standings {
	return standings{loading: true, width: width, height: height}
}

// standingsLoadedMsg carries the result of the async GetStandings fetch.
type standingsLoadedMsg struct {
	data backend.Standings
	err  error
}

func (m standings) Init() tea.Cmd {
	return func() tea.Msg {
		d, err := backend.GetStandings()
		return standingsLoadedMsg{data: d, err: err}
	}
}

func (m standings) Update(msg tea.Msg) (standings, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case standingsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.data = msg.data
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "o":
			m.expanded = !m.expanded
		case "left", "h":
			m.showWest = false
		case "right", "l":
			m.showWest = true
		case "tab", "shift+tab":
			m.showWest = !m.showWest
		}
	}
	return m, nil
}

// ---- view ----------------------------------------------------------------

func (m standings) View() tea.View {
	hFrame, vFrame := docStyle.GetFrameSize()
	width := m.width - hFrame
	if width < 40 {
		width = 40
	}

	titleText := "NBA Standings"
	if m.data.Season != "" {
		titleText += " · " + m.data.Season
	}
	title := lipgloss.PlaceHorizontal(width, lipgloss.Center, headerBarSty.Render(titleText))

	var main string
	switch {
	case m.err != nil:
		main = lipgloss.PlaceHorizontal(width, lipgloss.Center,
			errSty.Render("Failed to load standings: "+m.err.Error()))
	case m.loading:
		main = lipgloss.PlaceHorizontal(width, lipgloss.Center,
			mutedSty.Render("Loading standings…"))
	default:
		if len(m.data.East) == 0 && len(m.data.West) == 0 {
			main = lipgloss.PlaceHorizontal(width, lipgloss.Center,
				mutedSty.Render("No standings published for this season yet."))
		} else if m.expanded {
			if m.showWest {
				main = renderConference("WESTERN", homeColor, m.data.West, true)
			} else {
				main = renderConference("EASTERN", awayColor, m.data.East, true)
			}
			main = lipgloss.PlaceHorizontal(width, lipgloss.Center, main)
		} else {
			east := renderConference("EASTERN", awayColor, m.data.East, false)
			west := renderConference("WESTERN", homeColor, m.data.West, false)
			tables := lipgloss.JoinHorizontal(lipgloss.Top, east, "  ", west)
			if lipgloss.Width(tables) > width {
				tables = east
				if m.showWest {
					tables = west
				}
			}
			main = lipgloss.PlaceHorizontal(width, lipgloss.Center, tables)
		}
	}

	body := lipgloss.NewStyle().MaxWidth(contentWidth(m.width)).Render(lipgloss.JoinVertical(lipgloss.Left, title, "", main))
	statsHint := "more stats"
	if m.expanded {
		statsHint = "less stats"
	}
	hintWidth := max(contentWidth(m.width), width)
	var hint string
	if m.expanded {
		conference := "EAST"
		if m.showWest {
			conference = "WEST"
		}
		hint = renderHints(hintWidth, [2]string{"tab/←→", "conference (" + conference + ")"}, [2]string{"o", statsHint}, [2]string{"t", "teams"}, [2]string{"q", "back"})
	} else {
		hint = renderHints(hintWidth, [2]string{"tab/←→", "conference"}, [2]string{"o", statsHint}, [2]string{"t", "teams"}, [2]string{"q", "back"})
	}

	if room := m.height - vFrame - lipgloss.Height(hint) - 1; room > 0 && lipgloss.Height(body) > room {
		body = lipgloss.NewStyle().MaxHeight(room).Render(body)
	}

	spacerH := m.height - vFrame - lipgloss.Height(body) - lipgloss.Height(hint)
	if spacerH < 1 {
		spacerH = 1
	}
	spacer := lipgloss.NewStyle().Height(spacerH).Render("")

	sections := lipgloss.JoinVertical(lipgloss.Left, body, spacer, hint)
	v := tea.NewView(docStyle.Render(sections))
	v.AltScreen = true
	return v
}

// standings-table column widths
const (
	wRank   = 2
	wClinch = 2 // clinch marker (x, y, e, …)
	wTeam   = 16
	wWL     = 3
	wPct    = 5
	wGB     = 4
	wL10    = 5
	wStrk   = 4
)

func renderConference(title string, teamColor color.Color, rows []backend.Standing, expanded bool) string {
	header := pad("#", wRank, true) + " " + pad("", wClinch, false) + " " +
		pad("TEAM", wTeam, false) + " " +
		pad("W", wWL, true) + " " + pad("L", wWL, true) + " " +
		pad("PCT", wPct, true) + " " + pad("GB", wGB, true) + " " +
		pad("L10", wL10, true) + " " + pad("STRK", wStrk, true)
	if expanded {
		header += " " + pad("CONF", 7, true) + " " + pad("DIV", 7, true) +
			" " + pad("HOME", 5, true) + " " + pad("ROAD", 5, true) +
			" " + pad("PF", 5, true) + " " + pad("PA", 5, true) + " " + pad("DIFF", 5, true)
	}

	titleLine := lipgloss.PlaceHorizontal(lipgloss.Width(header), lipgloss.Center,
		lipgloss.NewStyle().Bold(true).Foreground(teamColor).Render(title))

	lines := []string{
		titleLine,
		colHeaderSty.Render(header),
	}
	for _, s := range rows {
		lines = append(lines,
			pad(num(s.Rank), wRank, true)+" "+
				accentSty.Render(pad(s.Clinch, wClinch, false))+" "+
				pad(truncate(s.Team, wTeam), wTeam, false)+" "+
				pad(num(s.Wins), wWL, true)+" "+
				pad(num(s.Losses), wWL, true)+" "+
				pad(pctStr(s.PCT), wPct, true)+" "+
				pad(gbStr(s.GamesBack), wGB, true)+" "+
				pad(s.L10, wL10, true)+" "+
				pad(s.Streak, wStrk, true))
		if expanded {
			last := len(lines) - 1
			lines[last] += " " + pad(s.ConfRecord, 7, true) + " " + pad(s.DivRecord, 7, true) +
				" " + pad(s.HomeRecord, 5, true) + " " + pad(s.RoadRecord, 5, true) +
				" " + pad(fmt.Sprintf("%.1f", s.PointsPG), 5, true) + " " +
				pad(fmt.Sprintf("%.1f", s.OppPointsPG), 5, true) + " " +
				pad(fmt.Sprintf("%.1f", s.DiffPointsPG), 5, true)
		}
	}

	return panelSty.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// pctStr renders a win percentage NBA-style: ".652", "1.000".
func pctStr(p float64) string {
	s := fmt.Sprintf("%.3f", p)
	return strings.TrimPrefix(s, "0")
}

// gbStr renders games-back: "-" for the conference leader, else one decimal.
func gbStr(gb float64) string {
	if gb == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f", gb)
}
