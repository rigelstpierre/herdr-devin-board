package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
	"github.com/rigelstpierre/herdr-devin-board/internal/github"
	"github.com/rigelstpierre/herdr-devin-board/internal/host"
	"github.com/rigelstpierre/herdr-devin-board/internal/ui"
)

const refreshInterval = 30 * time.Second

func main() {
	credentialsPath, err := devin.DefaultCredentialsPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "herdr-devin-board:", err)
		os.Exit(1)
	}
	h := host.New(host.ExecRunner, runtime.GOOS, os.Getenv("HERDR_PANE_ID"), os.Getenv("HERDR_BIN_PATH"))
	model := ui.New(ui.Deps{
		Load:     newLoader(credentialsPath, github.NewClient(github.GHRunner)),
		OpenURL:  h.OpenURL,
		SSH:      h.SSH,
		Now:      time.Now,
		Interval: refreshInterval,
	})
	if _, err := tea.NewProgram(model, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "herdr-devin-board:", err)
		os.Exit(1)
	}
}
