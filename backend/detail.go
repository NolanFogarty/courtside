package backend

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	nba "github.com/NolanFogarty/nba-sdk"
	"github.com/NolanFogarty/nba-sdk/live"
	"github.com/NolanFogarty/nba-sdk/stats"
)

// PlayerLine is one player's box-score line.
type PlayerLine struct {
	Name     string
	Min      string // minutes played
	Starter  bool
	PersonID int

	Pts, Ast, Reb, Blk, To int
	PlusMinus              int

	// expanded detail
	FGM, FGA, TPM, TPA, FTM, FTA int
	OReb, DReb, Stl, PF          int
}

// TeamDetail is one team's full box score plus the aggregate stats shown in the
// comparison bar.
type TeamDetail struct {
	Name, Tricode string
	Score         int
	Players       []PlayerLine

	FGPct, FG3Pct float64 // shooting percentages, 0-100
	FTM, FTA      int     // free throws made / attempted
	Reb, Ast, To  int

	// expanded detail
	FGM, FGA, TPM, TPA       int
	OReb, DReb, Stl, Blk, PF int
}

// PlayLine is a single play-by-play event. Team is the tricode, empty for
// neutral events (timeouts, period boundaries, etc.).
type PlayLine struct {
	Period int
	Clock  string // "MM:SS"
	Team   string
	Desc   string
}

// GameDetail is everything the detail screen needs for one game.
type GameDetail struct {
	Away, Home TeamDetail
	Plays      []PlayLine // newest first
}

// GetGameDetail fetches a game's box score and play-by-play and projects them
// into a GameDetail. It prefers the live CDN feeds (cdn.nba.com), which serve
// in-progress and recently finished games reliably, and falls back to the
// stats.nba.com v3 endpoints for older games (the live files are eventually
// removed). stats.nba.com is known to return empty/5xx for live games, so the
// CDN-first order matters.
func GetGameDetail(gameID string) (GameDetail, error) {
	client := nba.NewClient()
	ctx := context.Background()

	d, err := fetchBoxScore(client, ctx, gameID)
	if err != nil {
		return GameDetail{}, err
	}

	// Play-by-play is best-effort; an unavailable feed shouldn't block the box
	// score. The next refresh retries.
	d.Plays = fetchPlayByPlay(client, ctx, gameID)
	return d, nil
}

// fetchBoxScore returns the box score, preferring the live CDN feed and falling
// back to the stats.nba.com traditional box score.
func fetchBoxScore(client *nba.Client, ctx context.Context, gameID string) (GameDetail, error) {
	if box, err := client.Live.BoxScore(ctx, gameID); err == nil {
		g := box.Game
		if len(g.HomeTeam.Players) > 0 || len(g.AwayTeam.Players) > 0 {
			detail := GameDetail{
				Away: liveTeamDetail(g.AwayTeam),
				Home: liveTeamDetail(g.HomeTeam),
			}
			if !hasStarters(detail.Away) || !hasStarters(detail.Home) {
				if statsBox, statsErr := client.Stats.BoxScoreTraditionalV3(ctx, gameID); statsErr == nil {
					applyStarterFlags(&detail.Away, statsBox.BoxScoreTraditional.AwayTeam)
					applyStarterFlags(&detail.Home, statsBox.BoxScoreTraditional.HomeTeam)
				}
			}
			inferStartersByMinutes(&detail.Away)
			inferStartersByMinutes(&detail.Home)
			return detail, nil
		}
	}

	box, err := client.Stats.BoxScoreTraditionalV3(ctx, gameID)
	if err != nil {
		return GameDetail{}, err
	}
	return GameDetail{
		Away: toTeamDetail(box.BoxScoreTraditional.AwayTeam),
		Home: toTeamDetail(box.BoxScoreTraditional.HomeTeam),
	}, nil
}

