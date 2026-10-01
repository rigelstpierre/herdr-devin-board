package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
	"github.com/rigelstpierre/herdr-devin-board/internal/github"
	"github.com/rigelstpierre/herdr-devin-board/internal/host"
	"github.com/rigelstpierre/herdr-devin-board/internal/ui"
)

func main() {
	credentialsPath, err := devin.DefaultCredentialsPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "herdr-devin-board:", err)
		os.Exit(1)
	}
	svc := newService(credentialsPath, archiveKeyPath(), github.NewClient(github.GHRunner))
	h := host.New(host.ExecRunner, runtime.GOOS, os.Getenv("HERDR_PANE_ID"), os.Getenv("HERDR_BIN_PATH")).
		InWorkspace(os.Getenv("HERDR_WORKSPACE_ID")).
		InDirectory(workspaceDir(os.Getenv("HERDR_PLUGIN_CONTEXT_JSON")))
	model := ui.New(ui.Deps{
		Load:    svc.Load,
		Archive: svc.Archive,
		OpenURL: h.OpenURL,
		SSH:     h.SSH,
		Attach:  h.Attach,
		Now:     time.Now,
	})
	if _, err := tea.NewProgram(model, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "herdr-devin-board:", err)
		os.Exit(1)
	}
}

func archiveKeyPath() string {
	dir := os.Getenv("HERDR_PLUGIN_CONFIG_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config", "herdr", "plugins", "config", "rigelstpierre.devin-board")
	}
	return filepath.Join(dir, "api_key")
}
