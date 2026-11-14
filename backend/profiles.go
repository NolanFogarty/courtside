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
	"sync"
	"time"
)

// Row addresses NBA result-set columns by name; missing values remain unknown.
type Row map[string]json.RawMessage

func (r Row) Text(key string) string {
	b := r[key]
	if len(b) == 0 || string(b) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(b, &s) == nil {
		return s
	}
	return string(b)
}
func (r Row) Number(key string) float64 { n, _ := strconv.ParseFloat(r.Text(key), 64); return n }
func (r Row) Has(key string) bool       { return r.Text(key) != "" }
func (r Row) IsNumeric(key string) bool {
	t := strings.TrimSpace(r.Text(key))
	if t == "" {
		return false
	}
	_, err := strconv.ParseFloat(t, 64)
	return err == nil
}

type resultSet struct {
	Name    string              `json:"name"`
	Headers []string            `json:"headers"`
	Rows    [][]json.RawMessage `json:"rowSet"`
}

func decodeSets(reader io.Reader) (map[string][]Row, error) {
	var response struct {
		Sets json.RawMessage `json:"resultSets"`
		Set  *resultSet      `json:"resultSet"`
	}
	if err := json.NewDecoder(reader).Decode(&response); err != nil {
		return nil, err
	}
	var sets []resultSet
	if len(response.Sets) > 0 && string(response.Sets) != "null" {
		if err := json.Unmarshal(response.Sets, &sets); err != nil {
			var single resultSet
			if err := json.Unmarshal(response.Sets, &single); err != nil {
				return nil, err
			}
			sets = append(sets, single)
		}
	}
	if response.Set != nil {
		sets = append(sets, *response.Set)
	}
	if len(sets) == 0 {
		return nil, fmt.Errorf("NBA returned no result sets")
	}
	out := make(map[string][]Row)
	for _, s := range sets {
		if s.Name == "" {
			continue
		}
		out[s.Name] = []Row{}
		for _, cells := range s.Rows {
			row := Row{}
			for i, h := range s.Headers {
				if i < len(cells) {
					row[h] = cells[i]
				}
			}
			out[s.Name] = append(out[s.Name], row)
		}
	}
	return out, nil
}

type profileClient struct {
	http              *http.Client
	statsURL, espnURL string
}

