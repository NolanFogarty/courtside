package ui

import (
	"fmt"
	"image/color"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/NolanFogarty/courtside/backend"
	"github.com/charmbracelet/x/ansi"
)

// ---- data ----------------------------------------------------------------
//
// Dummy data for now. The shapes mirror what the SDK's BoxScoreTraditionalV3
// and PlayByPlayV3 endpoints provide, so real data can slot in later.

type playerStat struct {
	name                              string
	min                               string
	starter                           bool
	pts, ast, reb, blk, to, plusMinus int

	// expanded detail
	fgm, fga, tpm, tpa, ftm, fta int
	oreb, dreb, stl, pf          int
}

// played reports whether the player saw any court time. Minutes are the
// authoritative signal (a player can log time without recording a stat); the
// stat-line check is a fallback for sub-minute appearances, since minutes are
// truncated to whole numbers upstream. Players who didn't play are dimmed.
func (p playerStat) played() bool {
	if p.min != "" && p.min != "0" {
		return true
	}
	return p.pts != 0 || p.ast != 0 || p.reb != 0 || p.blk != 0 || p.to != 0
}

type teamBox struct {
	name    string
	tricode string
	score   int
	players []playerStat

	// aggregate stats shown in the comparison bar
	fgPct, fg3Pct float64
	ftm, fta      int
	reb, ast, to  int

	// expanded detail
	fgm, fga, tpm, tpa       int
	oreb, dreb, stl, blk, pf int
}

type playEvent struct {
	period int
	clock  string // "8:44"
	team   string // tricode; empty for neutral events
	desc   string
}

type gameDetail struct {
	away, home teamBox
	plays      []playEvent // newest first
}

// statRow is one row of the team-comparison table. away/home are the display
// strings; awayKey/homeKey are the numeric values used to decide which side to
// highlight. When lowerBetter is true the smaller value is the "leader".
type statRow struct {
	label            string
	away, home       string
	awayKey, homeKey float64
	lowerBetter      bool
}

// leader reports which side to highlight. Ties highlight neither.
func (r statRow) leader() (away, home bool) {
	if r.awayKey == r.homeKey {
		return false, false
	}
	awayWins := r.awayKey > r.homeKey
	if r.lowerBetter {
		awayWins = r.awayKey < r.homeKey
	}
	return awayWins, !awayWins
}

// ---- styles --------------------------------------------------------------

var (
	awayColor    = lipgloss.Color("39")  // blue
	homeColor    = lipgloss.Color("203") // red
	mutedColor   = lipgloss.Color("245")
	dimColor     = lipgloss.Color("240")
	accentColor  = lipgloss.Color("15")
	headerBarSty = lipgloss.NewStyle().Bold(true).Padding(0, 1).
			Border(lipgloss.RoundedBorder()).BorderForeground(dimColor)
	panelSty = lipgloss.NewStyle().Padding(0, 1).
			Border(lipgloss.RoundedBorder()).BorderForeground(dimColor)
	// focusedPanelSty highlights the currently selected (scrollable) box with an
	// accent-colored border.
	focusedPanelSty = lipgloss.NewStyle().Padding(0, 1).
			Border(lipgloss.RoundedBorder()).BorderForeground(accentColor)
	colHeaderSty   = lipgloss.NewStyle().Bold(true).Foreground(mutedColor)
	dimRowSty      = lipgloss.NewStyle().Foreground(dimColor)
	mutedSty       = lipgloss.NewStyle().Foreground(mutedColor)
	accentSty      = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	scoreWinnerSty = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("22")).Padding(0, 1)
	scoreLoserSty  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("88")).Padding(0, 1)
	hintSty        = lipgloss.NewStyle().Foreground(mutedColor)
	errSty         = lipgloss.NewStyle().Bold(true).Foreground(homeColor)

	// help-bar styling matched to the bubbles list help: muted key/desc/separator
	// colors and the list's left padding, so all key hints look the same.
	helpKeySty  = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262"))
	helpDescSty = lipgloss.NewStyle().Foreground(lipgloss.Color("#4A4A4A"))
	helpSepSty  = lipgloss.NewStyle().Foreground(lipgloss.Color("#3C3C3C"))
)

// stat-table column widths
const (
	wName = 14
	wNum  = 4 // PTS, AST, REB, BLK, TO
	wPM   = 5 // +/-
)

// ---- model ---------------------------------------------------------------

// panelFocus identifies which of the three scrollable boxes is currently
// selected (and therefore highlighted and driven by the scroll keys). The
// play-by-play feed is focused by default (zero value).
type panelFocus int

const (
	focusPBP panelFocus = iota
	focusAway
	focusHome
)

type detail struct {
	selection     backend.Game
	gameID        string
	game          gameDetail
	loading       bool
	err           error
	scheduled     bool   // game hasn't tipped off yet; no stats to fetch
	tipoff        string // start time, shown when scheduled
	live          bool   // game in progress; auto-refresh its data
	refreshCount  int    // auto-refresh polls since opening; stops runaway live polling
	fullPbp       bool   // play-by-play expanded to full screen
	fullScroll    int    // fullscreen row offset into the wrapped feed
	expanded      bool   // show detailed (expanded) stats
	sortMode      bool
	sortColumn    int
	sortBy        string
	sortDesc      bool
	focus         panelFocus // which box the scroll keys drive
	awayScroll    int        // away box scroll offset (0 = top scorer first)
	homeScroll    int        // home box scroll offset
	pbpScroll     int        // play-by-play scroll offset (0 = newest at top)
	width, height int
}

