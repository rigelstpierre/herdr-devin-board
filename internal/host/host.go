package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
)

type Runner func(name string, args ...string) ([]byte, error)

func ExecRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type Host struct {
	run       Runner
	goos      string
	paneID    string
	herdrPath string
}

func New(run Runner, goos, paneID, herdrPath string) *Host {
	if herdrPath == "" {
		herdrPath = "herdr"
	}
	return &Host{run: run, goos: goos, paneID: paneID, herdrPath: herdrPath}
}

func (h *Host) OpenURL(target string) error {
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
