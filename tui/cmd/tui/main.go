// Command tui runs the terminal user interface for the accounting
// application, talking to the accounting REST API.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"money/tui/internal/client"
	"money/tui/internal/ui"
)

func main() {
	apiURL := os.Getenv("MONEY_API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:30730"
	}

	c := client.New(apiURL)
	app := ui.New(c)

	if _, err := tea.NewProgram(app).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
