package main

import "testing"

func TestWorkspaceDirFromPluginContext(t *testing.T) {
	cases := map[string]string{
		`{"workspace_id":"wB","workspace_cwd":"/Users/me/project"}`: "/Users/me/project",
		`{"workspace_id":"wB"}`: "",
		`not json`:              "",
		``:                      "",
	}
	for raw, want := range cases {
		if got := workspaceDir(raw); got != want {
			t.Errorf("workspaceDir(%q) = %q want %q", raw, got, want)
		}
	}
}