// newDetail seeds the header from the list selection (team names and scores are
// known immediately). A game that hasn't started has no box score / play-by-play
// to fetch, so we show its tip-off time instead of loading.
func newDetail(g backend.Game, width, height int) detail {
	d := detail{selection: g, gameID: g.GameId, live: g.IsLive(), width: width, height: height}
	d.game.away = teamBox{name: g.AwayTeam, score: g.AwayScore}
	d.game.home = teamBox{name: g.HomeTeam, score: g.HomeScore}
	if g.NotStarted() {
		d.scheduled = true
		d.tipoff = g.GameClock
	} else {
		d.loading = true
	}
	return d
}

// refreshCmd re-fetches the box score and play-by-play while the game is live
// or still scheduled (a pre-tip game may tip while open). Live polling stops
// after maxLiveRefreshes to avoid polling forever once the game is final.
// Existing scroll position and expanded state are preserved by Update.
func (m detail) refreshCmd() tea.Cmd {
	if !m.live && !m.scheduled {
		return nil
	}
	if m.live && m.refreshCount >= maxLiveRefreshes {
		return nil
	}
	return m.Init()
}

// gameDetailMsg carries the result of the async GetGameDetail fetch.
type gameDetailMsg struct {
	gameID string
	detail backend.GameDetail
	err    error
}

// maxLiveRefreshes caps live auto-refresh at ~3 hours (15s interval) so a game
// watched to final doesn't poll forever.
const maxLiveRefreshes = 720

type gamePlaysMsg struct {
	gameID string
	plays  []backend.PlayLine
}

func (m detail) Init() tea.Cmd {
	id := m.gameID
	selection := m.selection
	selection.GameId = id
	return tea.Batch(func() tea.Msg {
		d, err := backend.GetGameBoxScoreForGame(selection)
		return gameDetailMsg{gameID: id, detail: d, err: err}
	}, func() tea.Msg {
		return gamePlaysMsg{gameID: id, plays: backend.GetGamePlays(id)}
	})
}

func (m detail) Update(msg tea.Msg) (detail, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case gamePlaysMsg:
		if msg.gameID == m.gameID && len(msg.plays) > 0 {
			m.game.plays = toGameDetail(backend.GameDetail{Plays: msg.plays}).plays
			m.clampScrolls()
		}
	case gameDetailMsg:
		if msg.gameID != m.gameID {
			return m, nil
		}
		wasLoading := m.loading
		m.loading = false
		if msg.err != nil {
			// Only surface an error on the initial load; ignore transient
			// auto-refresh failures and keep showing the last good data.
			if wasLoading {
				m.err = msg.err
			} else if m.live || m.scheduled {
				m.refreshCount++
				if m.live && m.refreshCount >= maxLiveRefreshes {
					m.live = false
				}
			}
		} else {
			game := toGameDetail(msg.detail)
			// A scheduled game has no box score yet, so an empty response
			// must not wipe the seeded header (names and scores).
			if game.away.name == "" && len(game.away.players) == 0 {
				game.away = m.game.away
			}
			if game.home.name == "" && len(game.home.players) == 0 {
				game.home = m.game.home
			}
			// Play-by-play arrives independently; neither arrival order nor
			// a temporary feed failure should erase the last good plays.
			if len(game.plays) == 0 {
				game.plays = m.game.plays
			}
			m.game = game
			m.err = nil
			if m.scheduled && len(msg.detail.Away.Players)+len(msg.detail.Home.Players) > 0 {
				m.scheduled = false
				m.live = true
				m.refreshCount = 0
			} else if (m.live || m.scheduled) && !wasLoading {
				m.refreshCount++
				if m.live && m.refreshCount >= maxLiveRefreshes {
					m.live = false
				}
			}
		}
	case tea.KeyPressMsg:
		if m.sortMode {
			switch msg.String() {
			case "esc", "q":
				m.sortMode = false
			case "left", "h":
				m.sortColumn = (m.sortColumn + len(m.playerCols()) - 1) % len(m.playerCols())
			case "right", "l":
				m.sortColumn = (m.sortColumn + 1) % len(m.playerCols())
			case "enter":
				m.applySelectedSort()
			case "up", "down":
				m.applySelectedSort()
			}
			return m, nil
		}
		if m.fullPbp {
			switch msg.String() {
			case "esc", "q", "enter":
				m.fullPbp = false
			case "up", "k":
				m.fullScroll = clampInt(m.fullScroll-1, 0, m.fullMaxScroll())
			case "down", "j":
				m.fullScroll = clampInt(m.fullScroll+1, 0, m.fullMaxScroll())
			case "pgup":
				m.fullScroll = clampInt(m.fullScroll-m.fullVisibleRows(), 0, m.fullMaxScroll())
			case "pgdown":
				m.fullScroll = clampInt(m.fullScroll+m.fullVisibleRows(), 0, m.fullMaxScroll())
			case "home":
				m.fullScroll = 0
			case "end":
				m.fullScroll = m.fullMaxScroll()
			}
			return m, nil
		}
		switch msg.String() {
		case "s":
			m.sortMode = true
			m.sortColumn = 0
		case "enter":
			if !m.scheduled && !m.loading && m.err == nil && m.focus == focusPBP {
				m.fullPbp = true
				m.fullScroll = 0
			}
		case "o":
			m.expanded = !m.expanded
			m.clampScrolls()
		case "tab", "right", "l":
			m.focus = (m.focus + 1) % 3
		case "shift+tab", "left", "h":
			m.focus = (m.focus + 2) % 3
		case "up", "k":
			m.scrollFocused(-1)
		case "down", "j":
			m.scrollFocused(1)
		case "pgup":
			m.pageFocused(-1)
		case "pgdown":
			m.pageFocused(1)
		}
	}
	return m, nil
}