// fetchPlayByPlay returns the play-by-play newest-first, preferring the live CDN
// feed and falling back to stats.nba.com. Returns nil if both are unavailable.
func fetchPlayByPlay(client *nba.Client, ctx context.Context, gameID string) []PlayLine {
	if pbp, err := client.Live.PlayByPlay(ctx, gameID); err == nil && len(pbp.Game.Actions) > 0 {
		return playLines(len(pbp.Game.Actions), func(i int) (int, string, string, string) {
			a := pbp.Game.Actions[i]
			return a.Period, a.Clock, a.TeamTricode, a.Description
		})
	}
	if pbp, err := client.Stats.PlayByPlayV3(ctx, gameID); err == nil {
		return playLines(len(pbp.Game.Actions), func(i int) (int, string, string, string) {
			a := pbp.Game.Actions[i]
			return a.Period, a.Clock, a.TeamTricode, a.Description
		})
	}
	return nil
}

// playLines projects n play-by-play actions (oldest-first) into newest-first
// PlayLines using the supplied accessor.
func playLines(n int, at func(i int) (period int, clock, team, desc string)) []PlayLine {
	out := make([]PlayLine, 0, n)
	for i := n - 1; i >= 0; i-- {
		period, clock, team, desc := at(i)
		out = append(out, PlayLine{
			Period: period,
			Clock:  parseGameClock(clock),
			Team:   team,
			Desc:   desc,
		})
	}
	return out
}

func toTeamDetail(t stats.BoxTeam) TeamDetail {
	s := t.Statistics
	td := TeamDetail{
		Name:    t.TeamName,
		Tricode: t.TeamTricode,
		Score:   s.Points,
		FGPct:   pct(s.FieldGoalsMade, s.FieldGoalsAttempted),
		FG3Pct:  pct(s.ThreePointersMade, s.ThreePointersAttempted),
		FTM:     s.FreeThrowsMade,
		FTA:     s.FreeThrowsAttempted,
		Reb:     s.ReboundsTotal,
		Ast:     s.Assists,
		To:      s.Turnovers,
		FGM:     s.FieldGoalsMade,
		FGA:     s.FieldGoalsAttempted,
		TPM:     s.ThreePointersMade,
		TPA:     s.ThreePointersAttempted,
		OReb:    s.ReboundsOffensive,
		DReb:    s.ReboundsDefensive,
		Stl:     s.Steals,
		Blk:     s.Blocks,
		PF:      s.FoulsPersonal,
	}

	for _, p := range t.Players {
		name := p.NameI
		if name == "" {
			name = p.FamilyName
		}
		ps := p.Statistics
		td.Players = append(td.Players, PlayerLine{
			Name:      name,
			Starter:   p.Starter,
			PersonID:  p.PersonID,
			Min:       fmtMinutes(ps.Minutes),
			Pts:       ps.Points,
			Ast:       ps.Assists,
			Reb:       ps.ReboundsTotal,
			Blk:       ps.Blocks,
			To:        ps.Turnovers,
			PlusMinus: int(ps.PlusMinusPoints),
			FGM:       ps.FieldGoalsMade,
			FGA:       ps.FieldGoalsAttempted,
			TPM:       ps.ThreePointersMade,
			TPA:       ps.ThreePointersAttempted,
			FTM:       ps.FreeThrowsMade,
			FTA:       ps.FreeThrowsAttempted,
			OReb:      ps.ReboundsOffensive,
			DReb:      ps.ReboundsDefensive,
			Stl:       ps.Steals,
			PF:        ps.FoulsPersonal,
		})
	}
	return td
}

