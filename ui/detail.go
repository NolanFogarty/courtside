package ui

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/NolanFogarty/courtside/backend"
)

// ---- data ----------------------------------------------------------------
//
// Dummy data for now. The shapes mirror what the SDK's BoxScoreTraditionalV3
// and PlayByPlayV3 endpoints provide, so real data can slot in later.

type playerStat struct {
	name                              string
	min                               string
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
	accentColor  = lipgloss.Color("214")
	headerBarSty = lipgloss.NewStyle().Bold(true).Padding(0, 1).
			Border(lipgloss.RoundedBorder()).BorderForeground(dimColor)
	panelSty = lipgloss.NewStyle().Padding(0, 1).
			Border(lipgloss.RoundedBorder()).BorderForeground(dimColor)
	// focusedPanelSty highlights the currently selected (scrollable) box with an
	// accent-colored border.
	focusedPanelSty = lipgloss.NewStyle().Padding(0, 1).
			Border(lipgloss.RoundedBorder()).BorderForeground(accentColor)
	colHeaderSty = lipgloss.NewStyle().Bold(true).Foreground(mutedColor)
	dimRowSty    = lipgloss.NewStyle().Foreground(dimColor)
	mutedSty     = lipgloss.NewStyle().Foreground(mutedColor)
	accentSty    = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	hintSty      = lipgloss.NewStyle().Foreground(mutedColor)
	errSty       = lipgloss.NewStyle().Bold(true).Foreground(homeColor)

	// help-bar styling matched to the bubbles list help: muted key/desc/separator
	// colors and the list's left padding, so all key hints look the same.
	helpKeySty  = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262"))
	helpDescSty = lipgloss.NewStyle().Foreground(lipgloss.Color("#4A4A4A"))
	helpSepSty  = lipgloss.NewStyle().Foreground(lipgloss.Color("#3C3C3C"))
	helpSty     = lipgloss.NewStyle().PaddingLeft(2)
)

// renderHints renders key/description pairs in the same muted colors, separator,
// and left padding as the bubbles list's help bar.
func renderHints(pairs ...[2]string) string {
	sep := helpSepSty.Render(" • ")
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = helpKeySty.Render(p[0]) + " " + helpDescSty.Render(p[1])
	}
	return helpSty.Render(strings.Join(parts, sep))
}

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
	gameID        string
	game          gameDetail
	loading       bool
	err           error
	scheduled     bool       // game hasn't tipped off yet; no stats to fetch
	tipoff        string     // start time, shown when scheduled
	live          bool       // game in progress; auto-refresh its data
	expanded      bool       // show detailed (expanded) stats
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
	d := detail{gameID: g.GameId, live: g.IsLive(), width: width, height: height}
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

// refreshCmd re-fetches the box score and play-by-play, but only while the game
// is live. Existing scroll position and expanded state are preserved by Update.
func (m detail) refreshCmd() tea.Cmd {
	if !m.live {
		return nil
	}
	id := m.gameID
	return func() tea.Msg {
		d, err := backend.GetGameDetail(id)
		return gameDetailMsg{detail: d, err: err}
	}
}

// gameDetailMsg carries the result of the async GetGameDetail fetch.
type gameDetailMsg struct {
	detail backend.GameDetail
	err    error
}

func (m detail) Init() tea.Cmd {
	if m.scheduled {
		return nil // nothing to fetch until the game starts
	}
	id := m.gameID
	return func() tea.Msg {
		d, err := backend.GetGameDetail(id)
		return gameDetailMsg{detail: d, err: err}
	}
}

func (m detail) Update(msg tea.Msg) (detail, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case gameDetailMsg:
		wasLoading := m.loading
		m.loading = false
		if msg.err != nil {
			// Only surface an error on the initial load; ignore transient
			// auto-refresh failures and keep showing the last good data.
			if wasLoading {
				m.err = msg.err
			}
		} else {
			m.game = toGameDetail(msg.detail)
			m.err = nil
		}
	case tea.KeyPressMsg:
		switch msg.String() {
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
		}
	}
	return m, nil
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
			blk: p.Blk, to: p.To, plusMinus: p.PlusMinus,
			fgm: p.FGM, fga: p.FGA, tpm: p.TPM, tpa: p.TPA, ftm: p.FTM, fta: p.FTA,
			oreb: p.OReb, dreb: p.DReb, stl: p.Stl, pf: p.PF,
		}
	}
	return tb
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
	const headerH, blankH, hintH, minSpacerH = 3, 1, 1, 1
	return m.height - vFrame - headerH - blankH - hintH - minSpacerH
}

// sortedByPoints returns a copy of players ordered by points, highest first.
func sortedByPoints(players []playerStat) []playerStat {
	out := append([]playerStat(nil), players...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].pts > out[j].pts })
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
	if perTeam < 3 {
		perTeam = 3 // keep a usable window even on very short screens
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

	// Only the loaded box score has scrollable / expandable content.
	hint := renderHints([2]string{"q", "back"})
	if !m.scheduled && !m.loading && m.err == nil {
		statsDesc := "more stats"
		if m.expanded {
			statsDesc = "less stats"
		}
		hint = renderHints(
			[2]string{"tab/←→", "switch box"},
			[2]string{"↑/k", "up"},
			[2]string{"↓/j", "down"},
			[2]string{"o", statsDesc},
			[2]string{"q", "back"},
		)
	}

	// Push the hint to the bottom of the screen with a spacer that fills the
	// leftover height between the body and the hint.
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

// renderHeader is a centered scoreboard bar using the same away/score/home
// layout as the list view.
func (m detail) renderHeader(width int) string {
	g := m.game
	line := fmt.Sprintf("%-13s %3d - %-3d %13s",
		g.away.name, g.away.score, g.home.score, g.home.name)
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
		sortedByPoints(m.game.away.players), m.focus == focusAway, rows, m.awayScroll)
	bar := m.renderTeamBar()
	home := m.renderTeamTable(m.game.home, homeColor,
		sortedByPoints(m.game.home.players), m.focus == focusHome, rows, m.homeScroll)
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
	for _, c := range cols {
		hdr += " " + pad(c.header, c.width, true)
	}

	rows := make([]string, 0, len(window)+2)
	rows = append(rows, title, colHeaderSty.Render(hdr))
	for _, p := range window {
		row := pad(truncate(p.name, wName), wName, false)
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
