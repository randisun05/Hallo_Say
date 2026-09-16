// Package ollama implements llm.Provider against a self-hosted Ollama
// server (https://ollama.com), e.g. running Qwen2.5 on your own VM. Uses
// the standard library only.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/randisun05/Hallo_Say/internal/llm"
)

type Client struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewClient builds a client for a local/self-hosted Ollama server. baseURL
// defaults to http://localhost:11434 if empty.
func NewClient(baseURL, model string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		model:      model,
		httpClient: &http.Client{Timeout: 5 * time.Minute}, // local CPU inference can be slow
	}
}

type chatMessage struct {
	Role      string     `json:"role"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
}

type toolCall struct {
	Function toolCallFunction `json:"function"`
}

type toolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type toolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type toolDef struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type chatRequest struct {
	Model    string                 `json:"model"`
	Messages []chatMessage          `json:"messages"`
	Tools    []toolDef              `json:"tools,omitempty"`
	Stream   bool                   `json:"stream"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
	Done    bool        `json:"done"`
	Error   string      `json:"error"`
}

func toOllamaMessages(system string, messages []llm.Message) []chatMessage {
	out := make([]chatMessage, 0, len(messages)+1)
	if system != "" {
		out = append(out, chatMessage{Role: "system", Content: system})
	}

	for _, m := range messages {
		if isPureToolResults(m) {
			for _, b := range m.Content {
				out = append(out, chatMessage{Role: "tool", Content: b.ToolResultText})
			}
			continue
		}

		role := "user"
		if m.Role == llm.RoleAssistant {
			role = "assistant"
		}
		var text strings.Builder
		var calls []toolCall
		for _, b := range m.Content {
			switch b.Type {
			case llm.BlockText:
				text.WriteString(b.Text)
			case llm.BlockToolUse:
				calls = append(calls, toolCall{Function: toolCallFunction{Name: b.ToolName, Arguments: b.ToolInput}})
			}
		}
		out = append(out, chatMessage{Role: role, Content: text.String(), ToolCalls: calls})
	}
	return out
}

func isPureToolResults(m llm.Message) bool {
	if len(m.Content) == 0 {
		return false
	}
	for _, b := range m.Content {
		if b.Type != llm.BlockToolResult {
			return false
		}
	}
	return true
}

func toOllamaTools(tools []llm.Tool) []toolDef {
	if len(tools) == 0 {
		return nil
	}
	out := make([]toolDef, len(tools))
	for i, t := range tools {
		out[i] = toolDef{
			Type: "function",
			Function: toolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		}
	}
	return out
}

func fromOllamaMessage(m chatMessage) []llm.ContentBlock {
	out := make([]llm.ContentBlock, 0, 1+len(m.ToolCalls))
	if m.Content != "" {
		out = append(out, llm.ContentBlock{Type: llm.BlockText, Text: m.Content})
	}
	for i, tc := range m.ToolCalls {
		out = append(out, llm.ContentBlock{
			Type:      llm.BlockToolUse,
			ToolUseID: llm.NewToolUseID(tc.Function.Name, i),
			ToolName:  tc.Function.Name,
			ToolInput: tc.Function.Arguments,
		})
	}
	return out
}

func (c *Client) SendMessage(ctx context.Context, system string, messages []llm.Message, tools []llm.Tool, maxTokens int) (*llm.Response, error) {
	req := chatRequest{
		Model:    c.model,
		Messages: toOllamaMessages(system, messages),
		Tools:    toOllamaTools(tools),
		Stream:   false,
		Options:  map[string]interface{}{"num_predict": maxTokens},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("content-type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call ollama server (%s): %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama api error (%d): %s", resp.StatusCode, string(respBody))
	}

	var out chatResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if out.Error != "" {
		return nil, fmt.Errorf("ollama error: %s", out.Error)
	}

	blocks := fromOllamaMessage(out.Message)
	stopReason := llm.StopEnd
	for _, b := range blocks {
		if b.Type == llm.BlockToolUse {
			stopReason = llm.StopToolUse
			break
		}
	}

	return &llm.Response{Content: blocks, StopReason: stopReason}, nil
}