func newProfileClient() profileClient {
	return profileClient{&http.Client{Timeout: 18 * time.Second}, "https://stats.nba.com/stats/", "https://site.api.espn.com/apis/site/v2/sports/basketball/nba/"}
}
func (c profileClient) get(ctx context.Context, endpoint string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.nba.com/")
	req.Header.Set("Origin", "https://www.nba.com")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("x-nba-stats-origin", "stats")
	req.Header.Set("x-nba-stats-token", "true")
	req.Header.Set("sec-ch-ua", `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Windows"`)
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-site")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("data provider returned %s", resp.Status)
	}
	return resp, nil
}
func (c profileClient) stats(ctx context.Context, endpoint string, query url.Values) (map[string][]Row, error) {
	resp, err := c.get(ctx, c.statsURL+endpoint+"?"+query.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return decodeSets(io.LimitReader(resp.Body, 16<<20))
}
func first(rows []Row) Row {
	if len(rows) > 0 {
		return rows[0]
	}
	return Row{}
}
func profileQuery(season string) url.Values {
	return url.Values{"LeagueID": {"00"}, "Season": {season}, "SeasonType": {"Regular Season"}}
}
func dashboardQuery(season string) url.Values {
	q := profileQuery(season)
	for k, v := range map[string]string{"MeasureType": "Base", "PerMode": "PerGame", "PlusMinus": "N", "PaceAdjust": "N", "Rank": "N", "LastNGames": "0", "Month": "0", "OpponentTeamID": "0", "PORound": "0", "Period": "0", "DateFrom": "", "DateTo": "", "GameSegment": "", "Location": "", "Outcome": "", "SeasonSegment": "", "VsConference": "", "VsDivision": ""} {
		q.Set(k, v)
	}
	return q
}

// Parallel sections fail independently, so a missing feed never hides the roster.
func loadSections(jobs map[string]func() error) map[string]string {
	return loadSectionsWithContext(context.Background(), jobs)
}

func loadSectionsWithContext(ctx context.Context, jobs map[string]func() error) map[string]string {
	var mu sync.Mutex
	var wg sync.WaitGroup
	failures := map[string]string{}
	finished := map[string]bool{}
	for name, job := range jobs {
		wg.Go(func() {
			defer func() {
				mu.Lock()
				finished[name] = true
				mu.Unlock()
			}()
			if err := job(); err != nil {
				mu.Lock()
				failures[name] = err.Error()
				mu.Unlock()
			}
		})
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		mu.Lock()
		for name := range jobs {
			// Only blame the jobs still running; a section that already
			// succeeded keeps its data instead of being marked failed.
			if !finished[name] {
				if _, ok := failures[name]; !ok {
					failures[name] = ctx.Err().Error()
				}
			}
		}
		mu.Unlock()
		<-done
	}
	return failures
}

type RosterPlayer struct {
	ID                                                                              int
	Name, Number, Position, Height, Weight, BirthDate, Experience, School, Acquired string
	Age                                                                             int
	Stats                                                                           Row
	Salary                                                                          *int64
	Injury                                                                          *Injury
}
type TeamProfile struct {
	Team                        Team
	Season                      string
	Info, Ranks                 Row
	Totals                      Row
	Franchise                   map[string][]Row
	Roster                      []RosterPlayer
	Coaches, PlayerStats, Games []Row
	Injuries                    []Injury
	Errors                      map[string]string
	Updated                     time.Time
}

func GetTeamProfile(team Team) TeamProfile {
	return newProfileClient().teamProfile(team, currentSeason())
}
func (c profileClient) teamProfile(team Team, season string) TeamProfile {
	ctx, cancel := context.WithTimeout(context.Background(), 22*time.Second)
	defer cancel()
	out := TeamProfile{Team: team, Season: season}
	var supplements supplementalRoster
	var injuries []Injury
	out.Errors = loadSectionsWithContext(ctx, map[string]func() error{
		"overview": func() error {
			q := profileQuery(season)
			q.Set("TeamID", strconv.Itoa(team.ID))
			sets, e := c.stats(ctx, "teaminfocommon", q)
			if e == nil {
				out.Info = first(sets["TeamInfoCommon"])
				out.Ranks = first(sets["TeamSeasonRanks"])
			}
			return e
		},
		"franchise": func() error {
			sets, e := c.stats(ctx, "teamdetails", url.Values{"TeamID": {strconv.Itoa(team.ID)}})
			if e == nil {
				out.Franchise = sets
			}
			return e
		},
		"roster": func() error {
			q := profileQuery(season)
			q.Set("TeamID", strconv.Itoa(team.ID))
			sets, e := c.stats(ctx, "commonteamroster", q)
			if e != nil {
				return e
			}
			if _, ok := sets["CommonTeamRoster"]; !ok {
				return fmt.Errorf("missing CommonTeamRoster")
			}
			for _, r := range sets["CommonTeamRoster"] {
				id := 0
				if r.IsNumeric("PLAYER_ID") {
					id = int(r.Number("PLAYER_ID"))
				}
				age := 0
				if r.IsNumeric("AGE") {
					age = int(r.Number("AGE"))
				}
				out.Roster = append(out.Roster, RosterPlayer{ID: id, Name: r.Text("PLAYER"), Number: r.Text("NUM"), Position: r.Text("POSITION"), Height: r.Text("HEIGHT"), Weight: r.Text("WEIGHT"), BirthDate: r.Text("BIRTH_DATE"), Age: age, Experience: r.Text("EXP"), School: r.Text("SCHOOL"), Acquired: r.Text("HOW_ACQUIRED")})
			}
			out.Coaches = sets["Coaches"]
			return nil
		},
		"stats": func() error {
			q := dashboardQuery(season)
			q.Set("TeamID", strconv.Itoa(team.ID))
			sets, e := c.stats(ctx, "teamplayerdashboard", q)
			if e != nil {
				return e
			}
			if _, ok := sets["PlayersSeasonTotals"]; !ok {
				return fmt.Errorf("missing PlayersSeasonTotals")
			}
			out.PlayerStats = sets["PlayersSeasonTotals"]
			out.Totals = first(sets["TeamOverall"])
			return nil
		},
		"games": func() error {
			q := profileQuery(season)
			q.Set("TeamID", strconv.Itoa(team.ID))
			sets, e := c.stats(ctx, "teamgamelog", q)
			if e != nil {
				return e
			}
			if _, ok := sets["TeamGameLog"]; !ok {
				return fmt.Errorf("missing TeamGameLog")
			}
			out.Games = sets["TeamGameLog"]
			return nil
		},
		"salaries": func() error { var e error; supplements, e = c.supplementalRoster(ctx, team, season); return e },
		"injuries": func() error { var e error; injuries, e = c.injuries(ctx, team); return e },
	})
	statsByID := map[int]Row{}
	for _, r := range out.PlayerStats {
		if !r.IsNumeric("PLAYER_ID") {
			continue
		}
		id := int(r.Number("PLAYER_ID"))
		if id == 0 {
			continue
		}
		statsByID[id] = r
	}
	for i := range out.Roster {
		p := &out.Roster[i]
		p.Stats = statsByID[p.ID]
		if s, ok := supplements.Players[normalizeName(p.Name)]; ok {
			p.Salary = s.Salary
		}
		for j := range injuries {
			if normalizeName(injuries[j].Name) == normalizeName(p.Name) {
				injuries[j].PlayerID = p.ID
				p.Injury = &injuries[j]
				break
			}
		}
	}
	out.Injuries = injuries
	out.Updated = time.Now()
	return out
}

type PlayerProfile struct {
	ID                                      int
	Season                                  string
	Info                                    Row
	Seasons, Career, Playoffs, Games, Highs []Row
	Errors                                  map[string]string
	Updated                                 time.Time
}

func GetPlayerProfile(id int) PlayerProfile {
	return newProfileClient().playerProfile(id, currentSeason())
}
func (c profileClient) playerProfile(id int, season string) PlayerProfile {
	ctx, cancel := context.WithTimeout(context.Background(), 22*time.Second)
	defer cancel()
	out := PlayerProfile{ID: id, Season: season}
	out.Errors = loadSectionsWithContext(ctx, map[string]func() error{
		"bio": func() error {
			sets, e := c.stats(ctx, "commonplayerinfo", url.Values{"PlayerID": {strconv.Itoa(id)}, "LeagueID": {"00"}})
			if e != nil {
				return e
			}
			if _, ok := sets["CommonPlayerInfo"]; !ok {
				return fmt.Errorf("missing CommonPlayerInfo")
			}
			out.Info = first(sets["CommonPlayerInfo"])
			return nil
		},
		"career": func() error {
			sets, e := c.stats(ctx, "playercareerstats", url.Values{"PlayerID": {strconv.Itoa(id)}, "LeagueID": {"00"}, "PerMode": {"PerGame"}})
			if e != nil {
				return e
			}
			if _, ok := sets["SeasonTotalsRegularSeason"]; !ok {
				return fmt.Errorf("missing SeasonTotalsRegularSeason")
			}
			out.Seasons = sets["SeasonTotalsRegularSeason"]
			out.Career = sets["CareerTotalsRegularSeason"]
			out.Playoffs = sets["SeasonTotalsPostSeason"]
			out.Highs = sets["CareerHighs"]
			return nil
		},
		"games": func() error {
			q := profileQuery(season)
			q.Set("PlayerID", strconv.Itoa(id))
			sets, e := c.stats(ctx, "playergamelog", q)
			if e != nil {
				return e
			}
			if _, ok := sets["PlayerGameLog"]; !ok {
				return fmt.Errorf("missing PlayerGameLog")
			}
			out.Games = sets["PlayerGameLog"]
			return nil
		},
	})
	out.Updated = time.Now()
	return out
}

// Match complete names only, with diacritics and punctuation normalized. Never
// join NBA and ESPN numeric IDs, which belong to different namespaces.
func normalizeName(name string) string {
	name = strings.NewReplacer(
		"č", "c", "ć", "c", "ç", "c", "ş", "s", "š", "s", "ž", "z",
		"é", "e", "è", "e", "ê", "e", "ë", "e", "ē", "e",
		"ö", "o", "ó", "o", "ò", "o", "ô", "o", "õ", "o", "ø", "o",
		"ü", "u", "ú", "u", "ù", "u", "û", "u", "ū", "u", "ů", "u",
		"í", "i", "ì", "i", "î", "i", "ï", "i", "ī", "i",
		"á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "ā", "a", "å", "a", "ą", "a",
		"ñ", "n", "ņ", "n", "ń", "n",
		"ģ", "g", "ğ", "g",
		"ł", "l", "ļ", "l",
		"ď", "d", "đ", "d",
		"ř", "r",
		"ţ", "t",
		"ß", "ss", "æ", "ae", "œ", "oe",
	).Replace(strings.ToLower(name))
	var b strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
