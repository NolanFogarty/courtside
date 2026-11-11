package backend

import (
	"context"
	"fmt"
	"time"

	nba "github.com/NolanFogarty/nba-sdk"
	"github.com/NolanFogarty/nba-sdk/live"
)

// gameStatus values returned by the NBA scoreboards.
const (
	statusUpcoming = 1
	statusLive     = 2
	statusFinal    = 3
)

func parseGameClock(raw string) string {
	var mins int
	var secs float64
	if _, err := fmt.Sscanf(raw, "PT%dM%fS", &mins, &secs); err == nil {
		return fmt.Sprintf("%02d:%02d", mins, int(secs))
	}
	if _, err := fmt.Sscanf(raw, "PT%dM", &mins); err == nil {
		return fmt.Sprintf("%02d:00", mins)
	}
	return raw
}

func periodLabel(period int) string {
	if period > 4 {
		if period == 5 {
			return "OT"
		}
		return fmt.Sprintf("OT%d", period-4)
	}
	return fmt.Sprintf("Q%d", period)
}

// formatTipLocal renders a tip-off time in the viewer's local time zone.
func formatTipLocal(t time.Time) string {
	lt := t.Local()
	abbrev, _ := lt.Zone()
	return lt.Format("3:04 PM") + " " + abbrev
}

type Game struct {
	GameId    string
	StartUTC  string
	HomeTeam  string
	AwayTeam  string
	GameClock string
	HomeScore int
	AwayScore int
	status    int // 1 = scheduled, 2 = live, 3 = final
}

func GameFromSeasonSchedule(g SeasonGame) Game {
	clock := g.Status
	if g.GameStatus == statusUpcoming || g.GameStatus == 0 {
		if t, err := time.Parse(time.RFC3339, g.DateTimeUTC); err == nil {
			clock = formatTipLocal(t)
		} else if clock == "" {
			clock = g.Time
		}
	}
	status := g.GameStatus
	if status != statusUpcoming && status != statusLive && status != statusFinal {
		status = statusUpcoming
	}
	return Game{GameId: g.GameID, StartUTC: g.DateTimeUTC, AwayTeam: g.Away, HomeTeam: g.Home, AwayScore: g.AwayScore, HomeScore: g.HomeScore, GameClock: clock, status: status}
}

// NotStarted reports whether the game is scheduled but hasn't tipped off yet
// (so there's no box score or play-by-play to fetch). GameClock holds the
// tip-off time in this state. Unknown statuses are treated as not started so
// we don't fire a box-score fetch for a game that may not exist yet.
func (g Game) NotStarted() bool {
	return g.status == statusUpcoming || g.status == 0
}

// IsLive reports whether the game is currently in progress, in which case its
// data is worth auto-refreshing.
func (g Game) IsLive() bool {
	return g.status == statusLive
}

// IsFinal reports whether the game is over and its score is official.
func (g Game) IsFinal() bool {
	return g.status == statusFinal
}

// toGame converts an SDK scoreboard game into our internal Game, deriving the
// display clock from the game's status. Upcoming tip-offs render in the
// viewer's local time zone.
func toGame(g live.Game) Game {
	var clock string
	switch g.GameStatus {
	case statusLive:
		clock = fmt.Sprintf("%s %s", periodLabel(g.Period), parseGameClock(g.GameClock))
	case statusFinal:
		clock = "Final"
	default:
		if t, err := time.Parse(time.RFC3339, g.GameTimeUTC); err == nil {
			clock = formatTipLocal(t)
		} else {
			clock = g.GameStatusText
		}
	}

	return Game{
		GameId:    g.GameID,
		StartUTC:  g.GameTimeUTC,
		HomeTeam:  g.HomeTeam.TeamName,
		AwayTeam:  g.AwayTeam.TeamName,
		GameClock: clock,
		HomeScore: g.HomeTeam.Score,
		AwayScore: g.AwayTeam.Score,
		status:    g.GameStatus,
	}
}

// todaysGames fetches today's scoreboard and returns only the games whose
// status passes keep.
func todaysGames(keep func(status int) bool) ([]Game, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := nba.NewClient()
	resp, err := client.Stats.ScoreboardV3(ctx, todayLocal())
	if err != nil {
		return nil, err
	}

	games := make([]Game, 0, len(resp.Scoreboard.Games))
	for _, g := range resp.Scoreboard.Games {
		if keep(g.GameStatus) {
			games = append(games, scoreboardGame(g))
		}
	}
	return games, nil
}

// GetScheduledGames returns today's games that are not currently in progress
// (upcoming and finished).
func GetScheduledGames() ([]Game, error) {
	return todaysGames(func(status int) bool {
		return status != statusLive
	})
}

// GetLiveGames returns today's games that are currently in progress.
func GetLiveGames() ([]Game, error) {
	return todaysGames(func(status int) bool {
		return status == statusLive
	})
}

// GetTodaysGames returns every game on today's scoreboard (upcoming,
// in-progress, and final) — the same source used at startup. GetGamesForDate
// routes today's date here so the navigated "today" view matches the initial
// one instead of using the staler scoreboardv2 schedule.
func GetTodaysGames() ([]Game, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := nba.NewClient()
	resp, err := client.Stats.ScoreboardV3(ctx, todayLocal())
	if err != nil {
		return nil, err
	}
	games := make([]Game, 0, len(resp.Scoreboard.Games))
	for _, g := range resp.Scoreboard.Games {
		games = append(games, scoreboardGame(g))
	}
	return games, nil
}