func (m *detail) applySelectedSort() {
	cols := m.playerCols()
	if len(cols) == 0 {
		return
	}
	if m.sortColumn < 0 || m.sortColumn >= len(cols) {
		m.sortColumn = 0
	}
	column := cols[m.sortColumn].header
	if m.sortBy == column {
		m.sortDesc = !m.sortDesc
	} else {
		m.sortBy = column
		m.sortDesc = true
	}
	m.awayScroll, m.homeScroll = 0, 0
}

// pageFocused moves the active box by approximately one visible screen.
func (m *detail) pageFocused(direction int) {
	step := m.pbpVisible()
	if m.focus != focusPBP {
		step = m.teamVisibleRows()
	}
	if step < 1 {
		step = 1
	}
	m.scrollFocused(direction * step)
}

// scrollFocused moves the focused box's scroll offset by delta, clamped to that
// box's range.
func (m *detail) scrollFocused(delta int) {
	switch m.focus {
	case focusAway:
		m.awayScroll = clampInt(m.awayScroll+delta, 0, m.awayMaxScroll())
	case focusHome:
		m.homeScroll = clampInt(m.homeScroll+delta, 0, m.homeMaxScroll())
	default: // focusPBP
		m.pbpScroll = clampInt(m.pbpScroll+delta, 0, m.maxScroll())
	}
}

// clampScrolls re-clamps every box's scroll offset, used after a layout change
// (e.g. toggling expanded stats) shrinks a box's scroll range.
func (m *detail) clampScrolls() {
	m.awayScroll = clampInt(m.awayScroll, 0, m.awayMaxScroll())
	m.homeScroll = clampInt(m.homeScroll, 0, m.homeMaxScroll())
	m.pbpScroll = clampInt(m.pbpScroll, 0, m.maxScroll())
}

// toGameDetail maps the backend's GameDetail into the view's render types.
func toGameDetail(d backend.GameDetail) gameDetail {
	g := gameDetail{away: toTeamBox(d.Away), home: toTeamBox(d.Home)}
	g.plays = make([]playEvent, len(d.Plays))
	for i, p := range d.Plays {
		g.plays[i] = playEvent{period: p.Period, clock: p.Clock, team: p.Team, desc: p.Desc}
	}
	return g
}

func toTeamBox(t backend.TeamDetail) teamBox {
	tb := teamBox{
		name: t.Name, tricode: t.Tricode, score: t.Score,
		fgPct: t.FGPct, fg3Pct: t.FG3Pct, ftm: t.FTM, fta: t.FTA,
		reb: t.Reb, ast: t.Ast, to: t.To,
		fgm: t.FGM, fga: t.FGA, tpm: t.TPM, tpa: t.TPA,
		oreb: t.OReb, dreb: t.DReb, stl: t.Stl, blk: t.Blk, pf: t.PF,
	}
	tb.players = make([]playerStat, len(t.Players))
	for i, p := range t.Players {
		tb.players[i] = playerStat{
			name: p.Name, min: p.Min, pts: p.Pts, ast: p.Ast, reb: p.Reb,
			starter: p.Starter,
			blk:     p.Blk, to: p.To, plusMinus: p.PlusMinus,
			fgm: p.FGM, fga: p.FGA, tpm: p.TPM, tpa: p.TPA, ftm: p.FTM, fta: p.FTA,
			oreb: p.OReb, dreb: p.DReb, stl: p.Stl, pf: p.PF,
		}
	}
	markStartingFive(tb.players)
	return tb
}

func markStartingFive(players []playerStat) {
	count := 0
	for _, player := range players {
		if player.starter {
			count++
		}
	}
	if count == 5 || len(players) < 5 {
		return
	}
	indices := make([]int, len(players))
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(i, j int) bool {
		mi, _ := strconv.Atoi(players[indices[i]].min)
		mj, _ := strconv.Atoi(players[indices[j]].min)
		return mi > mj
	})
	for i := range players {
		players[i].starter = false
	}
	for _, i := range indices[:5] {
		players[i].starter = true
	}
}

// maxScroll is the furthest the play-by-play feed can scroll back.
func (m detail) maxScroll() int {
	max := len(m.game.plays) - m.pbpVisible()
	if max < 0 {
		return 0
	}
	return max
}

// leftColumnHeight is the rendered height of the left column (both player
// tables plus the team-stats bar between them).
func (m detail) leftColumnHeight() int {
	return lipgloss.Height(m.renderPlayerColumn())
}

