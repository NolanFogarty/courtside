package backend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestScheduleRetryableOnly(t *testing.T) {
	if isScheduleRetryable(nil) {
		t.Fatal("nil error must not be retryable")
	}
	if isScheduleRetryable(fmt.Errorf("schedule API returned 404 Not Found")) {
		t.Fatal("404 must not be retried")
	}
	if !isScheduleRetryable(fmt.Errorf("schedule API returned 500 Internal Server Error")) {
		t.Fatal("500 must be retryable")
	}
	if !isScheduleRetryable(context.DeadlineExceeded) {
		t.Fatal("timeout must be retryable")
	}
}

func TestFetchSeasonScheduleNonRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := fetchSeasonScheduleWithClient(ctx, server.Client(), server.URL); err == nil {
		t.Fatal("expected 404 to fail fast")
	}
}

func TestFetchSeasonScheduleEmptyPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"leagueSchedule":{}}`)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	games, err := fetchScheduleFromURL(ctx, server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if games == nil || len(games) != 0 {
		t.Fatalf("empty payload must yield non-nil empty slice, got %#v", games)
	}
}

func TestRowNumericGuards(t *testing.T) {
	r := Row{"PTS": []byte(`"DNP"`), "ID": []byte(`"abc"`)}
	if r.IsNumeric("PTS") || r.IsNumeric("ID") {
		t.Fatal("non-numeric text must not be numeric")
	}
	if r.Number("PTS") != 0 {
		t.Fatal("Number still returns 0 for garbage; callers must check IsNumeric first")
	}
	if !(Row{"PTS": []byte(`30.5`)}.IsNumeric("PTS")) {
		t.Fatal("numeric text must be numeric")
	}
}

func TestNormalizeDiacritics(t *testing.T) {
	if normalizeName("Porziņģis") != "porzingis" {
		t.Fatalf("g-cedilla not transliterated: %q", normalizeName("Porziņģis"))
	}
	if normalizeName("Valančiūnas") != "valanciunas" {
		t.Fatalf("got %q", normalizeName("Valančiūnas"))
	}
	if normalizeName("Şengün") != "sengun" {
		t.Fatalf("got %q", normalizeName("Şengün"))
	}
}

func TestInjuryTeamMatching(t *testing.T) {
	lal := Team{ID: 14, Name: "Los Angeles Lakers", Code: "LAL", ESPN: "lal"}
	if !injuryBelongsToTeam("Los Angeles Lakers", "LAL", lal) {
		t.Fatal("code match must belong")
	}
	if injuryBelongsToTeam("Boston Celtics", "BOS", lal) {
		t.Fatal("cross-team code must not belong")
	}
	nop := Team{ID: 19, Name: "New Orleans Pelicans", Code: "NOP", ESPN: "no"}
	if !injuryBelongsToTeam("New Orleans Pelicans", "NO", nop) {
		t.Fatal("ESPN NO must map to NOP")
	}
	if !injuryBelongsToTeam("Los Angeles Lakers", "", lal) {
		t.Fatal("empty code must fall back to name match")
	}
	if injuryBelongsToTeam("Boston Celtics", "", lal) {
		t.Fatal("name mismatch must not belong")
	}
}

func TestInjuryTeamMatchingEspnAbbreviations(t *testing.T) {
	cases := []Team{
		{ID: 1610612744, Name: "Golden State Warriors", Code: "GSW", ESPN: "gs"},
		{ID: 1610612752, Name: "New York Knicks", Code: "NYK", ESPN: "ny"},
		{ID: 1610612759, Name: "San Antonio Spurs", Code: "SAS", ESPN: "sa"},
		{ID: 1610612762, Name: "Utah Jazz", Code: "UTA", ESPN: "utah"},
		{ID: 1610612764, Name: "Washington Wizards", Code: "WAS", ESPN: "wsh"},
	}
	codes := map[string]string{"GSW": "GS", "NYK": "NY", "SAS": "SA", "UTA": "UTAH", "WAS": "WSH"}
	for _, team := range cases {
		if !injuryBelongsToTeam(team.Name, codes[team.Code], team) {
			t.Fatalf("ESPN code %q must map to %s (%s)", codes[team.Code], team.Name, team.Code)
		}
	}
}

func TestGameFromSeasonScheduleUnknown(t *testing.T) {
	g := GameFromSeasonSchedule(SeasonGame{GameID: "1", Status: "Weird", GameStatus: 0})
	if !g.NotStarted() || g.IsLive() {
		t.Fatalf("unknown status must be treated as not started: %+v", g)
	}
	if g.GameClock != "Weird" && g.GameClock == "" {
		t.Fatalf("unexpected clock %q", g.GameClock)
	}
}

func TestZeroValueGameIsNotStarted(t *testing.T) {
	// A Game built without a status (e.g. a literal in tests or a fresh
	// scoreboard row) must not be mistaken for a played game: no box-score
	// fetch, and TBD instead of 0-0.
	if g := (Game{}); !g.NotStarted() || g.IsLive() {
		t.Fatalf("zero-value game must be not started: %+v", g)
	}
}

func TestPeriodAndClock(t *testing.T) {
	if periodLabel(5) != "OT" || periodLabel(6) != "OT2" {
		t.Fatalf("bad OT labels: %q %q", periodLabel(5), periodLabel(6))
	}
	if parseGameClock("PT12M") != "12:00" {
		t.Fatalf("PT12M must parse, got %q", parseGameClock("PT12M"))
	}
}

func TestClinchMark(t *testing.T) {
	if clinchMark(" - x") != "x" || clinchMark(" - e") != "e" || clinchMark("") != "" {
		t.Fatal("clinch markers wrong")
	}
	if clinchMark(" - ") != "" {
		t.Fatal("dangling dash must yield empty")
	}
}

func TestLoadSectionsKeepsFinishedData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	failures := loadSectionsWithContext(ctx, map[string]func() error{
		"fast": func() error { return nil },
		"slow": func() error { <-ctx.Done(); return ctx.Err() },
	})
	if _, ok := failures["fast"]; ok {
		t.Fatal("finished section must not be marked failed")
	}
	if _, ok := failures["slow"]; !ok {
		t.Fatal("stalled section must be reported")
	}
}
