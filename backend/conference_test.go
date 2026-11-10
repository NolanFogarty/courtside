package backend

import (
	"strings"
	"testing"
)

func TestConferenceForTeamCoversAllThirty(t *testing.T) {
	east, west := 0, 0
	for _, team := range Teams {
		conf := ConferenceForTeam(team.Name)
		if conf == "" {
			t.Errorf("no conference for full name %q", team.Name)
			continue
		}
		// The scoreboard only carries nicknames, so those must resolve too.
		nick := team.Name
		if j := strings.LastIndex(nick, " "); j >= 0 {
			// "Trail Blazers" is a two-word nickname; keep both words.
			if !strings.HasSuffix(team.Name, "Trail Blazers") {
				nick = nick[j+1:]
			}
		}
		if got := ConferenceForTeam(nick); got != conf {
			t.Errorf("nickname %q resolved to %q, want %q", nick, got, conf)
		}
		switch conf {
		case "east":
			east++
		case "west":
			west++
		default:
			t.Errorf("unexpected conference %q for %q", conf, team.Name)
		}
	}
	if east != 15 || west != 15 {
		t.Fatalf("want 15 east and 15 west teams, got %d and %d", east, west)
	}
}

func TestConferenceForTeamFormats(t *testing.T) {
	for name, want := range map[string]string{
		"Boston Celtics": "east", "CELTICS": "east", "Celtics": "east",
		"Los Angeles Lakers": "west", "Trail Blazers": "west",
		"Portland Trail Blazers": "west", "76ers": "east",
		"Philadelphia 76ers": "east", "": "", "Seattle Supersonics": "",
	} {
		if got := ConferenceForTeam(name); got != want {
			t.Errorf("ConferenceForTeam(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestGameConferencePrefersHomeTeam(t *testing.T) {
	// Cross-conference matchup hosted in Boston sorts east.
	g := Game{AwayTeam: "Lakers", HomeTeam: "Celtics"}
	if got := GameConference(g); got != "east" {
		t.Fatalf("GameConference = %q, want east", got)
	}
	// Same matchup hosted in Los Angeles sorts west.
	g = Game{AwayTeam: "Celtics", HomeTeam: "Lakers"}
	if got := GameConference(g); got != "west" {
		t.Fatalf("GameConference = %q, want west", got)
	}
	// Unknown home side falls back to the away team.
	g = Game{AwayTeam: "Bulls", HomeTeam: "Unknown"}
	if got := GameConference(g); got != "east" {
		t.Fatalf("GameConference = %q, want east fallback", got)
	}
	if got := GameConference(Game{AwayTeam: "Unknown", HomeTeam: "Unknown"}); got != "" {
		t.Fatalf("GameConference for unknown teams = %q, want empty", got)
	}
}
