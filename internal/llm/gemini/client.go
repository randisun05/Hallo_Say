// Package gemini implements llm.Provider against the Google Gemini API
// (generativelanguage.googleapis.com), which has a free usage tier. Uses
// the standard library only.
package gemini

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

const apiBase = "https://generativelanguage.googleapis.com/v1beta/models"

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

type part struct {
	Text             string            `json:"text,omitempty"`
	FunctionCall     *functionCall     `json:"functionCall,omitempty"`
	FunctionResponse *functionResponse `json:"functionResponse,omitempty"`
}

type functionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type functionResponse struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

type content struct {
	Role  string `json:"role"`
	Parts []part `json:"parts"`
}

type functionDeclaration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type toolDecl struct {
	FunctionDeclarations []functionDeclaration `json:"functionDeclarations"`
}

type generationConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
}

type request struct {
	SystemInstruction *content         `json:"system_instruction,omitempty"`
	Contents          []content        `json:"contents"`
	Tools             []toolDecl       `json:"tools,omitempty"`
	GenerationConfig  generationConfig `json:"generationConfig"`
}

type candidate struct {
	Content      content `json:"content"`
	FinishReason string  `json:"finishReason"`
}

type response struct {
	Candidates []candidate `json:"candidates"`
}

type apiError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func toGeminiContents(messages []llm.Message) []content {
	out := make([]content, 0, len(messages))
	for _, m := range messages {
		role := "user"
		if m.Role == llm.RoleAssistant {
			role = "model"
		}
		parts := make([]part, 0, len(m.Content))
		for _, b := range m.Content {
			switch b.Type {
			case llm.BlockText:
				if b.Text != "" {
					parts = append(parts, part{Text: b.Text})
				}
			case llm.BlockToolUse:
				parts = append(parts, part{FunctionCall: &functionCall{Name: b.ToolName, Args: b.ToolInput}})
			case llm.BlockToolResult:
				name := llm.ToolNameFromID(b.ToolResultForID)
				resp := map[string]interface{}{"result": b.ToolResultText}
				if b.ToolResultError {
					resp = map[string]interface{}{"error": b.ToolResultText}
				}
				parts = append(parts, part{FunctionResponse: &functionResponse{Name: name, Response: resp}})
			}
		}
		if len(parts) == 0 {
			continue
		}
		out = append(out, content{Role: role, Parts: parts})
	}
	return out
}

func toGeminiTools(tools []llm.Tool) []toolDecl {
	if len(tools) == 0 {
		return nil
	}
	decls := make([]functionDeclaration, len(tools))
	for i, t := range tools {
		decls[i] = functionDeclaration{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  uppercaseSchemaTypes(t.InputSchema),
		}
	}
	return []toolDecl{{FunctionDeclarations: decls}}
}

// uppercaseSchemaTypes converts JSON-Schema-style "type": "object"/"string"
// values into the uppercase enum names Gemini's schema format expects.
func uppercaseSchemaTypes(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}
	out := make(map[string]interface{}, len(schema))
	for k, v := range schema {
		switch k {
		case "type":
			if s, ok := v.(string); ok {
				out[k] = strings.ToUpper(s)
				continue
			}
			out[k] = v
		case "properties":
			if m, ok := v.(map[string]interface{}); ok {
				props := make(map[string]interface{}, len(m))
				for pk, pv := range m {
					if pm, ok := pv.(map[string]interface{}); ok {
						props[pk] = uppercaseSchemaTypes(pm)
					} else {
						props[pk] = pv
					}
				}
				out[k] = props
				continue
			}
			out[k] = v
		case "items":
			if m, ok := v.(map[string]interface{}); ok {
				out[k] = uppercaseSchemaTypes(m)
				continue
			}
			out[k] = v
		default:
			out[k] = v
		}
	}
	return out
}

func fromGeminiParts(parts []part) []llm.ContentBlock {
	out := make([]llm.ContentBlock, 0, len(parts))
	callIndex := 0
	for _, p := range parts {
		switch {
		case p.Text != "":
			out = append(out, llm.ContentBlock{Type: llm.BlockText, Text: p.Text})
		case p.FunctionCall != nil:
			id := llm.NewToolUseID(p.FunctionCall.Name, callIndex)
			callIndex++
			out = append(out, llm.ContentBlock{
				Type:      llm.BlockToolUse,
				ToolUseID: id,
				ToolName:  p.FunctionCall.Name,
				ToolInput: p.FunctionCall.Args,
			})
		}
	}
	return out
}

func (c *Client) SendMessage(ctx context.Context, system string, messages []llm.Message, tools []llm.Tool, maxTokens int) (*llm.Response, error) {
	req := request{
		Contents:         toGeminiContents(messages),
		Tools:            toGeminiTools(tools),
		GenerationConfig: generationConfig{MaxOutputTokens: maxTokens},
	}
	if system != "" {
		req.SystemInstruction = &content{Parts: []part{{Text: system}}}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", apiBase, c.model, c.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("content-type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call gemini api: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr apiError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("gemini api error (%d): %s", resp.StatusCode, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("gemini api error (%d): %s", resp.StatusCode, string(respBody))
	}

	var out response
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if len(out.Candidates) == 0 {
		return &llm.Response{StopReason: llm.StopEnd}, nil
	}

	cand := out.Candidates[0]
	blocks := fromGeminiParts(cand.Content.Parts)

	stopReason := llm.StopEnd
	for _, b := range blocks {
		if b.Type == llm.BlockToolUse {
			stopReason = llm.StopToolUse
			break
		}
	}

	return &llm.Response{Content: blocks, StopReason: stopReason}, nil
}
