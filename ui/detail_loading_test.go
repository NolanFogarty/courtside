package ui

import (
	"errors"
	"testing"

	"github.com/NolanFogarty/courtside/backend"
)

func TestDetailFeedsArriveIndependently(t *testing.T) {
	box := gameDetailMsg{gameID: "test", detail: backend.GameDetail{Away: backend.TeamDetail{Name: "Away", Players: []backend.PlayerLine{{Name: "Player", Pts: 12}}}}}
	plays := gamePlaysMsg{gameID: "test", plays: []backend.PlayLine{{Desc: "Made shot", Period: 1}}}
	for _, playsFirst := range []bool{false, true} {
		m := detail{gameID: "test", loading: true}
		if playsFirst {
			m, _ = m.Update(plays)
		}
		m, _ = m.Update(box)
		if m.loading || len(m.game.away.players) != 1 {
			t.Fatal("box score not displayed independently")
		}
		if !playsFirst {
			m, _ = m.Update(plays)
		}
		if len(m.game.plays) != 1 {
			t.Fatal("plays lost due to arrival order")
		}
		m, _ = m.Update(gamePlaysMsg{gameID: "other", plays: plays.plays})
		m, _ = m.Update(gamePlaysMsg{gameID: "test"})
		m, _ = m.Update(gameDetailMsg{gameID: "test", err: errors.New("transient failure")})
		if len(m.game.plays) != 1 || len(m.game.away.players) != 1 {
			t.Fatal("refresh failure erased existing data")
		}
	}
}
