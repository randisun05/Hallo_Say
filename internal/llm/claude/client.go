// Package claude implements llm.Provider against the Anthropic Messages
// API, using the standard library only.
package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/randisun05/Hallo_Say/internal/llm"
)

const (
	apiURL     = "https://api.anthropic.com/v1/messages"
	apiVersion = "2023-06-01"
)

type Client struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewClient(apiKey, model string) *Client {
	return &Client{
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

type wireBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type wireMessage struct {
	Role    string      `json:"role"`
	Content []wireBlock `json:"content"`
}

type wireTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type wireRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    string        `json:"system,omitempty"`
	Messages  []wireMessage `json:"messages"`
	Tools     []wireTool    `json:"tools,omitempty"`
}

type wireResponse struct {
	Content    []wireBlock `json:"content"`
	StopReason string      `json:"stop_reason"`
}

type apiError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func toWireMessages(messages []llm.Message) []wireMessage {
	out := make([]wireMessage, len(messages))
	for i, m := range messages {
		blocks := make([]wireBlock, len(m.Content))
		for j, b := range m.Content {
			switch b.Type {
			case llm.BlockText:
				blocks[j] = wireBlock{Type: "text", Text: b.Text}
			case llm.BlockToolUse:
				blocks[j] = wireBlock{Type: "tool_use", ID: b.ToolUseID, Name: b.ToolName, Input: b.ToolInput}
			case llm.BlockToolResult:
				blocks[j] = wireBlock{Type: "tool_result", ToolUseID: b.ToolResultForID, Content: b.ToolResultText, IsError: b.ToolResultError}
			}
		}
		out[i] = wireMessage{Role: m.Role, Content: blocks}
	}
	return out
}

func toWireTools(tools []llm.Tool) []wireTool {
	out := make([]wireTool, len(tools))
	for i, t := range tools {
		out[i] = wireTool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema}
	}
	return out
}

func fromWireBlocks(blocks []wireBlock) []llm.ContentBlock {
	out := make([]llm.ContentBlock, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case "text":
			out = append(out, llm.ContentBlock{Type: llm.BlockText, Text: b.Text})
		case "tool_use":
			out = append(out, llm.ContentBlock{Type: llm.BlockToolUse, ToolUseID: b.ID, ToolName: b.Name, ToolInput: b.Input})
		}
	}
	return out
}

func (c *Client) SendMessage(ctx context.Context, system string, messages []llm.Message, tools []llm.Tool, maxTokens int) (*llm.Response, error) {
	body, err := json.Marshal(wireRequest{
		Model:     c.model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  toWireMessages(messages),
		Tools:     toWireTools(tools),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", apiVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call anthropic api: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr apiError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("anthropic api error (%d): %s", resp.StatusCode, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("anthropic api error (%d): %s", resp.StatusCode, string(respBody))
	}

	var out wireResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	stopReason := llm.StopEnd
	if out.StopReason == "tool_use" {
		stopReason = llm.StopToolUse
	}
	return &llm.Response{Content: fromWireBlocks(out.Content), StopReason: stopReason}, nil
}
