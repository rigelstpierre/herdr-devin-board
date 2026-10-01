package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
)

type CI int

const (
	CIUnknown CI = iota
	CINone
	CIPassing
	CIPending
	CIFailing
)

type PRStatus struct {
	Number  int
	State   string
	IsDraft bool
	CI      CI
	Review  string
}

type Runner func(ctx context.Context, args ...string) ([]byte, error)

func GHRunner(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "gh", args...).Output()
}

type Client struct {
	run Runner
}

func NewClient(run Runner) *Client {
	return &Client{run: run}
}

func (c *Client) PRStatus(ctx context.Context, prURL string) (PRStatus, error) {
	out, err := c.run(ctx, "pr", "view", prURL, "--json", "number,state,isDraft,reviewDecision,statusCheckRollup")
	if err != nil {
		return PRStatus{}, fmt.Errorf("gh pr view %s: %w", prURL, err)
	}
	var raw struct {
		Number         int     `json:"number"`
		State          string  `json:"state"`
		IsDraft        bool    `json:"isDraft"`
		ReviewDecision string  `json:"reviewDecision"`
		Rollup         []check `json:"statusCheckRollup"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return PRStatus{}, fmt.Errorf("decode gh output for %s: %w", prURL, err)
	}
	return PRStatus{
		Number:  raw.Number,
		State:   raw.State,
		IsDraft: raw.IsDraft,
		Review:  raw.ReviewDecision,
		CI:      rollupCI(latestPerCheck(raw.Rollup)),
	}, nil
}

func (c *Client) Statuses(ctx context.Context, urls []string, parallel int) map[string]PRStatus {
	statuses := make(map[string]PRStatus, len(urls))
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, parallel)
	for _, prURL := range urls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			status, err := c.PRStatus(ctx, prURL)
			if err != nil {
				return
			}
			mu.Lock()
			statuses[prURL] = status
			mu.Unlock()
		}()
	}
	wg.Wait()
	return statuses
}

type check struct {
	Typename   string `json:"__typename"`
	Name       string `json:"name"`
	Workflow   string `json:"workflowName"`
	Context    string `json:"context"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
	StartedAt  string `json:"startedAt"`
}

func (c check) key() string {
	if c.Typename == "StatusContext" {
		return "status:" + c.Context
	}
	return "check:" + c.Workflow + "/" + c.Name
}

func latestPerCheck(checks []check) []check {
	latest := map[string]check{}
	var order []string
	for _, c := range checks {
		key := c.key()
		previous, seen := latest[key]
		if !seen {
			order = append(order, key)
		}
		if !seen || c.StartedAt >= previous.StartedAt {
			latest[key] = c
		}
	}
	result := make([]check, 0, len(order))
	for _, key := range order {
		result = append(result, latest[key])
	}
	return result
}

func rollupCI(checks []check) CI {
	if len(checks) == 0 {
		return CINone
	}
	overall := CIPassing
	for _, c := range checks {
		switch checkResult(c) {
		case CIFailing:
			return CIFailing
		case CIPending:
			overall = CIPending
		}
	}
	return overall
}

func checkResult(c check) CI {
	if c.Typename == "StatusContext" {
		switch c.State {
		case "FAILURE", "ERROR":
			return CIFailing
		case "PENDING", "EXPECTED":
			return CIPending
		}
		return CIPassing
	}
	if c.Status != "COMPLETED" {
		return CIPending
	}
	switch c.Conclusion {
	case "FAILURE", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE":
		return CIFailing
	}
	return CIPassing
}
