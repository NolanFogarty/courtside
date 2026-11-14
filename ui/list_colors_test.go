package ui

import (
	"strings"
	"testing"

	"github.com/NolanFogarty/courtside/backend"
	"github.com/charmbracelet/x/ansi"
)

func TestFinalGameOutcomeUsesForegroundWithoutBackground(t *testing.T) {
	game := backend.GameFromSeasonSchedule(backend.SeasonGame{Away: "Away", Home: "Home", AwayScore: 110, HomeScore: 105, Status: "Final", GameStatus: 3})

	for _, selected := range []bool{false, true} {
		line := formatGameLine(game, 60, selected)
		if strings.Contains(line, "\x1b[48;") || strings.Contains(line, "\x1b[48m") {
			t.Errorf("selected=%v: outcome must not use background color: %q", selected, line)
		}
		plain := ansi.Strip(line)
		for _, want := range []string{"Away", "Home", "110", "105", "Final"} {
			if !strings.Contains(plain, want) {
				t.Errorf("selected=%v: missing %q in %q", selected, want, plain)
			}
		}
		if strings.Contains(line, winTextSty.Render("Away")) == false || !strings.Contains(line, lossTextSty.Render("Home")) {
			t.Errorf("selected=%v: winner and loser should retain foreground styles: %q", selected, line)
		}
	}
}

func TestFinalGameHomeWinnerForegroundStyles(t *testing.T) {
	game := backend.GameFromSeasonSchedule(backend.SeasonGame{Away: "Away", Home: "Home", AwayScore: 90, HomeScore: 100, Status: "Final", GameStatus: 3})
	line := formatGameLine(game, 60, false)
	if !strings.Contains(line, lossTextSty.Render("Away")) || !strings.Contains(line, winTextSty.Render("Home")) {
		t.Fatalf("winner and loser foreground styles missing: %q", line)
	}
	if strings.Contains(line, "\x1b[48;") || strings.Contains(line, "\x1b[48m") {
		t.Fatalf("outcome must not use background color: %q", line)
	}
}
