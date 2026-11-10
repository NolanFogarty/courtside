package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	nba "github.com/NolanFogarty/nba-sdk"
	"github.com/NolanFogarty/nba-sdk/stats"
)

type SeasonGame struct {
	GameID, Date, Time, DateTimeUTC, Status, Away, Home string
	GameStatus                                          int
	AwayScore, HomeScore                                int
	AwayWins, AwayLosses, HomeWins, HomeLosses          int
}

func GetSeasonSchedule() ([]SeasonGame, error) {
	return fetchSeasonSchedule(currentSeason())
}

// scheduleStatsUA is the Windows Chrome fingerprint Akamai's bot detector
// accepts. A bare "Mozilla/5.0" UA is silently held open indefinitely, which
// surfaced as "context deadline exceeded" after 3x60s attempts.
const scheduleStatsUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

func scheduleURL(season string) string {
	return "https://stats.nba.com/stats/scheduleleaguev2?LeagueID=00&Season=" + season
}

func newScheduleRequest(ctx context.Context, rawURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", scheduleStatsUA)
	req.Header.Set("Referer", "https://www.nba.com/")
	req.Header.Set("Origin", "https://www.nba.com")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("x-nba-stats-origin", "stats")
	req.Header.Set("x-nba-stats-token", "true")
	req.Header.Set("sec-ch-ua", `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Windows"`)
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-site")
	return req, nil
}

func scheduleHTTPClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

func fetchSeasonSchedule(season string) ([]SeasonGame, error) {
	client := scheduleHTTPClient()
	ctx := context.Background()
	return fetchSeasonScheduleWithClient(ctx, client, scheduleURL(season))
}

func fetchSeasonScheduleWithClient(ctx context.Context, client *http.Client, rawURL string) ([]SeasonGame, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		games, err := fetchScheduleFromURL(attemptCtx, client, rawURL)
		cancel()
		if err == nil {
			return games, nil
		}
		lastErr = err
		if !isScheduleRetryable(err) {
			return nil, err
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(attempt+1) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil, fmt.Errorf("schedule request failed after 3 attempts: %w", lastErr)
}

func isScheduleRetryable(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	// Non-retryable client errors: bad request, auth, not found.
	for _, prefix := range []string{"schedule API returned 400", "schedule API returned 401", "schedule API returned 403", "schedule API returned 404"} {
		if strings.HasPrefix(s, prefix) {
			return false
		}
	}
	return true
}

func fetchScheduleFromURL(ctx context.Context, client *http.Client, rawURL string) ([]SeasonGame, error) {
	req, err := newScheduleRequest(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("schedule API returned %s", resp.Status)
	}
	var data struct {
		League struct {
			GameDates []struct {
				Date  string `json:"gameDate"`
				Games []struct {
					ID          string `json:"gameId"`
					Status      string `json:"gameStatusText"`
					GameStatus  int    `json:"gameStatus"`
					Date        string `json:"gameDateEst"`
					Time        string `json:"gameTimeEst"`
					DateTimeUTC string `json:"gameDateTimeUTC"`
					Away        struct {
						Name   string `json:"teamName"`
						Score  int    `json:"score"`
						Wins   int    `json:"wins"`
						Losses int    `json:"losses"`
					} `json:"awayTeam"`
					Home struct {
						Name   string `json:"teamName"`
						Score  int    `json:"score"`
						Wins   int    `json:"wins"`
						Losses int    `json:"losses"`
					} `json:"homeTeam"`
				} `json:"games"`
			} `json:"gameDates"`
		} `json:"leagueSchedule"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	games := []SeasonGame{}
	for _, day := range data.League.GameDates {
		for _, g := range day.Games {
			games = append(games, SeasonGame{GameID: g.ID, Date: g.Date, Time: g.Time, DateTimeUTC: g.DateTimeUTC, Status: g.Status, GameStatus: g.GameStatus, Away: g.Away.Name, Home: g.Home.Name, AwayScore: g.Away.Score, HomeScore: g.Home.Score, AwayWins: g.Away.Wins, AwayLosses: g.Away.Losses, HomeWins: g.Home.Wins, HomeLosses: g.Home.Losses})
		}
	}
	return games, nil
}

// GetGamesForDate returns every game on the given date ("YYYY-MM-DD") — past
// results, in-progress, and future scheduled games — from stats.nba.com's
// scoreboardv3 endpoint, which returns full team names, scores, and status
// directly. Today uses this same endpoint so the navigated "today" view matches
// the initial scoreboard.
func GetGamesForDate(date string) ([]Game, error) {
	if isTodayLocal(date) {
		return GetTodaysGames()
	}

	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := nba.NewClient()
	resp, err := client.Stats.ScoreboardV3(ctx, d)
	if err != nil {
		return nil, err
	}

	games := make([]Game, 0, len(resp.Scoreboard.Games))
	for _, g := range resp.Scoreboard.Games {
		games = append(games, scoreboardGame(g))
	}
	return games, nil
}

// scoreboardGame converts a scoreboardv3 game into our internal Game, deriving
// the display clock from the game's status. Upcoming tip-offs render in the
// viewer's local time zone.
func scoreboardGame(g stats.ScoreboardV3Game) Game {
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
			clock = strings.TrimSpace(g.GameStatusText)
		}
	}

	return Game{
		GameId:    g.GameID,
		HomeTeam:  g.HomeTeam.TeamName,
		AwayTeam:  g.AwayTeam.TeamName,
		HomeScore: g.HomeTeam.Score,
		AwayScore: g.AwayTeam.Score,
		GameClock: clock,
		status:    g.GameStatus,
	}
}

// isTodayLocal reports whether the given "YYYY-MM-DD" date is today in the
// viewer's local time zone.
func isTodayLocal(date string) bool {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return false
	}
	now := todayLocal()
	y, m, day := now.Date()
	return d.Year() == y && d.Month() == m && d.Day() == day
}

// todayLocal returns the current date in the viewer's local time zone.
func todayLocal() time.Time {
	return time.Now().Local()
}
