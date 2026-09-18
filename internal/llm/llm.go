// Package llm defines a provider-agnostic chat/tool-use interface so the
// agent can run against different backends (Claude, Gemini, ...).
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

	// ProviderSignature is opaque per-provider metadata attached to a
	// model-generated block (e.g. Gemini's "thoughtSignature" on thinking
	// models) that must be replayed unchanged on the next turn for the
	// conversation to stay valid. Providers that don't need it leave it
	// empty; other providers ignore it.
	ProviderSignature string
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

// Provider is implemented by each backend (Claude, Gemini, Ollama, ...).
type Provider interface {
	SendMessage(ctx context.Context, system string, messages []Message, tools []Tool, maxTokens int) (*Response, error)
}

// NewToolUseID and ToolNameFromID help providers (Gemini, Ollama, ...) that
// don't hand back a real call ID for function/tool calls: we encode the
// tool name into a synthetic ID so the agent's tool_use/tool_result
// bookkeeping still works, then recover the name when building the
// provider-specific tool-result payload.
func NewToolUseID(name string, index int) string {
	return fmt.Sprintf("%s#%d", name, index)
}

func ToolNameFromID(id string) string {
	if i := strings.LastIndex(id, "#"); i >= 0 {
		return id[:i]
	}
	return id
}
