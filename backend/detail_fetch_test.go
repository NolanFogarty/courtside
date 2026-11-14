package backend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	nba "github.com/NolanFogarty/nba-sdk"
)

func TestLiveBoxDoesNotWaitForStarterSupplement(t *testing.T) {
	var statsCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/stats/") {
			statsCalls.Add(1)
			http.Error(w, "unavailable", 503)
			return
		}
		fmt.Fprint(w, `{"game":{"awayTeam":{"teamName":"Away","score":12,"players":[{"nameI":"A. Player","statistics":{"points":12,"minutes":"PT05M00S"}}]},"homeTeam":{"teamName":"Home","score":10,"players":[{"nameI":"B. Player","statistics":{"points":10}}]}}}`)
	}))
	defer server.Close()
	client := nba.NewClient(nba.WithLiveBaseURL(server.URL), nba.WithStatsBaseURL(server.URL), nba.WithRetry(0, 0))
	got, err := fetchBoxScore(client, context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if got.Away.Score != 12 || got.Away.Players[0].Pts != 12 {
		t.Fatalf("missing live data: %+v", got)
	}
	if statsCalls.Load() != 0 {
		t.Fatal("live box score requested optional stats supplement")
	}
}

func TestEmptyBoxScoreIsNotSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) }))
	defer server.Close()
	client := nba.NewClient(nba.WithLiveBaseURL(server.URL), nba.WithStatsBaseURL(server.URL), nba.WithRetry(0, 0))
	if _, err := fetchBoxScore(client, context.Background(), "test"); err == nil {
		t.Fatal("empty box score reported success")
	}
}
