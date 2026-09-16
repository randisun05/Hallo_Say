package ollama

import (
	"encoding/json"
	"testing"

	"github.com/randisun05/Hallo_Say/internal/llm"
)

func TestToolUseRoundTrip(t *testing.T) {
	ollamaMsg := chatMessage{
		Role: "assistant",
		ToolCalls: []toolCall{
			{Function: toolCallFunction{Name: "set_reminder", Arguments: json.RawMessage(`{"message":"beli susu"}`)}},
		},
	}

	blocks := fromOllamaMessage(ollamaMsg)
	if len(blocks) != 1 || blocks[0].Type != llm.BlockToolUse || blocks[0].ToolName != "set_reminder" {
		t.Fatalf("unexpected blocks: %+v", blocks)
	}

	toolResult := llm.ContentBlock{
		Type:            llm.BlockToolResult,
		ToolResultForID: blocks[0].ToolUseID,
		ToolResultText:  "Reminder tersimpan",
	}
	messages := []llm.Message{
		{Role: llm.RoleAssistant, Content: blocks},
		{Role: llm.RoleUser, Content: []llm.ContentBlock{toolResult}},
	}

	out := toOllamaMessages("sistem", messages)
	// system + assistant (with tool_calls) + tool
	if len(out) != 3 {
		t.Fatalf("expected 3 ollama messages, got %d: %+v", len(out), out)
	}
	if out[0].Role != "system" || out[0].Content != "sistem" {
		t.Fatalf("system message wrong: %+v", out[0])
	}
	if out[1].Role != "assistant" || len(out[1].ToolCalls) != 1 || out[1].ToolCalls[0].Function.Name != "set_reminder" {
		t.Fatalf("assistant message wrong: %+v", out[1])
	}
	if out[2].Role != "tool" || out[2].Content != "Reminder tersimpan" {
		t.Fatalf("tool result message wrong: %+v", out[2])
	}
}
