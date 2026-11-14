package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/NolanFogarty/courtside/backend"
)

const (
	listview int = iota
	detailview
	standingsview
	scheduleview
	teamview
	playerview
)

// refreshInterval is how often live data (the list while viewing today, and an
// in-progress game's detail) is re-fetched.
const refreshInterval = 15 * time.Second

// refreshTickMsg is the periodic heartbeat that drives auto-refresh.
type refreshTickMsg struct{}

func refreshTick() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return refreshTickMsg{} })
}

type root struct {
	current        int
	detailReturn   int
	list           gamelist
	detail         detail
	standings      standings
	schedule       schedule
	scheduleReturn int
	teams          teams
	player         player
	teamReturn     int
	playerRequest  int
	width, height  int
}

func newRoot(games []backend.Game) root {
	return root{
		current: listview,
		list:    newGamesList(games),
		teams:   newTeams(0, 0),
	}
}

func (r root) Init() tea.Cmd {
	return tea.Batch(r.list.Init(), refreshTick())
}

func (r root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Quit from anywhere
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+c" {
		return r, tea.Quit
	}

	// Keep all sub-views sized regardless of which is active.
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		r.width, r.height = ws.Width, ws.Height
		updated, cmd := r.list.Update(ws)
		r.list = updated.(gamelist)
		r.detail, _ = r.detail.Update(ws)
		r.standings, _ = r.standings.Update(ws)
		r.schedule, _ = r.schedule.Update(ws)
		r.teams, _ = r.teams.Update(ws)
		r.player, _ = r.player.Update(ws)
		return r, cmd
	}

	// Async responses belong to their models even when a different page is
	// active. Request IDs prevent an old team or player fetch replacing a new one.
	switch msg.(type) {
	case scheduleLoaded:
		r.schedule, _ = r.schedule.Update(msg)
		return r, nil
	case standingsLoadedMsg:
		r.standings, _ = r.standings.Update(msg)
		return r, nil
	case gameDetailMsg, gamePlaysMsg:
		r.detail, _ = r.detail.Update(msg)
		return r, nil
	case teamLoadedMsg:
		r.teams, _ = r.teams.Update(msg)
		return r, nil
	case playerLoadedMsg:
		r.player, _ = r.player.Update(msg)
		return r, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "t" && r.navigationAvailable() {
		if r.current == teamview || r.current == playerview {
			return r, nil
		}
		r.teamReturn = r.current
		if r.teams.width == 0 {
			r.teams = newTeams(r.width, r.height)
		}
		r.teams.browsing, r.teams.filtering = true, false
		r.current = teamview
		return r, nil
	}

	// Auto-refresh heartbeat: re-fetch whichever view is active and wants fresh
	// data, then reschedule. A single tick lineage avoids overlapping polls.
	if _, ok := msg.(refreshTickMsg); ok {
		var cmd tea.Cmd
		switch r.current {
		case listview:
			var rollCmd tea.Cmd
			r.list, rollCmd = r.list.rolloverCmd()
			cmd = rollCmd
		case detailview:
			cmd = r.detail.refreshCmd()
		}
		if cmd != nil {
			return r, tea.Batch(cmd, refreshTick())
		}
		return r, refreshTick()
	}

	// Day navigation messages always belong to the list, even if the user has
	// since opened the detail view.
	switch msg.(type) {
	case gamesLoadedMsg, daySettledMsg:
		updated, cmd := r.list.Update(msg)
		r.list = updated.(gamelist)
		return r, cmd
	}

	switch r.current {
	case listview:
		// Enter opens the selected game; "s" opens standings; "q" quits.
		// All are ignored while the list is filtering or the date picker is
		// open, so typing never triggers navigation.
		if key, ok := msg.(tea.KeyPressMsg); ok &&
			!r.list.filtering && !r.list.entering {
			switch key.String() {
			case "q":
				return r, tea.Quit
			case "a":
				r.schedule = newSchedule(r.width, r.height)
				r.scheduleReturn = listview
				r.current = scheduleview
				return r, r.schedule.Init()
			case "enter":
				if sel, ok := r.list.selectedGame(); ok {
					r.detail = newDetail(sel, r.width, r.height)
					r.detailReturn = listview
					r.current = detailview
					return r, r.detail.Init() // kick off the async fetch
				}
			case "s":
				r.standings = newStandings(r.width, r.height)
				r.current = standingsview
				return r, r.standings.Init()
			}
		}

		updated, cmd := r.list.Update(msg)
		r.list = updated.(gamelist)
		return r, cmd

	case detailview:
		if key, ok := msg.(tea.KeyPressMsg); ok {
			if r.detail.sortMode || r.detail.fullPbp {
				var cmd tea.Cmd
				r.detail, cmd = r.detail.Update(msg)
				return r, cmd
			}
			switch key.String() {
			case "esc", "q":
				r.current = r.detailReturn
				return r, nil
			}
		}

		var cmd tea.Cmd
		r.detail, cmd = r.detail.Update(msg)
		return r, cmd

	case standingsview:
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "esc", "q":
				r.current = listview
				return r, nil
			}
		}

		var cmd tea.Cmd
		r.standings, cmd = r.standings.Update(msg)
		return r, cmd
	case teamview:
		if _, ok := msg.(tea.KeyPressMsg); ok && r.teams.sortMode {
			var cmd tea.Cmd
			r.teams, cmd = r.teams.Update(msg)
			return r, cmd
		}
		if key, ok := msg.(tea.KeyPressMsg); ok && !r.teams.filtering {
			switch key.String() {
			case "q":
				if !r.teams.browsing {
					r.current = r.teamReturn
				} else if r.teams.filter != "" {
					r.teams.filter = ""
					r.teams.choice = 0
				} else {
					r.current = r.teamReturn
				}
				return r, nil
			case "esc":
				if !r.teams.browsing {
					r.teams.browsing = true
				} else if r.teams.filter != "" {
					r.teams.filter = ""
					r.teams.choice = 0
				} else {
					r.current = r.teamReturn
				}
				return r, nil
			case "a":
				if !r.teams.browsing {
					r.schedule = newSchedule(r.width, r.height)
					words := strings.Fields(r.teams.data.Team.Name)
					r.schedule.filter = words[len(words)-1]
					r.scheduleReturn = teamview
					r.current = scheduleview
					return r, r.schedule.Init()
				}
			case "enter":
				if p, ok := r.teams.SelectedPlayer(); ok && p.ID > 0 {
					r.playerRequest++
					r.player = newPlayer(p, r.teams.data.Team, r.width, r.height, r.playerRequest)
					r.current = playerview
					return r, r.player.Init()
				}
			}
		}
		var cmd tea.Cmd
		r.teams, cmd = r.teams.Update(msg)
		return r, cmd
	case playerview:
		if _, ok := msg.(tea.KeyPressMsg); ok && r.player.sortMode {
			var cmd tea.Cmd
			r.player, cmd = r.player.Update(msg)
			return r, cmd
		}
		if key, ok := msg.(tea.KeyPressMsg); ok && (key.String() == "esc" || key.String() == "q") {
			r.current = teamview
			return r, nil
		}
		var cmd tea.Cmd
		r.player, cmd = r.player.Update(msg)
		r.playerRequest = max(r.playerRequest, r.player.request)
		return r, cmd

	case scheduleview:
		if key, ok := msg.(tea.KeyPressMsg); ok {
			if !r.schedule.filtering && (key.String() == "q" || (key.String() == "esc" && r.schedule.filter == "")) {
				r.current = r.scheduleReturn
				return r, nil
			}
			if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "enter" && !r.schedule.filtering {
				if game, found := r.schedule.SelectedGame(); found {
					r.detail = newDetail(game, r.width, r.height)
					r.detailReturn = scheduleview
					r.current = detailview
					return r, r.detail.Init()
				}
			}
		}
		var cmd tea.Cmd
		r.schedule, cmd = r.schedule.Update(msg)
		return r, cmd
	}

	return r, nil
}

func (r root) View() tea.View {
	switch r.current {
	case detailview:
		return r.detail.View()
	case standingsview:
		return r.standings.View()
	case scheduleview:
		return r.schedule.View()
	case teamview:
		return r.teams.View()
	case playerview:
		return r.player.View()
	default:
		return r.list.View()
	}
}

// Top-level shortcuts yield to text inputs and the box-score sort controls.
func (r root) navigationAvailable() bool {
	switch r.current {
	case listview:
		return !r.list.entering && !r.list.filtering
	case scheduleview:
		return !r.schedule.filtering
	case teamview:
		return !r.teams.filtering && !r.teams.sortMode
	case playerview:
		return !r.player.sortMode
	case detailview:
		return !r.detail.sortMode
	}
	return true
}
