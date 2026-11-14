package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/NolanFogarty/courtside/backend"
	"github.com/charmbracelet/x/ansi"
)

func fixtureDetail() detail {
	d := detail{gameID: "1", width: 70, height: 30}
	d.game.home = teamBox{name: "Lakers", tricode: "LAL"}
	d.game.away = teamBox{name: "Celtics", tricode: "BOS"}
	d.game.plays = []playEvent{
		{period: 2, clock: "08:44", team: "LAL", desc: strings.Repeat("word ", 30)},
		{period: 2, clock: "08:30", team: "", desc: "Timeout called by Boston"},
	}
	return d
}

func TestScheduledDetailKeepsSeededHeaderOnEmptyResponse(t *testing.T) {
	g := backend.GameFromSeasonSchedule(backend.SeasonGame{
		GameID: "s1", Away: "Lakers", Home: "Celtics", GameStatus: 1,
		DateTimeUTC: "2026-11-03T00:30:00Z", Status: "7:30 PM ET",
	})
	d := newDetail(g, 100, 32)
	if !d.scheduled {
		t.Fatal("upcoming game must open as scheduled")
	}
	// The box score endpoint has nothing for a game that hasn't tipped off;
	// its empty response must not wipe the seeded team names.
	d, _ = d.Update(gameDetailMsg{gameID: "s1", detail: backend.GameDetail{}})
	view := ansi.Strip(d.View().Content)
	if !strings.Contains(view, "Lakers") || !strings.Contains(view, "Celtics") {
		t.Fatalf("scheduled header must keep team names, got %q", view)
	}
}

func TestFullscreenPBPTogglesAndWraps(t *testing.T) {
	m := fixtureDetail()
	m, _ = m.Update(press("enter"))
	if !m.fullPbp {
		t.Fatal("enter must open fullscreen play-by-play")
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "FULL SCREEN") {
		t.Fatalf("fullscreen view missing title: %q", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("fullscreen line exceeds width %d: %q", m.width, line)
		}
	}
	wrapped := 0
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "word") {
			wrapped++
		}
	}
	if wrapped < 2 {
		t.Fatalf("long play must wrap onto multiple lines, got %d: %q", wrapped, view)
	}
	m, _ = m.Update(press("esc"))
	if m.fullPbp {
		t.Fatal("esc must exit fullscreen play-by-play")
	}
}

func TestFullscreenPBPRequiresFocus(t *testing.T) {
	m := fixtureDetail()
	m.focus = focusAway
	m, _ = m.Update(press("enter"))
	if m.fullPbp {
		t.Fatal("enter must not open fullscreen when play-by-play is not focused")
	}
	m.focus = focusPBP
	m, _ = m.Update(press("enter"))
	if !m.fullPbp {
		t.Fatal("enter must open fullscreen when play-by-play is focused")
	}
}

func TestFullscreenPBPScrolls(t *testing.T) {
	m := fixtureDetail()
	for i := 0; i < 40; i++ {
		m.game.plays = append(m.game.plays, playEvent{period: 2, clock: "08:00", team: "BOS", desc: "short play"})
	}
	m, _ = m.Update(press("enter"))
	m, _ = m.Update(press("end"))
	if m.fullScroll != m.fullMaxScroll() || m.fullScroll == 0 {
		t.Fatalf("end must scroll to bottom, got %d of %d", m.fullScroll, m.fullMaxScroll())
	}
}

func TestScheduleUsesLocalTime(t *testing.T) {
	g := backend.SeasonGame{DateTimeUTC: "2026-11-03T00:30:00Z"}
	want, _ := time.Parse(time.RFC3339, g.DateTimeUTC)
	if scheduleLocalDate(g) != want.Local().Format("2006-01-02") {
		t.Fatalf("schedule date must be local, got %q", scheduleLocalDate(g))
	}
	abbrev, _ := time.Now().Zone()
	if tip := formatScheduleTip(want); !strings.HasSuffix(tip, abbrev) {
		t.Fatalf("tip must carry local zone %q, got %q", abbrev, tip)
	}
}
