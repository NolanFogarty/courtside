package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

type Injury struct {
	PlayerID                                      int
	Name, Status, Date, Comment, Kind, ReturnDate string
}
type supplementalPlayer struct{ Salary *int64 }
type supplementalRoster struct{ Players map[string]supplementalPlayer }

func (c profileClient) supplementalRoster(ctx context.Context, team Team, season string) (supplementalRoster, error) {
	out := supplementalRoster{Players: map[string]supplementalPlayer{}}
	resp, err := c.get(ctx, c.espnURL+"teams/"+team.ESPN+"/roster")
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	var data struct {
		Athletes []struct {
			Name      string `json:"displayName"`
			Contracts []struct {
				Salary *int64 `json:"salary"`
				Season struct {
					Year int `json:"year"`
				} `json:"season"`
			} `json:"contracts"`
		} `json:"athletes"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&data); err != nil {
		return out, err
	}
	if data.Athletes == nil {
		return out, fmt.Errorf("salary provider returned no roster")
	}
	if len(season) < 4 {
		return out, fmt.Errorf("invalid season %q", season)
	}
	start, err := strconv.Atoi(season[:4])
	if err != nil {
		return out, err
	}
	for _, p := range data.Athletes {
		s := supplementalPlayer{}
		var best *int64
		for _, contract := range p.Contracts {
			if contract.Season.Year == start+1 && contract.Salary != nil && *contract.Salary > 0 {
				if best == nil || *contract.Salary > *best {
					v := *contract.Salary
					best = &v
				}
			}
		}
		s.Salary = best
		out.Players[normalizeName(p.Name)] = s
	}
	return out, nil
}
func (c profileClient) injuries(ctx context.Context, team Team) ([]Injury, error) {
	resp, err := c.get(ctx, c.espnURL+"injuries")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var data struct {
		Teams []struct {
			Name     string `json:"displayName"`
			Injuries []struct {
				Status  string `json:"status"`
				Date    string `json:"date"`
				Comment string `json:"shortComment"`
				Athlete struct {
					Name string `json:"displayName"`
					Team struct {
						Code string `json:"abbreviation"`
					} `json:"team"`
				} `json:"athlete"`
				Details struct {
					Type       string `json:"type"`
					ReturnDate string `json:"returnDate"`
				} `json:"details"`
			} `json:"injuries"`
		} `json:"injuries"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&data); err != nil {
		return nil, err
	}
	if data.Teams == nil {
		return nil, fmt.Errorf("injury provider returned no report")
	}
	out := []Injury{}
	for _, t := range data.Teams {
		for _, i := range t.Injuries {
			if !injuryBelongsToTeam(t.Name, i.Athlete.Team.Code, team) {
				continue
			}
			out = append(out, Injury{Name: i.Athlete.Name, Status: i.Status, Date: i.Date, Comment: i.Comment, Kind: i.Details.Type, ReturnDate: i.Details.ReturnDate})
		}
	}
	return out, nil
}

func injuryBelongsToTeam(sectionName, athleteCode string, team Team) bool {
	// ESPN's injury sections are per team, so a section-name match is
	// authoritative even when the abbreviation differs.
	if normalizeName(sectionName) == normalizeName(team.Name) {
		return true
	}
	if athleteCode != "" {
		if canonicalAbbrev(athleteCode) == canonicalAbbrev(team.Code) {
			return true
		}
		return false
	}
	return false
}

// canonicalAbbrev maps ESPN's abbreviations to the NBA tricodes used in
// Team.Code. Most match exactly; these are the known divergences observed in
// the live ESPN injuries feed.
func canonicalAbbrev(code string) string {
	if alias, ok := espnAbbrevAliases[code]; ok {
		return alias
	}
	return code
}

var espnAbbrevAliases = map[string]string{
	"GS":   "GSW",
	"SA":   "SAS",
	"NO":   "NOP",
	"NY":   "NYK",
	"UTAH": "UTA",
	"WSH":  "WAS",
}
