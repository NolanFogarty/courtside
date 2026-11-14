package backend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const espnDetailCompetitors = `[{"homeAway":"home","score":"49","team":{"id":"22","name":"Trail Blazers","abbreviation":"POR"}},{"homeAway":"away","score":"50","team":{"id":"25","name":"Thunder","abbreviation":"OKC"}}]`
const espnDetailBoard = `{"events":[{"id":"401909965","date":"2026-11-11T04:00Z","competitions":[{"competitors":` + espnDetailCompetitors + `}]}]}`
const espnDetailFixture = `{
 "header":{"id":"401909965","competitions":[{"date":"2026-11-11T04:00Z","competitors":` + espnDetailCompetitors + `}]},
 "boxscore":{
  "teams":[{"team":{"id":"25"},"statistics":[{"name":"fieldGoalsMade-fieldGoalsAttempted","displayValue":"16-32"},{"name":"assists","displayValue":"9"}]}],
  "players":[
   {"team":{"id":"25"},"statistics":[{"names":["PTS","MIN","FG","+/-","AST"],"athletes":[{"athlete":{"id":"4278073","shortName":"S. Gilgeous-Alexander"},"starter":true,"stats":["12","13","3-9","-1","1"]},{"athlete":{"shortName":"DNP"},"didNotPlay":true,"stats":[]}]}]},
   {"team":{"id":"22"},"statistics":[{"names":["MIN","PTS"],"athletes":[{"athlete":{"displayName":"Deni Avdija"},"stats":["15","8"]}]}]}
  ]},
 "plays":[{"text":"First play","period":{"number":1},"clock":{"displayValue":"12:00"},"team":{"id":"22"}},{"text":"Latest play","period":{"number":2},"clock":{"displayValue":"01:00"}}]
}`

func TestESPNDetailFallback(t *testing.T) {
	g := Game{GameId: "0022600216", StartUTC: "2026-11-11T04:00:00Z", HomeTeam: "Trail Blazers", AwayTeam: "Thunder"}
	for _, tc := range []struct {
		name, board, summary string
		fail                 bool
	}{
		{"success", espnDetailBoard, espnDetailFixture, false},
		{"wrong teams", strings.ReplaceAll(espnDetailBoard, "Thunder", "Celtics"), espnDetailFixture, true},
		{"wrong date", strings.ReplaceAll(espnDetailBoard, "2026-11-11", "2026-11-12"), espnDetailFixture, true},
		{"wrong summary", espnDetailBoard, strings.ReplaceAll(espnDetailFixture, "401909965", "other"), true},
		{"empty summary", espnDetailBoard, `{}`, true},
		{"no players", espnDetailBoard, strings.ReplaceAll(espnDetailFixture, `"players":`, `"ignored":`), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/scoreboard":
					if r.URL.Query().Get("dates") != "20261110" {
						t.Errorf("wrong basketball date: %s", r.URL)
					}
					fmt.Fprint(w, tc.board)
				case "/summary":
					if r.URL.Query().Get("event") != "401909965" {
						t.Errorf("used NBA ID for ESPN: %s", r.URL)
					}
					fmt.Fprint(w, tc.summary)
				default:
					t.Errorf("unexpected URL %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			d, err := fetchESPNDetail(context.Background(), server.Client(), server.URL+"/", g)
			if tc.fail {
				if err == nil {
					t.Fatal("expected rejection")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if d.Source != "ESPN" || d.Away.Score != 50 || d.Home.Score != 49 || d.Away.FGPct != 50 || d.Away.Ast != 9 {
				t.Fatalf("incorrect teams: %+v", d)
			}
			p := d.Away.Players[0]
			if p.PersonID != 0 || p.Pts != 12 || p.FGM != 3 || p.FGA != 9 || p.Min != "13" || p.PlusMinus != -1 || !p.Starter {
				t.Fatalf("incorrect player: %+v", p)
			}
			if d.Away.Players[1].Min != "0" || d.Home.Players[0].Name != "Deni Avdija" {
				t.Fatal("DNP/name fallback failed")
			}
			if len(d.Plays) != 2 || d.Plays[0].Desc != "Latest play" || d.Plays[1].Team != "POR" {
				t.Fatalf("incorrect plays: %+v", d.Plays)
			}
		})
	}
}

func TestESPNDetailRequiresGameIdentity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := fetchESPNDetail(ctx, http.DefaultClient, "http://invalid/", Game{}); err == nil {
		t.Fatal("missing game identity accepted")
	}
}