// playerColumnBudget is the vertical space the player column can occupy before
// it overflows the screen: the window height minus the document margins, the
// scoreboard header (3) and the blank line under it, the bottom help hint, and
// the minimum one-line spacer above that hint. A non-positive height (before
// the first WindowSizeMsg) yields a huge budget so nothing is trimmed yet.
func (m detail) playerColumnBudget() int {
	if m.height <= 0 {
		return 1 << 30
	}
	_, vFrame := docStyle.GetFrameSize()
	const headerH, blankH, minSpacerH = 3, 1, 1
	return m.height - vFrame - headerH - blankH - lipgloss.Height(m.hints()) - minSpacerH
}

// sortedByPoints returns a copy of players ordered by points, highest first.
func sortedByPoints(players []playerStat) []playerStat {
	out := append([]playerStat(nil), players...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].pts > out[j].pts })
	return out
}

func (m detail) sortedPlayers(players []playerStat) []playerStat {
	out := append([]playerStat(nil), players...)
	if m.sortBy == "" {
		return sortedByPoints(out)
	}
	value := func(p playerStat) float64 {
		switch m.sortBy {
		case "MIN":
			v, _ := strconv.ParseFloat(p.min, 64)
			return v
		case "PTS":
			return float64(p.pts)
		case "FG":
			return float64(p.fgm)
		case "3P":
			return float64(p.tpm)
		case "FT":
			return float64(p.ftm)
		case "REB":
			return float64(p.reb)
		case "AST":
			return float64(p.ast)
		case "STL":
			return float64(p.stl)
		case "BLK":
			return float64(p.blk)
		case "TO":
			return float64(p.to)
		case "PF":
			return float64(p.pf)
		case "+/-":
			return float64(p.plusMinus)
		}
		return float64(p.pts)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if m.sortDesc {
			return value(out[i]) > value(out[j])
		}
		return value(out[i]) < value(out[j])
	})
	return out
}

// teamVisibleRows is how many player rows each team box shows. When the full
// rosters fit the vertical budget every player is shown; otherwise the boxes are
// capped to a shorter, scrollable window (split evenly so the two sides stay
// balanced) rather than dropping players. Both teams use the same row count.
func (m detail) teamVisibleRows() int {
	maxN := len(m.game.away.players)
	if n := len(m.game.home.players); n > maxN {
		maxN = n
	}

	// The bar (header + 2 team rows + border) plus a blank line above and below
	// is fixed overhead; the rest of the budget is split between the two boxes,
	// each of which spends 4 lines on chrome (border, title, header).
	barBlock := lipgloss.Height(m.renderTeamBar()) + 2
	perTeam := (m.playerColumnBudget() - barBlock - 2*4) / 2
	if perTeam < 1 {
		perTeam = 1 // always show at least one player per side
	}
	if perTeam > maxN {
		perTeam = maxN
	}
	return perTeam
}

// awayMaxScroll / homeMaxScroll report how far each team box can scroll given
// its roster size and the current visible-row window.
func (m detail) awayMaxScroll() int {
	return teamScrollMax(len(m.game.away.players), m.teamVisibleRows())
}

func (m detail) homeMaxScroll() int {
	return teamScrollMax(len(m.game.home.players), m.teamVisibleRows())
}

func teamScrollMax(total, visible int) int {
	if total <= visible {
		return 0
	}
	return total - visible
}

// pbpVisible is how many play-by-play lines show at once: enough to fill the
// play-by-play box to the full height of the left column.
func (m detail) pbpVisible() int {
	v := m.leftColumnHeight() - 4 // box border (2) + title & divider (2)
	if v < 4 {
		v = 4
	}
	return v
}

// ---- view ----------------------------------------------------------------

func (m detail) View() tea.View {
	hFrame, vFrame := docStyle.GetFrameSize()
	width := m.width - hFrame
	if width < 40 {
		width = 40
	}

	var body string
	switch {
	case m.fullPbp:
		body = m.renderPBPFullscreen(width)
	case m.scheduled:
		note := "Not started"
		if m.tipoff != "" {
			note = "Tip-off · " + m.tipoff
		}
		body = lipgloss.JoinVertical(lipgloss.Left, m.renderHeader(width), "",
			lipgloss.PlaceHorizontal(width, lipgloss.Center, mutedSty.Render(note)))
	case m.err != nil:
		body = lipgloss.JoinVertical(lipgloss.Left, m.renderHeader(width), "",
			lipgloss.PlaceHorizontal(width, lipgloss.Center,
				errSty.Render("Failed to load game: "+m.err.Error())))
	case m.loading:
		body = lipgloss.JoinVertical(lipgloss.Left, m.renderHeader(width), "",
			lipgloss.PlaceHorizontal(width, lipgloss.Center,
				mutedSty.Render("Loading game data…")))
	default:
		body = lipgloss.JoinVertical(lipgloss.Left, m.renderHeader(width), "", m.renderMain(width))
	}

	hintWidth := max(contentWidth(m.width), width)
	hint := m.hintsForWidth(hintWidth)

	body = lipgloss.NewStyle().MaxWidth(contentWidth(m.width)).Render(body)

	// The hint must always stay visible. On a terminal too short to hold the
	// whole body plus the hint (and a one-line spacer), clip the body rather than
	// letting the alt-screen renderer drop the hint off the bottom.
	availH := m.height - vFrame
	if maxBodyH := availH - lipgloss.Height(hint) - 1; maxBodyH > 0 && lipgloss.Height(body) > maxBodyH {
		body = lipgloss.NewStyle().MaxHeight(maxBodyH).Render(body)
	}

	// Push the hint to the bottom of the screen with a spacer that fills the
	// leftover height between the body and the hint.
	spacerH := availH - lipgloss.Height(body) - lipgloss.Height(hint)
	if spacerH < 1 {
		spacerH = 1
	}
	spacer := lipgloss.NewStyle().Height(spacerH).Render("")

	sections := lipgloss.JoinVertical(lipgloss.Left, body, spacer, hint)

	v := tea.NewView(docStyle.Render(sections))
	v.AltScreen = true
	return v
}

