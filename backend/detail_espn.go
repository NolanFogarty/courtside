package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	nba "github.com/NolanFogarty/nba-sdk"
)

// GetGameBoxScoreForGame retains the NBA identity and only uses ESPN after NBA
// feeds fail. Matching requires the scheduled date and both home/away teams.
func GetGameBoxScoreForGame(g Game) (GameDetail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	primary, stop := context.WithTimeout(ctx, 5*time.Second)
	d, err := fetchBoxScore(nba.NewClient(nba.WithRetry(0, 0)), primary, g.GameId)
	stop()
	if err == nil {
		return d, nil
	}
	fallback, fallbackErr := fetchESPNDetail(ctx, http.DefaultClient, "https://site.api.espn.com/apis/site/v2/sports/basketball/nba/", g)
	if fallbackErr != nil {
		return GameDetail{}, fmt.Errorf("%w; ESPN fallback: %v", err, fallbackErr)
	}
	return fallback, nil
}

type detailESPNTeam struct{ ID, Name, Abbreviation string }
type detailCompetitor struct {
	HomeAway, Score string
	Team            detailESPNTeam
}
type detailCompetition struct {
	Date        string
	Competitors []detailCompetitor
}
type detailEvent struct {
	ID, Date     string
	Competitions []detailCompetition
}
type detailSummary struct {
	Header   detailEvent
	Boxscore struct {
		Teams []struct {
			Team       detailESPNTeam
			Statistics []struct{ Name, DisplayValue string }
		}
		Players []struct {
			Team       detailESPNTeam
			Statistics []struct {
				Names    []string
				Athletes []struct {
					Athlete             struct{ ShortName, DisplayName string }
					Starter, DidNotPlay bool
					Stats               []string
				}
			}
		}
	}
	Plays []struct {
		Text   string
		Period struct{ Number int }
		Clock  struct{ DisplayValue string }
		Team   struct{ ID string }
	}
}

func detailJSON(ctx context.Context, client *http.Client, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ESPN returned %s", resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out)
}

func matchesDetailEvent(e detailEvent, g Game, start time.Time) bool {
	if len(e.Competitions) != 1 {
		return false
	}
	date := e.Date
	if date == "" {
		date = e.Competitions[0].Date
	}
	tip, err := time.Parse(time.RFC3339, date)
	if err != nil {
		tip, err = time.Parse("2006-01-02T15:04Z07:00", date)
	}
	if err != nil || !tip.Equal(start) {
		return false
	}
	home, away := false, false
	for _, c := range e.Competitions[0].Competitors {
		if c.HomeAway == "home" && strings.EqualFold(c.Team.Name, g.HomeTeam) {
			home = true
		}
		if c.HomeAway == "away" && strings.EqualFold(c.Team.Name, g.AwayTeam) {
			away = true
		}
	}
	return home && away
}

func fetchESPNDetail(ctx context.Context, client *http.Client, base string, g Game) (GameDetail, error) {
	start, err := time.Parse(time.RFC3339, g.StartUTC)
	if err != nil {
		return GameDetail{}, fmt.Errorf("missing valid game tip-off time")
	}
	eastern, err := time.LoadLocation("America/New_York")
	if err != nil {
		return GameDetail{}, err
	}
	var board struct{ Events []detailEvent }
	if err := detailJSON(ctx, client, base+"scoreboard?dates="+start.In(eastern).Format("20060102"), &board); err != nil {
		return GameDetail{}, err
	}
	id := ""
	for _, e := range board.Events {
		if matchesDetailEvent(e, g, start) {
			if id != "" {
				return GameDetail{}, fmt.Errorf("ambiguous ESPN game match")
			}
			id = e.ID
		}
	}
	if id == "" {
		return GameDetail{}, fmt.Errorf("no matching ESPN game")
	}
	var summary detailSummary
	if err := detailJSON(ctx, client, base+"summary?event="+url.QueryEscape(id), &summary); err != nil {
		return GameDetail{}, err
	}
	if summary.Header.ID != id || !matchesDetailEvent(summary.Header, g, start) {
		return GameDetail{}, fmt.Errorf("ESPN summary does not match selected game")
	}
	return projectESPNDetail(summary)
}