// liveTeamDetail projects a live CDN box-score team into a TeamDetail. The live
// feed lists the whole roster (including DNPs); the view dims players without a
// stat line.
func liveTeamDetail(t live.BoxScoreTeam) TeamDetail {
	s := t.Statistics
	td := TeamDetail{
		Name:    t.TeamName,
		Tricode: t.TeamTricode,
		Score:   t.Score,
		FGPct:   pct(s.FieldGoalsMade, s.FieldGoalsAttempted),
		FG3Pct:  pct(s.ThreePointersMade, s.ThreePointersAttempted),
		FTM:     s.FreeThrowsMade,
		FTA:     s.FreeThrowsAttempted,
		Reb:     s.ReboundsTotal,
		Ast:     s.Assists,
		To:      s.Turnovers,
		FGM:     s.FieldGoalsMade,
		FGA:     s.FieldGoalsAttempted,
		TPM:     s.ThreePointersMade,
		TPA:     s.ThreePointersAttempted,
		OReb:    s.ReboundsOffensive,
		DReb:    s.ReboundsDefensive,
		Stl:     s.Steals,
		Blk:     s.Blocks,
		PF:      s.FoulsPersonal,
	}

	for _, p := range t.Players {
		name := p.NameI
		if name == "" {
			name = p.FamilyName
		}
		ps := p.Statistics
		td.Players = append(td.Players, PlayerLine{
			Name:      name,
			Starter:   p.Starter == "1",
			PersonID:  p.PersonID,
			Min:       fmtMinutes(ps.Minutes),
			Pts:       ps.Points,
			Ast:       ps.Assists,
			Reb:       ps.ReboundsTotal,
			Blk:       ps.Blocks,
			To:        ps.Turnovers,
			PlusMinus: int(ps.PlusMinusPoints),
			FGM:       ps.FieldGoalsMade,
			FGA:       ps.FieldGoalsAttempted,
			TPM:       ps.ThreePointersMade,
			TPA:       ps.ThreePointersAttempted,
			FTM:       ps.FreeThrowsMade,
			FTA:       ps.FreeThrowsAttempted,
			OReb:      ps.ReboundsOffensive,
			DReb:      ps.ReboundsDefensive,
			Stl:       ps.Steals,
			PF:        ps.FoulsPersonal,
		})
	}
	return td
}

func hasStarters(team TeamDetail) bool {
	for _, player := range team.Players {
		if player.Starter {
			return true
		}
	}
	return false
}

func applyStarterFlags(team *TeamDetail, source stats.BoxTeam) {
	starters := make(map[int]bool)
	count := 0
	for _, player := range source.Players {
		if player.Starter {
			starters[player.PersonID] = true
			count++
		}
	}
	if count == 0 {
		return
	}
	for i := range team.Players {
		team.Players[i].Starter = starters[team.Players[i].PersonID]
	}
}

func inferStartersByMinutes(team *TeamDetail) {
	count := 0
	scored := false
	for _, player := range team.Players {
		if player.Starter {
			count++
		}
		if minuteValue(player.Min) > 0 {
			scored = true
		}
	}
	if count == 5 || len(team.Players) < 5 {
		return
	}
	if !scored {
		return
	}

	indices := make([]int, len(team.Players))
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(i, j int) bool {
		return minuteValue(team.Players[indices[i]].Min) > minuteValue(team.Players[indices[j]].Min)
	})
	for i := range team.Players {
		team.Players[i].Starter = false
	}
	for _, i := range indices[:5] {
		team.Players[i].Starter = true
	}
}

func minuteValue(raw string) int {
	minutes, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return minutes
}

// fmtMinutes extracts whole minutes from the box score's minutes field, which
// may be ISO ("PT34M12.00S") or "MM:SS".
func fmtMinutes(raw string) string {
	if raw == "" {
		return "0"
	}
	var m int
	if _, err := fmt.Sscanf(raw, "PT%dM", &m); err == nil {
		return strconv.Itoa(m)
	}
	if i := strings.IndexByte(raw, ':'); i >= 0 {
		return raw[:i]
	}
	return raw
}

// pct returns made/attempted as a 0-100 percentage, or 0 when none attempted.
func pct(made, attempted int) float64 {
	if attempted == 0 {
		return 0
	}
	return float64(made) / float64(attempted) * 100
}
