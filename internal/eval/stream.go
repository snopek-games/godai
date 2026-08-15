package eval

import (
	"bufio"
	"encoding/json"
	"io"
	"time"
)

// Decoding is tolerant on purpose: unknown events are ignored and malformed
// lines counted, so a new Claude Code release doesn't fail a run.

type streamEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`

	SessionID       string          `json:"session_id"`
	ParentToolUseID *string         `json:"parent_tool_use_id"`
	Message         json.RawMessage `json:"message"`

	Model           string        `json:"model"`
	Tools           []string      `json:"tools"`
	MCPServers      []MCPServer   `json:"mcp_servers"`
	MCPServerErrors []MCPSrvError `json:"mcp_server_errors"`

	IsError      bool    `json:"is_error"`
	DurationMS   int64   `json:"duration_ms"`
	DurationAPI  int64   `json:"duration_api_ms"`
	NumTurns     int     `json:"num_turns"`
	StopReason   string  `json:"stop_reason"`
	Result       string  `json:"result"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	Usage        Usage   `json:"usage"`
}

type MCPServer struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type MCPSrvError struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

func (u Usage) Total() int {
	return u.InputTokens + u.OutputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

func (u *Usage) add(o Usage) {
	u.InputTokens += o.InputTokens
	u.OutputTokens += o.OutputTokens
	u.CacheCreationInputTokens += o.CacheCreationInputTokens
	u.CacheReadInputTokens += o.CacheReadInputTokens
}

type apiMessage struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	ID        string          `json:"id"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`

	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	Content  json.RawMessage `json:"content"`
}

// RunMetrics is how the agent worked, independent of whether it got the answer
// right.
type RunMetrics struct {
	SessionID string `json:"session_id"`
	Model     string `json:"model"`

	WallTime     time.Duration `json:"-"`
	WallTimeMS   int64         `json:"wall_time_ms"`
	AgentTimeMS  int64         `json:"agent_duration_ms"`
	APITimeMS    int64         `json:"api_duration_ms"`
	NumTurns     int           `json:"num_turns"`
	TotalCostUSD float64       `json:"total_cost_usd"`
	Usage        Usage         `json:"usage"`
	TotalTokens  int           `json:"total_tokens"`

	ResultIsError  bool   `json:"result_is_error"`
	ResultSubtype  string `json:"result_subtype"`
	ResultMessage  string `json:"result_message,omitempty"`
	StopReason     string `json:"stop_reason"`
	MalformedLines int    `json:"malformed_lines"`

	// Claude Code emits a result event per stretch of a run; more than one means the totals above are summed.
	Segments int `json:"segments"`

	MCPServersLoaded []MCPServer   `json:"mcp_servers_loaded"`
	MCPErrors        []MCPSrvError `json:"mcp_errors"`
	ToolsAvailable   []string      `json:"-"`

	ToolCalls   []ToolCall      `json:"tool_calls"`
	toolResults map[string]bool // tool_use_id -> is_error
}

type ToolCall struct {
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	ID       string          `json:"id"`
	Turn     int             `json:"turn"`
	Subagent bool            `json:"subagent"`
	IsError  bool            `json:"is_error"`
}

func ParseStream(r io.Reader) (*RunMetrics, error) {
	m := &RunMetrics{toolResults: map[string]bool{}}

	sc := bufio.NewScanner(r)
	// A tool input can hold a whole file, so the default 64KB is not enough.
	sc.Buffer(make([]byte, 0, 1<<20), 32<<20)

	turn := 0
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev streamEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			m.MalformedLines++
			continue
		}

		switch ev.Type {
		case "system":
			if ev.Subtype == "init" {
				m.SessionID = ev.SessionID
				m.Model = ev.Model
				m.MCPServersLoaded = ev.MCPServers
				m.MCPErrors = ev.MCPServerErrors
				m.ToolsAvailable = ev.Tools
			}

		case "assistant":
			turn++
			var msg apiMessage
			if err := json.Unmarshal(ev.Message, &msg); err != nil {
				m.MalformedLines++
				continue
			}
			for _, b := range msg.Content {
				if b.Type != "tool_use" {
					continue
				}
				m.ToolCalls = append(m.ToolCalls, ToolCall{
					Name:     b.Name,
					Input:    b.Input,
					ID:       b.ID,
					Turn:     turn,
					Subagent: ev.ParentToolUseID != nil,
				})
			}

		case "user":
			// Tool results arrive as user messages. Recording which errored
			// separates picking the wrong tool from calling it wrong.
			var msg apiMessage
			if err := json.Unmarshal(ev.Message, &msg); err != nil {
				continue
			}
			for _, b := range msg.Content {
				if b.Type == "tool_result" && b.ToolUseID != "" {
					m.toolResults[b.ToolUseID] = b.IsError
				}
			}

		case "result":
			m.Segments++
			m.ResultSubtype = ev.Subtype
			m.ResultIsError = m.ResultIsError || ev.IsError
			if ev.IsError {
				m.ResultMessage = ev.Result
			}
			m.StopReason = ev.StopReason
			m.AgentTimeMS += ev.DurationMS
			m.APITimeMS += ev.DurationAPI
			m.NumTurns += ev.NumTurns
			m.TotalCostUSD += ev.TotalCostUSD
			m.Usage.add(ev.Usage)
			m.TotalTokens = m.Usage.Total()
		}
	}
	if err := sc.Err(); err != nil {
		return m, err
	}

	for i := range m.ToolCalls {
		m.ToolCalls[i].IsError = m.toolResults[m.ToolCalls[i].ID]
	}
	return m, nil
}
