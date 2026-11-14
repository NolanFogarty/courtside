package main

import (
	"fmt"
	"os"

	"github.com/NolanFogarty/courtside/ui"
)

func main() {
	// Open the UI immediately with no seeded games; the list fetches today
	// async via Init and the 15s tick retries. Blocking startup on the
	// scoreboard endpoint meant a hanging stats.nba.com froze the whole app
	// before first paint, and swallowing the error then showed a dead list.
	if err := ui.Run(nil); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
