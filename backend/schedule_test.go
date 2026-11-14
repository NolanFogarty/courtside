package backend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NolanFogarty/nba-sdk/stats"
)

// The full-season schedule endpoint sits behind Akamai, which silently holds
// connections open when the browser fingerprint is incomplete. This guards the
// header set that unblocked the request.
func TestScheduleRequestUsesFullBrowserFingerprint(t *testing.T) {
	req, err := newScheduleRequest(context.Background(), scheduleURL("2026-27"))
	if err != nil {
		t.Fatal(err)
	}
	if ua := req.Header.Get("User-Agent"); !strings.Contains(ua, "Windows NT") || !strings.Contains(ua, "Chrome/120") {
		t.Fatalf("User-Agent must be the Windows Chrome fingerprint, got %q", ua)
	}
	for _, h := range []string{"Referer", "Origin", "Accept", "x-nba-stats-origin", "x-nba-stats-token", "sec-ch-ua", "sec-fetch-mode", "sec-fetch-site"} {
		if req.Header.Get(h) == "" {
			t.Fatalf("schedule request is missing required header %q", h)
		}
	}
}

func TestFetchSeasonScheduleSendsHeadersAndParses(t *testing.T) {
	var gotUA, gotToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotToken = r.Header.Get("x-nba-stats-token")
		fmt.Fprint(w, `{"leagueSchedule":{"gameDates":[{"gameDate":"11/02/2026","games":[{"gameId":"1","gameStatusText":"Final","gameStatus":3,"gameDateEst":"2026-11-02","gameTimeEst":"7:30 PM ET","gameDateTimeUTC":"2026-11-03T00:30:00Z","awayTeam":{"teamName":"Lakers","score":110,"wins":1,"losses":0},"homeTeam":{"teamName":"Celtics","score":105,"wins":0,"losses":1}}]}]}}`)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	games, err := fetchScheduleFromURL(ctx, server.Client(), server.URL+"?LeagueID=00&Season=2026-27")
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 || games[0].Away != "Lakers" || games[0].Home != "Celtics" || games[0].AwayScore != 110 {
		t.Fatalf("unexpected games: %+v", games)
	}
	if !strings.Contains(gotUA, "Windows NT") {
		t.Fatalf("test server saw incomplete User-Agent %q", gotUA)
	}
	if gotToken != "true" {
		t.Fatalf("test server saw x-nba-stats-token %q", gotToken)
	}
}

func TestScoreboardGameUpcomingUsesLocalTime(t *testing.T) {
	g := stats.ScoreboardV3Game{GameID: "1", GameStatus: statusUpcoming, GameStatusText: "7:30 pm ET", GameTimeUTC: "2026-11-03T00:30:00Z"}
	wantTime, err := time.Parse(time.RFC3339, g.GameTimeUTC)
	if err != nil {
		t.Fatal(err)
	}
	if got := scoreboardGame(g); got.GameClock != formatTipLocal(wantTime) {
		t.Fatalf("upcoming tip must render in local time, got %q want %q", got.GameClock, formatTipLocal(wantTime))
	}
	fallback := stats.ScoreboardV3Game{GameID: "2", GameStatus: statusUpcoming, GameStatusText: "7:30 pm ET"}
	if got := scoreboardGame(fallback); got.GameClock != "7:30 pm ET" {
		t.Fatalf("missing UTC tip must keep provider text, got %q", got.GameClock)
	}
}

func TestTodayLocalMatchesSystemDate(t *testing.T) {
	now := time.Now().Local()
	today := todayLocal()
	if now.Year() != today.Year() || now.YearDay() != today.YearDay() {
		t.Fatalf("todayLocal %v does not match system date %v", today, now)
	}
	if !isTodayLocal(now.Format("2006-01-02")) {
		t.Fatal("isTodayLocal must recognize the system date")
	}
}
