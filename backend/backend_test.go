package backend

import (
	"fmt"
	"testing"
)

func validateGames(t *testing.T, games []Game) {
	t.Helper()
	for i, game := range games {
		if game.GameId == "" {
			t.Errorf("game %d: GameId is empty", i)
		}
		if game.HomeTeam == "" {
			t.Errorf("game %d: HomeTeam is empty", i)
		}
		if game.AwayTeam == "" {
			t.Errorf("game %d: AwayTeam is empty", i)
		}
		if game.GameClock == "" {
			t.Errorf("game %d: GameClock is empty", i)
		}
	}
}

func TestGetLiveGames(t *testing.T) {
	games, err := GetLiveGames()
	if err != nil {
		t.Skipf("live endpoint unavailable: %v", err)
	}

	if games == nil {
		t.Fatal("GetLiveGames() returned nil")
	}

	fmt.Printf("\n=== Live Games (%d total) ===\n", len(games))
	for i, game := range games {
		fmt.Printf("%d. %s vs %s | %d-%d | %s\n", i+1, game.HomeTeam, game.AwayTeam, game.HomeScore, game.AwayScore, game.GameClock)
		fmt.Printf("GameID: %s\n", game.GameId)
	}

	validateGames(t, games)
}

func TestGetScheduledGames(t *testing.T) {
	games, err := GetScheduledGames()
	if err != nil {
		t.Skipf("live endpoint unavailable: %v", err)
	}

	if games == nil {
		t.Fatal("GetScheduledGames() returned nil")
	}

	fmt.Printf("\n=== Scheduled Games (%d total) ===\n", len(games))
	for i, game := range games {
		fmt.Printf("%d. %s vs %s | %s\n", i+1, game.HomeTeam, game.AwayTeam, game.GameClock)
		fmt.Printf("GameID: %s\n", game.GameId)
	}

	validateGames(t, games)
}