func (m detail) hints() string {
	return m.hintsForWidth(contentWidth(m.width))
}

func (m detail) hintsForWidth(w int) string {
	if m.fullPbp {
		return renderHints(w, [2]string{"↑/↓", "scroll"}, [2]string{"pgup/pgdn", "page"}, [2]string{"enter/esc", "exit full screen"})
	}
	// Only the loaded box score has scrollable / expandable content.
	hint := renderHints(w, [2]string{"t", "teams"}, [2]string{"q", "back"})
	if m.sortMode {
		hint = renderHints(w, [2]string{"←/→", "select stat"}, [2]string{"enter/↑/↓", "sort/toggle direction"}, [2]string{"esc/q", "done"})
	} else if !m.scheduled && !m.loading && m.err == nil {
		statsDesc := "more stats"
		if m.expanded {
			statsDesc = "less stats"
		}
		pairs := [][2]string{
			{"tab/←→", "switch box"},
			{"↑/k", "up"},
			{"↓/j", "down"},
			{"pgup/pgdn", "page"},
			{"s", "sort"},
			{"o", statsDesc},
		}
		if m.focus == focusPBP {
			pairs = append(pairs, [2]string{"enter", "full play-by-play"})
		}
		pairs = append(pairs, [2]string{"t", "teams"}, [2]string{"q", "back"})
		hint = renderHints(w, pairs...)
	}

	return hint
}

// renderHeader is a centered scoreboard bar using the same away/score/home
// layout as the list view.
func (m detail) renderHeader(width int) string {
	g := m.game
	if m.scheduled && g.away.score == 0 && g.home.score == 0 {
		line := fmt.Sprintf("%-13s %7s %13s", g.away.name, "vs", g.home.name)
		bar := headerBarSty.Render(line)
		return lipgloss.PlaceHorizontal(width, lipgloss.Center, bar)
	}
	awayScore := fmt.Sprintf("%d", g.away.score)
	homeScore := fmt.Sprintf("%d", g.home.score)
	if g.away.score > g.home.score {
		awayScore = scoreWinnerSty.Render(awayScore)
		homeScore = scoreLoserSty.Render(homeScore)
	} else if g.home.score > g.away.score {
		awayScore = scoreLoserSty.Render(awayScore)
		homeScore = scoreWinnerSty.Render(homeScore)
	}
	line := fmt.Sprintf("%-13s %3s - %-3s %13s",
		g.away.name, awayScore, homeScore, g.home.name)
	bar := headerBarSty.Render(line)
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, bar)
}

