package main

import "encoding/json"

func workspaceDir(pluginContextJSON string) string {
	var context struct {
		WorkspaceCwd string `json:"workspace_cwd"`
	}
	if json.Unmarshal([]byte(pluginContextJSON), &context) != nil {
		return ""
	}
	return context.WorkspaceCwd
}
