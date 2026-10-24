package api

import (
	"encoding/json"
)

// Resources and prompts: the small extras of the protocol. A resource is
// something an assistant can read without asking a question; a prompt is a
// ready-made request a person can pick in their chat program.

const (
	mcpResSummary = "mediarium://library/summary"
	mcpResHealth  = "mediarium://health"
)

var mcpResources = []map[string]any{
	{"uri": mcpResSummary, "name": "Library summary", "description": "Totals for movies, shows, episodes and albums, space used and the split by quality.", "mimeType": "application/json"},
	{"uri": mcpResHealth, "name": "Health", "description": "What is broken or worth fixing right now.", "mimeType": "application/json"},
}

func (c *mcpCtx) readResource(params json.RawMessage) (any, *rpcError) {
	var p struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.URI == "" {
		return nil, &rpcError{Code: rpcInvalidParams, Message: "resources/read needs a uri."}
	}
	var (
		out any
		err error
	)
	switch p.URI {
	case mcpResSummary:
		out, err = toolLibraryStats(c, struct{}{})
	case mcpResHealth:
		out, err = toolHealth(c, struct{}{})
	default:
		return nil, &rpcError{Code: -32002, Message: "Resource not found: " + p.URI}
	}
	if err != nil {
		return nil, &rpcError{Code: rpcInternalError, Message: "The resource could not be read."}
	}
	text, _ := json.Marshal(out)
	return map[string]any{"contents": []map[string]any{{"uri": p.URI, "mimeType": "application/json", "text": string(text)}}}, nil
}

type mcpPrompt struct {
	name, desc string
	args       []map[string]any
	text       func(args map[string]string) string
}

var mcpPrompts = []mcpPrompt{
	{
		name: "health_check", desc: "Check that everything in Mediarium is working and say what to fix first.",
		text: func(map[string]string) string {
			return "Use health_report and queue_status, and tell me in plain words whether everything in Mediarium is working. If something is wrong, say what to fix first."
		},
	},
	{
		name: "failed_downloads", desc: "Go through the failed downloads and suggest what to do.",
		text: func(map[string]string) string {
			return "Use downloads_failed to list the failed downloads. For each, say why it failed in plain words and whether another release is worth trying. Do not retry anything until I say so."
		},
	},
	{
		name: "release_group_check", desc: "Find the movies that are not from one release group.",
		args: []map[string]any{{"name": "group", "description": "The release group, such as FuN.", "required": true}},
		text: func(a map[string]string) string {
			return "Use release_group_report to find which movies are not from the release group " + a["group"] + ". Show the count and the first few titles."
		},
	},
}

func mcpPromptList() []map[string]any {
	out := []map[string]any{}
	for _, p := range mcpPrompts {
		e := map[string]any{"name": p.name, "description": p.desc}
		if p.args != nil {
			e["arguments"] = p.args
		}
		out = append(out, e)
	}
	return out
}

func (c *mcpCtx) getPrompt(params json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Name == "" {
		return nil, &rpcError{Code: rpcInvalidParams, Message: "prompts/get needs a prompt name."}
	}
	for _, pr := range mcpPrompts {
		if pr.name != p.Name {
			continue
		}
		for _, a := range pr.args {
			if req, _ := a["required"].(bool); req && p.Arguments[a["name"].(string)] == "" {
				return nil, &rpcError{Code: rpcInvalidParams, Message: "The prompt needs the argument " + a["name"].(string) + "."}
			}
		}
		return map[string]any{
			"description": pr.desc,
			"messages":    []map[string]any{{"role": "user", "content": map[string]any{"type": "text", "text": pr.text(p.Arguments)}}},
		}, nil
	}
	return nil, &rpcError{Code: rpcInvalidParams, Message: "Unknown prompt: " + p.Name}
}
