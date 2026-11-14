package backend

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	nba "github.com/NolanFogarty/nba-sdk"
)

// Standing is one team's row in the conference standings.
type Standing struct {
	Rank         int
	Team         string // nickname
	Wins         int
	Losses       int
	PCT          float64
	GamesBack    float64
	L10          string
	Streak       string
	Clinch       string // clinch/elimination marker (x, y, e, …), "" if none
	ConfRecord   string
	DivRecord    string
	HomeRecord   string
	RoadRecord   string
	PointsPG     float64
	OppPointsPG  float64
	DiffPointsPG float64
}

// Standings holds the league standings split by conference, each sorted by
// playoff rank.
type Standings struct {
	Season string
	East   []Standing
	West   []Standing
}

// GetStandings fetches the current regular-season standings.
func GetStandings() (Standings, error) {
	season := currentSeason()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := nba.NewClient()
	resp, err := client.Stats.LeagueStandingsV3(ctx, season)
	if err != nil {
		return Standings{}, err
	}

	out := Standings{Season: season}
	for _, t := range resp.Standings {
		s := Standing{
			Rank:         t.PlayoffRank,
			Team:         t.TeamName,
			Wins:         t.Wins,
			Losses:       t.Losses,
			PCT:          t.WinPCT,
			GamesBack:    t.ConferenceGamesBack,
			L10:          t.L10,
			Streak:       t.StrCurrentStreak,
			Clinch:       clinchMark(t.ClinchIndicator),
			ConfRecord:   t.ConferenceRecord,
			DivRecord:    t.DivisionRecord,
			HomeRecord:   t.Home,
			RoadRecord:   t.Road,
			PointsPG:     t.PointsPG,
			OppPointsPG:  t.OppPointsPG,
			DiffPointsPG: t.DiffPointsPG,
		}
		switch strings.ToLower(strings.TrimSpace(t.Conference)) {
		case "east":
			out.East = append(out.East, s)
		case "west":
			out.West = append(out.West, s)
		default:
			// Don't silently drop teams with unexpected conference labels.
			if len(out.East) <= len(out.West) {
				out.East = append(out.East, s)
			} else {
				out.West = append(out.West, s)
			}
		}
	}

	sort.SliceStable(out.East, func(i, j int) bool { return out.East[i].Rank < out.East[j].Rank })
	sort.SliceStable(out.West, func(i, j int) bool { return out.West[i].Rank < out.West[j].Rank })
	return out, nil
}

// clinchMark reduces an NBA clinch indicator (e.g. " - x", " - e") to its
// short marker ("x", "e"), or "" if there is none.
func clinchMark(indicator string) string {
	for _, f := range strings.Fields(indicator) {
		if f == "-" || f == "" {
			continue
		}
		// Return the last meaningful token; indicators are short markers.
		if len(f) <= 3 {
			return f
		}
	}
	return ""
}

// currentSeason returns the NBA season string ("YYYY-YY") for today in the
// viewer's local time zone. The preseason starts in September, so September
// rolls to the new season.
func currentSeason() string {
	now := time.Now().Local()
	year := now.Year()
	if now.Month() < time.September {
		// e.g. May 2026 → "2025-26"
		return fmt.Sprintf("%d-%02d", year-1, year%100)
	}
	// e.g. September 2026 → "2026-27"
	return fmt.Sprintf("%d-%02d", year, (year+1)%100)
}
