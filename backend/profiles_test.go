package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeProfileSets(t *testing.T) {
	for _, payload := range []string{
		`{"resultSets":[{"name":"Roster","headers":["ID","NAME","UNKNOWN","SHORT"],"rowSet":[[42,"Jane",null]]}]}`,
		`{"resultSet":{"name":"Roster","headers":["ID","NAME","UNKNOWN","SHORT"],"rowSet":[[42,"Jane",null]]}}`,
		`{"resultSets":{"name":"Roster","headers":["ID","NAME","UNKNOWN","SHORT"],"rowSet":[[42,"Jane",null]]}}`,
	} {
		sets, err := decodeSets(strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		row := sets["Roster"][0]
		if row.Number("ID") != 42 || row.Text("NAME") != "Jane" || row.Has("UNKNOWN") || row.Has("SHORT") {
			t.Fatalf("unexpected row: %v", row)
		}
	}
	if _, err := decodeSets(strings.NewReader(`{"message":"temporarily unavailable"}`)); err == nil {
		t.Fatal("expected an invalid response to fail")
	}
}

func TestTeamProfilePartialFailureAndSalarySeason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/stats/commonteamroster":
			fmt.Fprint(w, `{"resultSets":[{"name":"CommonTeamRoster","headers":["PLAYER_ID","PLAYER","NUM","POSITION"],"rowSet":[[77,"Luka Dončić","77","G"],[78,"Other Player","1","C"]]},{"name":"Coaches","headers":["COACH_NAME"],"rowSet":[["Coach Name"]]}]}`)
		case "/stats/teamplayerdashboard":
			if r.URL.Query().Get("PerMode") != "PerGame" || r.URL.Query().Get("TeamID") != "1610612747" {
				t.Error("missing dashboard parameters")
			}
			fmt.Fprint(w, `{"resultSets":[{"name":"PlayersSeasonTotals","headers":["PLAYER_ID","PTS","GP"],"rowSet":[[77,30.5,7]]}]}`)
		case "/stats/teamdetails":
			fmt.Fprint(w, `{"resultSets":[{"name":"TeamBackground","headers":["ARENA"],"rowSet":[["Home Arena"]]}]}`)
		case "/stats/teaminfocommon":
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		case "/stats/teamgamelog":
			fmt.Fprint(w, `{"resultSets":[{"name":"TeamGameLog","headers":["GAME_DATE","PTS"],"rowSet":[["NOV 01, 2026",100]]}]}`)
		case "/espn/teams/lal/roster":
			fmt.Fprint(w, `{"athletes":[{"displayName":"Luka Doncic","contracts":[{"salary":100,"season":{"year":2026}},{"salary":200,"season":{"year":2027}}]},{"displayName":"Other Player","contracts":[{"salary":999,"season":{"year":2026}}]}]}`)
		case "/espn/injuries":
			fmt.Fprint(w, `{"injuries":[{"displayName":"Los Angeles Lakers","injuries":[{"athlete":{"displayName":"Luka Doncic"},"status":"Out","date":"2026-11-02T10:00Z","shortComment":"Ankle injury","details":{"type":"Ankle","returnDate":"2026-11-04"}}]},{"displayName":"Other Team","injuries":[{"athlete":{"displayName":"Other Player"},"status":"Out"}]}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := profileClient{server.Client(), server.URL + "/stats/", server.URL + "/espn/"}
	result := client.teamProfile(Team{ID: 1610612747, Name: "Los Angeles Lakers", Code: "LAL", ESPN: "lal"}, "2026-27")
	if len(result.Errors) != 1 || result.Errors["overview"] == "" {
		t.Fatalf("failures: %v", result.Errors)
	}
	if result.Franchise["TeamBackground"][0].Text("ARENA") != "Home Arena" {
		t.Fatal("franchise data missing")
	}
	if len(result.Roster) != 2 || len(result.Coaches) != 1 || len(result.Games) != 1 {
		t.Fatalf("partial data missing: %+v", result)
	}
	p := result.Roster[0]
	if p.Salary == nil || *p.Salary != 200 {
		t.Fatalf("wrong salary season: %v", p.Salary)
	}
	if result.Roster[1].Salary != nil {
		t.Fatal("old salary must not be presented as current")
	}
	if p.Stats.Number("PTS") != 30.5 || p.Injury == nil || p.Injury.Status != "Out" {
		t.Fatalf("missing matched data: %+v", p)
	}
	if len(result.Injuries) != 1 || result.Injuries[0].PlayerID != 77 {
		t.Fatalf("wrong injury/team mapping: %+v", result.Injuries)
	}
}

func TestSupplementFailuresAreNotEmptyReports(t *testing.T) {
	for _, payload := range []string{`{}`, `{"error":"feed unavailable"}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, payload) }))
		c := profileClient{server.Client(), server.URL + "/", server.URL + "/"}
		if _, err := c.injuries(context.Background(), Teams[0]); err == nil {
			t.Error("invalid injury response must not imply no injuries")
		}
		if _, err := c.supplementalRoster(context.Background(), Teams[0], "2026-27"); err == nil {
			t.Error("invalid salary response must not imply zero payroll")
		}
		server.Close()
	}
}

func TestPlayerProfileSections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("PlayerID") != "42" {
			t.Error("player ID missing")
		}
		switch r.URL.Path {
		case "/commonplayerinfo":
			fmt.Fprint(w, `{"resultSets":[{"name":"CommonPlayerInfo","headers":["DISPLAY_FIRST_LAST"],"rowSet":[["A Player"]]}]}`)
		case "/playercareerstats":
			fmt.Fprint(w, `{"resultSets":[{"name":"SeasonTotalsRegularSeason","headers":["SEASON_ID","PTS"],"rowSet":[["2026-27",22.5]]},{"name":"CareerTotalsRegularSeason","headers":["PTS"],"rowSet":[[20]]},{"name":"SeasonTotalsPostSeason","headers":["PTS"],"rowSet":[[25]]}]}`)
		case "/playergamelog":
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	c := profileClient{server.Client(), server.URL + "/", server.URL + "/"}
	p := c.playerProfile(42, "2026-27")
	if p.Info.Text("DISPLAY_FIRST_LAST") != "A Player" || len(p.Seasons) != 1 || len(p.Career) != 1 || len(p.Playoffs) != 1 || p.Errors["games"] == "" {
		b, _ := json.Marshal(p)
		t.Fatalf("unexpected profile: %s", b)
	}
}
