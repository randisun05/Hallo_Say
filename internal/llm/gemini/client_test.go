package gemini

import (
	"encoding/json"
	"testing"

	"github.com/randisun05/Hallo_Say/internal/llm"
)

func TestUppercaseSchemaTypes(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{"type": "string", "description": "isi pesan"},
		},
		"required": []string{"message"},
	}

	out := uppercaseSchemaTypes(schema)
	if out["type"] != "OBJECT" {
		t.Fatalf("top-level type = %v, want OBJECT", out["type"])
	}
	props := out["properties"].(map[string]interface{})
	msgSchema := props["message"].(map[string]interface{})
	if msgSchema["type"] != "STRING" {
		t.Fatalf("nested type = %v, want STRING", msgSchema["type"])
	}
	if msgSchema["description"] != "isi pesan" {
		t.Fatalf("description lost: %v", msgSchema["description"])
	}
}

func TestToolUseRoundTrip(t *testing.T) {
	// Simulates the agent's loop: a function call comes back from Gemini,
	// gets executed, and the tool result must be routable back to the
	// right function name even though Gemini has no call-id concept.
	geminiResp := response{
		Candidates: []candidate{{
			Content: content{
				Role: "model",
				Parts: []part{
					{FunctionCall: &functionCall{Name: "set_reminder", Args: json.RawMessage(`{"message":"beli susu"}`)}},
				},
			},
		}},
	}

	blocks := fromGeminiParts(geminiResp.Candidates[0].Content.Parts)
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

	contents := toGeminiContents(messages)
	if len(contents) != 2 {
		t.Fatalf("expected 2 contents, got %d", len(contents))
	}
	fr := contents[1].Parts[0].FunctionResponse
	if fr == nil || fr.Name != "set_reminder" {
		t.Fatalf("functionResponse name = %+v, want set_reminder", fr)
	}
	if fr.Response["result"] != "Reminder tersimpan" {
		t.Fatalf("functionResponse.response = %+v", fr.Response)
	}
}
