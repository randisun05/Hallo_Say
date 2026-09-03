// Package agent wires together the Claude client, reminder store, and
// webhook tool into a single conversational agent per Telegram chat.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/randisun05/Hallo_Say/internal/claude"
	"github.com/randisun05/Hallo_Say/internal/reminder"
	"github.com/randisun05/Hallo_Say/internal/webhook"
)

const (
	maxToolIterations = 5
	maxHistoryEntries = 20
	maxTokens         = 1024
)

type Agent struct {
	claude   *claude.Client
	model    string
	store    *reminder.Store
	notifier func(chatID int64, text string)

	mu      sync.Mutex
	history map[int64][]claude.Message
}

func New(claudeClient *claude.Client, model string, store *reminder.Store, notifier func(chatID int64, text string)) *Agent {
	return &Agent{
		claude:   claudeClient,
		model:    model,
		store:    store,
		notifier: notifier,
		history:  make(map[int64][]claude.Message),
	}
}

var tools = []claude.Tool{
	{
		Name:        "set_reminder",
		Description: "Simpan reminder/to-do untuk chat ini yang akan dikirim ulang ke user pada waktu tertentu.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"message":   map[string]interface{}{"type": "string", "description": "Isi reminder yang akan dikirim ke user."},
				"remind_at": map[string]interface{}{"type": "string", "description": "Waktu pengingat dalam format RFC3339, mis. 2026-09-04T09:00:00+07:00."},
			},
			"required": []string{"message", "remind_at"},
		},
	},
	{
		Name:        "list_reminders",
		Description: "Tampilkan semua reminder yang masih aktif (belum terkirim) untuk chat ini.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
	{
		Name:        "cancel_reminder",
		Description: "Batalkan satu reminder berdasarkan ID-nya (ID didapat dari list_reminders).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{"type": "string", "description": "ID reminder yang mau dibatalkan."},
			},
			"required": []string{"id"},
		},
	},
	{
		Name:        "call_webhook",
		Description: "Panggil URL HTTP/HTTPS eksternal (mis. webhook, API status check) atas permintaan user. Tidak bisa mengakses alamat jaringan privat/internal.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url":    map[string]interface{}{"type": "string", "description": "URL tujuan, harus http/https."},
				"method": map[string]interface{}{"type": "string", "description": "HTTP method: GET, POST, PUT, PATCH, atau DELETE. Default GET."},
				"body":   map[string]interface{}{"type": "string", "description": "Body request, opsional (biasanya JSON)."},
			},
			"required": []string{"url"},
		},
	},
}

func systemPrompt(chatID int64) string {
	now := time.Now().Format(time.RFC3339)
	return fmt.Sprintf(`Kamu adalah asisten AI pribadi sekaligus asisten kerja milik user, diakses lewat Telegram.
Waktu sekarang: %s (gunakan ini untuk menghitung waktu relatif seperti "30 menit lagi" atau "besok pagi" saat memakai tool set_reminder).
Chat ID user ini: %d.

Kemampuanmu:
- Menjawab pertanyaan umum, meringkas teks/dokumen yang dikirim user.
- Membantu hal teknis/coding: review kode, jelaskan error, buatkan contoh kode.
- Mengatur reminder/to-do lewat tool set_reminder, list_reminders, cancel_reminder.
- Menjalankan automation sederhana lewat tool call_webhook bila user minta memanggil API/webhook tertentu.

Jawab singkat, jelas, dan dalam bahasa yang dipakai user. Gunakan tool hanya saat memang relevan dengan permintaan user.`, now, chatID)
}

func (a *Agent) Handle(ctx context.Context, chatID int64, userText string) (string, error) {
	a.mu.Lock()
	messages := append([]claude.Message{}, a.history[chatID]...)
	a.mu.Unlock()

	messages = append(messages, claude.Message{
		Role:    "user",
		Content: []claude.ContentBlock{{Type: "text", Text: userText}},
	})

	var finalText string
	for i := 0; i < maxToolIterations; i++ {
		resp, err := a.claude.SendMessage(ctx, a.model, systemPrompt(chatID), messages, tools, maxTokens)
		if err != nil {
			return "", err
		}

		messages = append(messages, claude.Message{Role: "assistant", Content: resp.Content})

		if resp.StopReason != "tool_use" {
			finalText = extractText(resp.Content)
			break
		}

		var toolResults []claude.ContentBlock
		for _, block := range resp.Content {
			if block.Type != "tool_use" {
				continue
			}
			result, isErr := a.executeTool(chatID, block.Name, block.Input)
			toolResults = append(toolResults, claude.ContentBlock{
				Type:      "tool_result",
				ToolUseID: block.ID,
				Content:   result,
				IsError:   isErr,
			})
		}
		messages = append(messages, claude.Message{Role: "user", Content: toolResults})
	}

	if finalText == "" {
		finalText = "Maaf, saya butuh beberapa langkah lagi untuk menyelesaikan ini. Coba ulangi permintaannya dengan lebih spesifik."
	}

	a.mu.Lock()
	if len(messages) > maxHistoryEntries {
		messages = messages[len(messages)-maxHistoryEntries:]
	}
	a.history[chatID] = messages
	a.mu.Unlock()

	return finalText, nil
}

func extractText(blocks []claude.ContentBlock) string {
	var out string
	for _, b := range blocks {
		if b.Type == "text" {
			out += b.Text
		}
	}
	return out
}

func (a *Agent) executeTool(chatID int64, name string, input json.RawMessage) (result string, isError bool) {
	switch name {
	case "set_reminder":
		var args struct {
			Message  string `json:"message"`
			RemindAt string `json:"remind_at"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return err.Error(), true
		}
		remindAt, err := time.Parse(time.RFC3339, args.RemindAt)
		if err != nil {
			return fmt.Sprintf("format remind_at tidak valid: %v", err), true
		}
		r, err := a.store.Add(chatID, args.Message, remindAt)
		if err != nil {
			return err.Error(), true
		}
		return fmt.Sprintf("Reminder tersimpan (id=%s) untuk %s", r.ID, r.RemindAt.Format(time.RFC3339)), false

	case "list_reminders":
		reminders := a.store.List(chatID)
		if len(reminders) == 0 {
			return "Tidak ada reminder aktif.", false
		}
		out := ""
		for _, r := range reminders {
			out += fmt.Sprintf("- [%s] %s pada %s\n", r.ID, r.Message, r.RemindAt.Format(time.RFC3339))
		}
		return out, false

	case "cancel_reminder":
		var args struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return err.Error(), true
		}
		ok, err := a.store.Cancel(chatID, args.ID)
		if err != nil {
			return err.Error(), true
		}
		if !ok {
			return "Reminder tidak ditemukan.", true
		}
		return "Reminder dibatalkan.", false

	case "call_webhook":
		var args struct {
			URL    string `json:"url"`
			Method string `json:"method"`
			Body   string `json:"body"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return err.Error(), true
		}
		out, err := webhook.Call(args.URL, args.Method, args.Body)
		if err != nil {
			return err.Error(), true
		}
		return out, false

	default:
		return fmt.Sprintf("tool tidak dikenal: %s", name), true
	}
}

// RunReminderScheduler polls the store for due reminders and delivers them
// via the notifier until ctx is cancelled.
func (a *Agent) RunReminderScheduler(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			due, err := a.store.DueReminders(time.Now())
			if err != nil {
				continue
			}
			for _, r := range due {
				a.notifier(r.ChatID, fmt.Sprintf("⏰ Reminder: %s", r.Message))
			}
		}
	}
}
