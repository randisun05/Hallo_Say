// Package llm defines a provider-agnostic chat/tool-use interface so the
// agent can run against different backends (Claude, Gemini, ...).
package llm

import (
	"context"
	"encoding/json"
)

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

const (
	BlockText       = "text"
	BlockToolUse    = "tool_use"
	BlockToolResult = "tool_result"
)

// ContentBlock is one piece of a message: plain text, a request from the
// model to call a tool, or the result of a tool call fed back to the model.
type ContentBlock struct {
	Type string

	// BlockText
	Text string

	// BlockToolUse
	ToolUseID string
	ToolName  string
	ToolInput json.RawMessage

	// BlockToolResult
	ToolResultForID string
	ToolResultText  string
	ToolResultError bool
}

type Message struct {
	Role    string
	Content []ContentBlock
}

type Tool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
}

// StopReason indicates why the model stopped generating.
type StopReason string

const (
	StopToolUse StopReason = "tool_use"
	StopEnd     StopReason = "end"
)

type Response struct {
	Content    []ContentBlock
	StopReason StopReason
}

// Provider is implemented by each backend (Claude, Gemini, ...).
type Provider interface {
	SendMessage(ctx context.Context, system string, messages []Message, tools []Tool, maxTokens int) (*Response, error)
}
