package host

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"sync"
)

type Runner func(name string, args ...string) ([]byte, error)

func ExecRunner(name string, args ...string) ([]byte, error) {
	out, err := exec.Command(name, args...).Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return out, fmt.Errorf("%w: %s", err, bytes.TrimSpace(exitErr.Stderr))
	}
	return out, err
}

var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

const (
	tabLabelPrefix   = "Devin · "
	maxTabLabelRunes = 32
)

type Host struct {
	run         Runner
	goos        string
	paneID      string
	herdrPath   string
	workspaceID string
	directory   string

	mu   sync.Mutex
	tabs map[string]string
}

func New(run Runner, goos, paneID, herdrPath string) *Host {
	if herdrPath == "" {
		herdrPath = "herdr"
	}
	return &Host{run: run, goos: goos, paneID: paneID, herdrPath: herdrPath, tabs: map[string]string{}}
}

func (h *Host) InWorkspace(workspaceID string) *Host {
	h.workspaceID = workspaceID
	return h
}

func (h *Host) InDirectory(directory string) *Host {
	h.directory = directory
	return h
}

func (h *Host) Attach(sessionID, title string) error {
	if !sessionIDPattern.MatchString(sessionID) {
		return fmt.Errorf("refusing to attach: unexpected session id %q", sessionID)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if tabID, ok := h.tabs[sessionID]; ok {
		if _, err := h.run(h.herdrPath, "tab", "get", tabID); err == nil {
			_, err := h.run(h.herdrPath, "tab", "focus", tabID)
			return err
		}
		delete(h.tabs, sessionID)
	}
	tabID, paneID, err := h.createTab(tabLabel(title))
	if err != nil {
		return err
	}
	h.tabs[sessionID] = tabID
	if _, err := h.run(h.herdrPath, "pane", "run", paneID, "devin --cloud --resume "+sessionID); err != nil {
		return fmt.Errorf("herdr pane run: %w", err)
	}
	return nil
}

func (h *Host) createTab(label string) (string, string, error) {
	args := []string{"tab", "create", "--label", label, "--focus"}
	if h.workspaceID != "" {
		args = append(args, "--workspace", h.workspaceID)
	}
	if h.directory != "" {
		args = append(args, "--cwd", h.directory)
	}
	out, err := h.run(h.herdrPath, args...)
	if err != nil {
		return "", "", fmt.Errorf("herdr tab create: %w", err)
	}
	var resp struct {
		Result struct {
			Tab struct {
				TabID string `json:"tab_id"`
			} `json:"tab"`
			RootPane struct {
				PaneID string `json:"pane_id"`
			} `json:"root_pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", "", fmt.Errorf("decode herdr tab create: %w", err)
	}
	if resp.Result.Tab.TabID == "" || resp.Result.RootPane.PaneID == "" {
		return "", "", errors.New("herdr tab create returned no tab or pane id")
	}
	return resp.Result.Tab.TabID, resp.Result.RootPane.PaneID, nil
}

func tabLabel(title string) string {
	label := []rune(tabLabelPrefix + title)
	if len(label) <= maxTabLabelRunes {
		return string(label)
	}
	return string(label[:maxTabLabelRunes-1]) + "…"
}

func (h *Host) OpenURL(target string) error {
	if parsed, err := url.Parse(target); err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("refusing to open non-web URL %q", target)
	}
	opener := "xdg-open"
	if h.goos == "darwin" {
		opener = "open"
	}
	if _, err := h.run(opener, target); err != nil {
		return fmt.Errorf("open %s: %w", target, err)
	}
	return nil
}

func (h *Host) SSH(sessionID string) error {
	if !sessionIDPattern.MatchString(sessionID) {
		return fmt.Errorf("refusing to ssh: unexpected session id %q", sessionID)
	}
	paneID, err := h.splitBesideBoard()
	if err != nil {
		return err
	}
	if _, err := h.run(h.herdrPath, "pane", "run", paneID, "devin ssh "+sessionID); err != nil {
		return fmt.Errorf("herdr pane run: %w", err)
	}
	return nil
}

func (h *Host) splitBesideBoard() (string, error) {
	args := []string{"pane", "split", "--direction", "right"}
	if h.paneID != "" {
		args = append(args, "--pane", h.paneID)
	} else {
		args = append(args, "--current")
	}
	out, err := h.run(h.herdrPath, args...)
	if err != nil {
		return "", fmt.Errorf("herdr pane split: %w", err)
	}
	var resp struct {
		Result struct {
			Pane struct {
				PaneID string `json:"pane_id"`
			} `json:"pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("decode herdr pane split: %w", err)
	}
	if resp.Result.Pane.PaneID == "" {
		return "", errors.New("herdr pane split returned no pane id")
	}
	return resp.Result.Pane.PaneID, nil
}