// renderMain lays out the two-column body: the player tables (with the team
// stats bar between them) stacked on the left, and the play-by-play feed
// filling the right, splitting the width down the middle.
func (m detail) renderMain(width int) string {
	const gap = 2
	colW := (width - gap) / 2

	left := m.renderPlayerColumn()
	leftW := lipgloss.Width(left)

	// Compact tables fit within half the screen, so keep the centered 50/50
	// feel by right-aligning the left column in its half. The wider expanded
	// tables claim the room they need and the feed takes whatever is left.
	if leftW <= colW {
		left = lipgloss.PlaceHorizontal(colW, lipgloss.Right, left)
		leftW = colW
	}
	rightW := width - leftW - gap
	if rightW < 24 {
		rightW = 24
	}
	right := m.renderInfoColumn(rightW, lipgloss.Height(left), m.focus == focusPBP)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

// renderPlayerColumn stacks the away player table, the horizontal team-stats
// bar, and the home player table vertically. Each team table is a scrollable
// window into its (points-sorted) roster, sized by teamVisibleRows; the focused
// box is highlighted.
func (m detail) renderPlayerColumn() string {
	rows := m.teamVisibleRows()
	away := m.renderTeamTable(m.game.away, awayColor,
		m.sortedPlayers(m.game.away.players), m.focus == focusAway, rows, m.awayScroll)
	bar := m.renderTeamBar()
	home := m.renderTeamTable(m.game.home, homeColor,
		m.sortedPlayers(m.game.home.players), m.focus == focusHome, rows, m.homeScroll)
	return lipgloss.JoinVertical(lipgloss.Left, away, "", bar, "", home)
}

// renderInfoColumn renders the play-by-play box, sized to its own content width
// (bounded by w) and stretched to the full height of the left column. The box
// border is highlighted when focused.
func (m detail) renderInfoColumn(w, height int, focused bool) string {
	maxW := w - 4 // box border (2) + padding (2)
	if maxW < 24 {
		maxW = 24
	}
	pbp := m.renderPBPColumn(m.pbpWidth(maxW))

	// Style.Height sets the box's total (border-inclusive) height, so target the
	// left column's height directly. Grow if the content would be taller.
	target := height
	if nat := lipgloss.Height(pbp) + 2; target < nat {
		target = nat
	}
	sty := panelSty
	if focused {
		sty = focusedPanelSty
	}
	return sty.Height(target).Render(pbp)
}

// pbpWidth is the content width the play-by-play feed needs to show its longest
// event line without truncation, capped at max. Computed over all plays (not
// just the visible window) so the box width stays stable while scrolling.
func (m detail) pbpWidth(max int) int {
	maxDesc := 0
	for _, p := range m.game.plays {
		if dw := lipgloss.Width(p.desc); dw > maxDesc {
			maxDesc = dw
		}
	}
	w := wWhen + 2 + wTag + 2 + maxDesc
	if len(m.game.plays) == 0 {
		// Keep room for the "unavailable" note so the divider stays aligned.
		if nw := lipgloss.Width(pbpUnavailable); w < nw {
			w = nw
		}
	}
	if w > max {
		w = max
	}
	return w
}

// statCol is one numeric column of the player table: a header, a fixed width,
// and how to render a player's value.
type statCol struct {
	header string
	width  int
	value  func(p playerStat) string
}

func num(n int) string        { return fmt.Sprintf("%d", n) }
func ma(made, att int) string { return fmt.Sprintf("%d-%d", made, att) }

// playerCols returns the player-table columns for the current mode.
func (m detail) playerCols() []statCol {
	if m.expanded {
		return []statCol{
			{"MIN", 4, func(p playerStat) string { return p.min }},
			{"PTS", 4, func(p playerStat) string { return num(p.pts) }},
			{"FG", 6, func(p playerStat) string { return ma(p.fgm, p.fga) }},
			{"3P", 5, func(p playerStat) string { return ma(p.tpm, p.tpa) }},
			{"FT", 5, func(p playerStat) string { return ma(p.ftm, p.fta) }},
			{"REB", 4, func(p playerStat) string { return num(p.reb) }},
			{"AST", 4, func(p playerStat) string { return num(p.ast) }},
			{"STL", 4, func(p playerStat) string { return num(p.stl) }},
			{"BLK", 4, func(p playerStat) string { return num(p.blk) }},
			{"TO", 4, func(p playerStat) string { return num(p.to) }},
			{"PF", 4, func(p playerStat) string { return num(p.pf) }},
			{"+/-", 5, func(p playerStat) string { return signed(p.plusMinus) }},
		}
	}
	return []statCol{
		{"PTS", wNum, func(p playerStat) string { return num(p.pts) }},
		{"AST", wNum, func(p playerStat) string { return num(p.ast) }},
		{"REB", wNum, func(p playerStat) string { return num(p.reb) }},
		{"BLK", wNum, func(p playerStat) string { return num(p.blk) }},
		{"TO", wNum, func(p playerStat) string { return num(p.to) }},
		{"+/-", wPM, func(p playerStat) string { return signed(p.plusMinus) }},
	}
}

// tableWidth is the content width of a player table with the given columns:
// the name column plus each column preceded by a single-space separator.
func tableWidth(cols []statCol) int {
	w := wName
	for _, c := range cols {
		w += 1 + c.width
	}
	return w
}

// renderTeamTable renders one team's box score as a scrollable window into the
// (already points-sorted) player slice: at most maxRows player rows starting at
// scroll, with a compact ↑/↓ indicator of how many are hidden. DNP rows are
// dimmed. The box border is highlighted when focused.
func (m detail) renderTeamTable(t teamBox, teamColor color.Color, players []playerStat, focused bool, maxRows, scroll int) string {
	cols := m.playerCols()

	total := len(players)
	if maxRows > total {
		maxRows = total
	}
	scroll = clampInt(scroll, 0, total-maxRows)
	window := players[scroll : scroll+maxRows]
	above, below := scroll, total-scroll-maxRows

	title := lipgloss.NewStyle().Bold(true).Foreground(teamColor).
		Render(fmt.Sprintf("%s (%d)", t.name, t.score))
	title += scrollHint(above, below)

	hdr := pad("PLAYER", wName, false)
	for i, c := range cols {
		cell := pad(c.header, c.width, true)
		if m.sortMode && i == m.sortColumn {
			cell = accentSty.Render(cell)
		}
		hdr += " " + cell
	}

	rows := make([]string, 0, len(window)+2)
	rows = append(rows, title, colHeaderSty.Render(hdr))
	for _, p := range window {
		var playerCell string
		if p.starter {
			playerCell = pad("★ "+truncate(p.name, wName-2), wName, false)
		} else {
			playerCell = pad(truncate(p.name, wName), wName, false)
		}
		row := playerCell
		for _, c := range cols {
			row += " " + pad(c.value(p), c.width, true)
		}
		if !p.played() {
			row = dimRowSty.Render(row)
		}
		rows = append(rows, row)
	}

	sty := panelSty
	if focused {
		sty = focusedPanelSty
	}
	return sty.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

// scrollHint renders a compact "↑N ↓N" indicator of how many rows are hidden
// above and below a scroll window. Empty when nothing is hidden.
func scrollHint(above, below int) string {
	if above == 0 && below == 0 {
		return ""
	}
	parts := make([]string, 0, 2)
	if above > 0 {
		parts = append(parts, fmt.Sprintf("↑%d", above))
	}
	if below > 0 {
		parts = append(parts, fmt.Sprintf("↓%d", below))
	}
	return hintSty.Render("  " + strings.Join(parts, " "))
}

// width of each team-stats bar stat column.
const barStatW = 6

// barStatRows builds the six columns shown in the horizontal team-stats bar
// from the game's aggregate stats, in display order: shooting (FG%, 3P%, FTM)
// then team (REB, AST, TO).
func (m detail) barStatRows() []statRow {
	a, h := m.game.away, m.game.home
	if m.expanded {
		return []statRow{
			maRow("FG", a.fgm, a.fga, h.fgm, h.fga),
			pctRow("FG%", a.fgPct, h.fgPct),
			maRow("3P", a.tpm, a.tpa, h.tpm, h.tpa),
			pctRow("3P%", a.fg3Pct, h.fg3Pct),
			ftmRow("FT", a.ftm, a.fta, h.ftm, h.fta),
			intRow("OREB", a.oreb, h.oreb, false),
			intRow("REB", a.reb, h.reb, false),
			intRow("AST", a.ast, h.ast, false),
			intRow("STL", a.stl, h.stl, false),
			intRow("BLK", a.blk, h.blk, false),
			intRow("TO", a.to, h.to, true), // fewer turnovers leads
			intRow("PF", a.pf, h.pf, true), // fewer fouls leads
		}
	}
	return []statRow{
		pctRow("FG%", a.fgPct, h.fgPct),
		pctRow("3P%", a.fg3Pct, h.fg3Pct),
		ftmRow("FTM", a.ftm, a.fta, h.ftm, h.fta),
		intRow("REB", a.reb, h.reb, false),
		intRow("AST", a.ast, h.ast, false),
		intRow("TO", a.to, h.to, true), // fewer turnovers leads
	}
}

// maRow is a "made-attempted" comparison column; the team with more makes leads.
func maRow(label string, am, aa, hm, ha int) statRow {
	return statRow{label: label,
		away: fmt.Sprintf("%d-%d", am, aa), home: fmt.Sprintf("%d-%d", hm, ha),
		awayKey: float64(am), homeKey: float64(hm)}
}

func pctRow(label string, a, h float64) statRow {
	return statRow{label: label,
		away: fmt.Sprintf("%.1f", a), home: fmt.Sprintf("%.1f", h),
		awayKey: a, homeKey: h}
}

func ftmRow(label string, am, aa, hm, ha int) statRow {
	return statRow{label: label,
		away: fmt.Sprintf("%d/%d", am, aa), home: fmt.Sprintf("%d/%d", hm, ha),
		awayKey: float64(am), homeKey: float64(hm)} // more made free throws leads
}

func intRow(label string, a, h int, lowerBetter bool) statRow {
	return statRow{label: label,
		away: fmt.Sprintf("%d", a), home: fmt.Sprintf("%d", h),
		awayKey: float64(a), homeKey: float64(h), lowerBetter: lowerBetter}
}

// renderTeamBar renders the horizontal team-stats bar between the two player
// tables: a header row of column labels followed by one row per team. The TEAM
// column is sized so the bar spans the player tables exactly.
func (m detail) renderTeamBar() string {
	rows := m.barStatRows()

	teamW := tableWidth(m.playerCols()) - len(rows)*barStatW
	if teamW < 4 {
		teamW = 4
	}

	hdr := pad("TEAM", teamW, false)
	for _, r := range rows {
		hdr += pad(r.label, barStatW, true)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		colHeaderSty.Render(hdr),
		m.barTeamRow(rows, m.game.away.tricode, awayColor, true, teamW),
		m.barTeamRow(rows, m.game.home.tricode, homeColor, false, teamW),
	)
	return panelSty.Render(content)
}

// barTeamRow renders one team's row of the bar, highlighting the cells where
// this team leads.
func (m detail) barTeamRow(rows []statRow, tricode string, teamColor color.Color, isAway bool, teamW int) string {
	row := lipgloss.NewStyle().Bold(true).Foreground(teamColor).
		Render(pad(tricode, teamW, false))
	for _, r := range rows {
		val, leads := r.home, false
		awayLeads, homeLeads := r.leader()
		if isAway {
			val, leads = r.away, awayLeads
		} else {
			leads = homeLeads
		}
		cell := pad(val, barStatW, true)
		if leads {
			cell = accentSty.Render(cell)
		}
		row += cell
	}
	return row
}

// renderPBPColumn renders the scrollable play-by-play feed, newest first.
func (m detail) renderPBPColumn(w int) string {
	visible := m.pbpVisible()
	plays := m.game.plays
	start := m.pbpScroll
	if start > m.maxScroll() {
		start = m.maxScroll()
	}
	end := start + visible
	if end > len(plays) {
		end = len(plays)
	}

	title := colHeaderSty.Render("PLAY-BY-PLAY")
	if start > 0 {
		title += hintSty.Render(fmt.Sprintf("   ↑ %d more", start))
	}
	lines := []string{title, dimRowSty.Render(strings.Repeat("─", w))}

	if len(plays) == 0 {
		lines = append(lines, mutedSty.Render(pbpUnavailable))
	}
	for _, p := range plays[start:end] {
		lines = append(lines, renderPlay(p, m.game.home.tricode, w))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// pbpUnavailable is shown in the feed when no play-by-play could be loaded (the
// endpoint failed, or the game just tipped off).
const pbpUnavailable = "Play-by-play unavailable"

// fullPBPRows flattens the feed (newest first) into display rows at width w.
// A play that doesn't fit wraps onto continuation lines indented under its
// first line, so nothing is silently truncated in fullscreen mode.
func (m detail) fullPBPRows(w int) []string {
	descW := w - wWhen - 2 - wTag - 2
	if descW < 10 {
		descW = 10
	}
	rows := []string{}
	for _, p := range m.game.plays {
		rows = append(rows, renderPlayWrapped(p, m.game.home.tricode, descW)...)
	}
	return rows
}

// renderPlayWrapped renders one play as one or more display rows: the clock
// and team tag on the first line, overflow words wrapped onto indented lines.
func renderPlayWrapped(p playEvent, homeTricode string, descW int) []string {
	when := mutedSty.Render(pad(fmt.Sprintf("Q%d %s", p.period, mmss(p.clock)), wWhen, false))
	var tag string
	if p.team == "" {
		tag = dimRowSty.Render(pad("---", wTag, false))
	} else {
		tagColor := awayColor
		if p.team == homeTricode {
			tagColor = homeColor
		}
		tag = lipgloss.NewStyle().Bold(true).Foreground(tagColor).
			Render(pad("["+p.team+"]", wTag, false))
	}
	prefix := when + "  " + tag + "  "
	indent := strings.Repeat(" ", wWhen+2+wTag+2)
	chunks := strings.Split(ansi.Wrap(p.desc, descW, ""), "\n")
	rows := []string{prefix + chunks[0]}
	for _, c := range chunks[1:] {
		rows = append(rows, indent+c)
	}
	if p.team == "" {
		for i := range rows {
			rows[i] = dimRowSty.Italic(true).Render(rows[i])
		}
	}
	return rows
}

// fullVisibleRows is how many wrapped feed rows fit in fullscreen mode.
func (m detail) fullVisibleRows() int {
	hFrame, vFrame := docStyle.GetFrameSize()
	width := m.width - hFrame
	if width < 40 {
		width = 40
	}
	hintH := lipgloss.Height(m.hintsForWidth(max(contentWidth(m.width), width)))
	room := m.height - vFrame - hintH - 1 - 4 // spacer, header, blank, title, divider
	return max(1, room)
}

// fullMaxScroll is the furthest the fullscreen feed can scroll.
func (m detail) fullMaxScroll() int {
	rows := m.fullPBPRows(max(1, contentWidth(m.width)-4))
	return max(0, len(rows)-m.fullVisibleRows())
}

// renderPBPFullscreen renders the whole body as the play-by-play feed:
// header on top, then as many wrapped rows as fit.
func (m detail) renderPBPFullscreen(width int) string {
	inner := max(1, contentWidth(width)-4)
	rows := m.fullPBPRows(inner)
	visible := m.fullVisibleRows()
	start := clampInt(m.fullScroll, 0, max(0, len(rows)-visible))
	end := min(len(rows), start+visible)
	title := colHeaderSty.Render("PLAY-BY-PLAY · FULL SCREEN")
	if start > 0 {
		title += hintSty.Render(fmt.Sprintf("   ↑ %d more", start))
	}
	if end < len(rows) {
		title += hintSty.Render(fmt.Sprintf("   ↓ %d more", len(rows)-end))
	}
	lines := []string{m.renderHeader(width), "", title, dimRowSty.Render(strings.Repeat("─", inner))}
	if len(rows) == 0 {
		lines = append(lines, mutedSty.Render(pbpUnavailable))
	}
	lines = append(lines, rows[start:end]...)
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// play-by-play line layout: "Q2 08:44" (8) + gap + "[NYK]" (5) + gap + desc
const (
	wWhen = 8
	wTag  = 5
)

func renderPlay(p playEvent, homeTricode string, width int) string {
	when := mutedSty.Render(pad(fmt.Sprintf("Q%d %s", p.period, mmss(p.clock)), wWhen, false))

	descW := width - wWhen - 2 - wTag - 2
	if descW < 4 {
		descW = 4
	}

	if p.team == "" {
		// Neutral event (timeout, jump ball, end of period) — fully muted.
		tag := dimRowSty.Render(pad("---", wTag, false))
		return when + "  " + tag + "  " + dimRowSty.Italic(true).Render(truncate(p.desc, descW))
	}

	tagColor := awayColor
	if p.team == homeTricode {
		tagColor = homeColor
	}
	tag := lipgloss.NewStyle().Bold(true).Foreground(tagColor).
		Render(pad("["+p.team+"]", wTag, false))
	return when + "  " + tag + "  " + truncate(p.desc, descW)
}

// mmss normalizes a "M:SS" clock to zero-padded "MM:SS".
func mmss(c string) string {
	var m, s int
	if _, err := fmt.Sscanf(c, "%d:%d", &m, &s); err != nil {
		return c
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// ---- helpers -------------------------------------------------------------

// pad left- or right-aligns s within width w (right=true right-aligns).
func pad(s string, w int, right bool) string {
	gap := w - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	if right {
		return strings.Repeat(" ", gap) + s
	}
	return s + strings.Repeat(" ", gap)
}

func truncate(s string, w int) string {
	if len(s) <= w {
		return s
	}
	if w <= 1 {
		return s[:w]
	}
	return s[:w-1] + "…"
}

func signed(n int) string {
	if n > 0 {
		return fmt.Sprintf("+%d", n)
	}
	return fmt.Sprintf("%d", n)
}

// clampInt constrains v to the inclusive [lo, hi] range. A hi below lo (an empty
// range) collapses to lo.
func clampInt(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