func detailInt(s string) int { n, _ := strconv.Atoi(s); return n }
func detailPair(s string) (int, int) {
	a, b, ok := strings.Cut(s, "-")
	if !ok {
		return 0, 0
	}
	return detailInt(a), detailInt(b)
}

func projectESPNDetail(s detailSummary) (GameDetail, error) {
	d := GameDetail{Source: "ESPN"}
	teams := map[string]*TeamDetail{}
	for _, c := range s.Header.Competitions[0].Competitors {
		var t *TeamDetail
		switch c.HomeAway {
		case "home":
			t = &d.Home
		case "away":
			t = &d.Away
		default:
			continue
		}
		t.Name, t.Tricode, t.Score = c.Team.Name, c.Team.Abbreviation, detailInt(c.Score)
		teams[c.Team.ID] = t
	}
	for _, team := range s.Boxscore.Teams {
		t := teams[team.Team.ID]
		if t == nil {
			continue
		}
		for _, stat := range team.Statistics {
			n := detailInt(stat.DisplayValue)
			switch stat.Name {
			case "fieldGoalsMade-fieldGoalsAttempted":
				t.FGM, t.FGA = detailPair(stat.DisplayValue)
			case "threePointFieldGoalsMade-threePointFieldGoalsAttempted":
				t.TPM, t.TPA = detailPair(stat.DisplayValue)
			case "freeThrowsMade-freeThrowsAttempted":
				t.FTM, t.FTA = detailPair(stat.DisplayValue)
			case "totalRebounds":
				t.Reb = n
			case "offensiveRebounds":
				t.OReb = n
			case "defensiveRebounds":
				t.DReb = n
			case "assists":
				t.Ast = n
			case "steals":
				t.Stl = n
			case "blocks":
				t.Blk = n
			case "turnovers":
				t.To = n
			case "fouls":
				t.PF = n
			}
		}
		t.FGPct, t.FG3Pct = pct(t.FGM, t.FGA), pct(t.TPM, t.TPA)
	}
	for _, team := range s.Boxscore.Players {
		t := teams[team.Team.ID]
		if t == nil {
			continue
		}
		for _, group := range team.Statistics {
			for _, a := range group.Athletes {
				// ESPN athlete IDs are NOT NBA person IDs. Leave PersonID unset.
				p := PlayerLine{Name: a.Athlete.ShortName, Starter: a.Starter, Min: "0"}
				if p.Name == "" {
					p.Name = a.Athlete.DisplayName
				}
				for i, label := range group.Names {
					if i >= len(a.Stats) || a.DidNotPlay {
						continue
					}
					v := a.Stats[i]
					n := detailInt(v)
					switch label {
					case "MIN":
						p.Min = fmtMinutes(v)
					case "PTS":
						p.Pts = n
					case "FG":
						p.FGM, p.FGA = detailPair(v)
					case "3PT":
						p.TPM, p.TPA = detailPair(v)
					case "FT":
						p.FTM, p.FTA = detailPair(v)
					case "REB":
						p.Reb = n
					case "AST":
						p.Ast = n
					case "TO":
						p.To = n
					case "STL":
						p.Stl = n
					case "BLK":
						p.Blk = n
					case "OREB":
						p.OReb = n
					case "DREB":
						p.DReb = n
					case "PF":
						p.PF = n
					case "+/-":
						p.PlusMinus = n
					}
				}
				t.Players = append(t.Players, p)
			}
		}
	}
	if len(d.Home.Players) == 0 || len(d.Away.Players) == 0 {
		return GameDetail{}, fmt.Errorf("ESPN returned no player data")
	}
	d.Plays = playLines(len(s.Plays), func(i int) (int, string, string, string) {
		p := s.Plays[i]
		code := ""
		if t := teams[p.Team.ID]; t != nil {
			code = t.Tricode
		}
		return p.Period.Number, p.Clock.DisplayValue, code, p.Text
	})
	return d, nil
}
