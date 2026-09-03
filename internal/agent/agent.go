// Package agent wires together an LLM provider, reminder store, webhook
// tool, and optional Google tools into a single conversational agent per
// Telegram chat.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/randisun05/Hallo_Say/internal/google"
	"github.com/randisun05/Hallo_Say/internal/llm"
	"github.com/randisun05/Hallo_Say/internal/reminder"
	"github.com/randisun05/Hallo_Say/internal/webhook"
)

const (
	maxToolIterations = 5
	maxHistoryEntries = 20
	maxTokens         = 1024
)

type Agent struct {
	provider llm.Provider
	store    *reminder.Store
	google   *google.Client // nil if Google integration isn't configured
	notifier func(chatID int64, text string)

	mu      sync.Mutex
	history map[int64][]llm.Message
}

func New(provider llm.Provider, store *reminder.Store, googleClient *google.Client, notifier func(chatID int64, text string)) *Agent {
	return &Agent{
		provider: provider,
		store:    store,
		google:   googleClient,
		notifier: notifier,
		history:  make(map[int64][]llm.Message),
	}
}

var baseTools = []llm.Tool{
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

var googleTools = []llm.Tool{
	{
		Name:        "list_calendar_events",
		Description: "Tampilkan acara mendatang di Google Calendar user (kalender utama).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"max_results": map[string]interface{}{"type": "integer", "description": "Jumlah maksimal acara yang ditampilkan, default 10."},
			},
		},
	},
	{
		Name:        "create_calendar_event",
		Description: "Buat acara baru di Google Calendar utama user.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"summary": map[string]interface{}{"type": "string", "description": "Judul acara."},
				"start":   map[string]interface{}{"type": "string", "description": "Waktu mulai, format RFC3339."},
				"end":     map[string]interface{}{"type": "string", "description": "Waktu selesai, format RFC3339."},
			},
			"required": []string{"summary", "start", "end"},
		},
	},
	{
		Name:        "search_drive_files",
		Description: "Cari file di Google Drive user berdasarkan nama.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query":       map[string]interface{}{"type": "string", "description": "Kata kunci nama file yang dicari."},
				"max_results": map[string]interface{}{"type": "integer", "description": "Jumlah maksimal hasil, default 10."},
			},
			"required": []string{"query"},
		},
	},
	{
		Name:        "list_recent_emails",
		Description: "Tampilkan email terbaru di Gmail user, opsional dengan query pencarian Gmail (mis. \"is:unread\").",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query":       map[string]interface{}{"type": "string", "description": "Query pencarian Gmail, opsional."},
				"max_results": map[string]interface{}{"type": "integer", "description": "Jumlah maksimal email, default 5."},
			},
		},
	},
	{
		Name:        "send_email",
		Description: "Kirim email dari akun Gmail user.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"to":      map[string]interface{}{"type": "string", "description": "Alamat email tujuan."},
				"subject": map[string]interface{}{"type": "string", "description": "Subjek email."},
				"body":    map[string]interface{}{"type": "string", "description": "Isi email (plain text)."},
			},
			"required": []string{"to", "subject", "body"},
		},
	},
}

func (a *Agent) availableTools() []llm.Tool {
	if a.google == nil {
		return baseTools
	}
	return append(append([]llm.Tool{}, baseTools...), googleTools...)
}

func systemPrompt(chatID int64, googleEnabled bool) string {
	now := time.Now().Format(time.RFC3339)
	googleNote := "Integrasi Google Calendar/Drive/Gmail belum dikonfigurasi."
	if googleEnabled {
		googleNote = "Kamu juga bisa mengelola Google Calendar (list/buat acara), mencari file Google Drive, dan membaca/mengirim Gmail lewat tool yang tersedia."
	}
	return fmt.Sprintf(`Kamu adalah asisten AI pribadi sekaligus asisten kerja milik user, diakses lewat Telegram.
Waktu sekarang: %s (gunakan ini untuk menghitung waktu relatif seperti "30 menit lagi" atau "besok pagi" saat memakai tool yang butuh waktu).
Chat ID user ini: %d.

Kemampuanmu:
- Menjawab pertanyaan umum, meringkas teks/dokumen yang dikirim user.
- Membantu hal teknis/coding: review kode, jelaskan error, buatkan contoh kode.
- Mengatur reminder/to-do lewat tool set_reminder, list_reminders, cancel_reminder.
- Menjalankan automation sederhana lewat tool call_webhook bila user minta memanggil API/webhook tertentu.
- %s

Jawab singkat, jelas, dan dalam bahasa yang dipakai user. Gunakan tool hanya saat memang relevan dengan permintaan user.`, now, chatID, googleNote)
}

func (a *Agent) Handle(ctx context.Context, chatID int64, userText string) (string, error) {
	a.mu.Lock()
	messages := append([]llm.Message{}, a.history[chatID]...)
	a.mu.Unlock()

	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: []llm.ContentBlock{{Type: llm.BlockText, Text: userText}},
	})

	tools := a.availableTools()

	var finalText string
	for i := 0; i < maxToolIterations; i++ {
		resp, err := a.provider.SendMessage(ctx, systemPrompt(chatID, a.google != nil), messages, tools, maxTokens)
		if err != nil {
			return "", err
		}

		messages = append(messages, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})

		if resp.StopReason != llm.StopToolUse {
			finalText = extractText(resp.Content)
			break
		}

		var toolResults []llm.ContentBlock
		for _, block := range resp.Content {
			if block.Type != llm.BlockToolUse {
				continue
			}
			result, isErr := a.executeTool(ctx, chatID, block.ToolName, block.ToolInput)
			toolResults = append(toolResults, llm.ContentBlock{
				Type:            llm.BlockToolResult,
				ToolResultForID: block.ToolUseID,
				ToolResultText:  result,
				ToolResultError: isErr,
			})
		}
		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: toolResults})
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

func extractText(blocks []llm.ContentBlock) string {
	var out string
	for _, b := range blocks {
		if b.Type == llm.BlockText {
			out += b.Text
		}
	}
	return out
}

func (a *Agent) executeTool(ctx context.Context, chatID int64, name string, input json.RawMessage) (result string, isError bool) {
	switch name {
	case "set_reminder":
		return a.toolSetReminder(chatID, input)
	case "list_reminders":
		return a.toolListReminders(chatID)
	case "cancel_reminder":
		return a.toolCancelReminder(chatID, input)
	case "call_webhook":
		return a.toolCallWebhook(input)
	case "list_calendar_events":
		return a.toolListCalendarEvents(ctx, input)
	case "create_calendar_event":
		return a.toolCreateCalendarEvent(ctx, input)
	case "search_drive_files":
		return a.toolSearchDriveFiles(ctx, input)
	case "list_recent_emails":
		return a.toolListRecentEmails(ctx, input)
	case "send_email":
		return a.toolSendEmail(ctx, input)
	default:
		return fmt.Sprintf("tool tidak dikenal: %s", name), true
	}
}

func (a *Agent) toolSetReminder(chatID int64, input json.RawMessage) (string, bool) {
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
}

func (a *Agent) toolListReminders(chatID int64) (string, bool) {
	reminders := a.store.List(chatID)
	if len(reminders) == 0 {
		return "Tidak ada reminder aktif.", false
	}
	out := ""
	for _, r := range reminders {
		out += fmt.Sprintf("- [%s] %s pada %s\n", r.ID, r.Message, r.RemindAt.Format(time.RFC3339))
	}
	return out, false
}

func (a *Agent) toolCancelReminder(chatID int64, input json.RawMessage) (string, bool) {
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
}

func (a *Agent) toolCallWebhook(input json.RawMessage) (string, bool) {
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
}

func (a *Agent) requireGoogle() (*google.Client, string, bool) {
	if a.google == nil {
		return nil, "Integrasi Google belum dikonfigurasi. Jalankan cmd/oauth-setup dan set GOOGLE_CLIENT_ID/SECRET.", true
	}
	return a.google, "", false
}

func (a *Agent) toolListCalendarEvents(ctx context.Context, input json.RawMessage) (string, bool) {
	client, errMsg, isErr := a.requireGoogle()
	if isErr {
		return errMsg, true
	}
	var args struct {
		MaxResults int `json:"max_results"`
	}
	_ = json.Unmarshal(input, &args)
	if args.MaxResults <= 0 {
		args.MaxResults = 10
	}

	events, err := client.ListUpcomingEvents(ctx, args.MaxResults)
	if err != nil {
		return err.Error(), true
	}
	if len(events) == 0 {
		return "Tidak ada acara mendatang.", false
	}
	out := ""
	for _, e := range events {
		out += fmt.Sprintf("- %s: %s s/d %s\n", e.Summary, e.Start.DateTime, e.End.DateTime)
	}
	return out, false
}

func (a *Agent) toolCreateCalendarEvent(ctx context.Context, input json.RawMessage) (string, bool) {
	client, errMsg, isErr := a.requireGoogle()
	if isErr {
		return errMsg, true
	}
	var args struct {
		Summary string `json:"summary"`
		Start   string `json:"start"`
		End     string `json:"end"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return err.Error(), true
	}
	start, err := time.Parse(time.RFC3339, args.Start)
	if err != nil {
		return fmt.Sprintf("format start tidak valid: %v", err), true
	}
	end, err := time.Parse(time.RFC3339, args.End)
	if err != nil {
		return fmt.Sprintf("format end tidak valid: %v", err), true
	}

	event, err := client.CreateEvent(ctx, args.Summary, start, end)
	if err != nil {
		return err.Error(), true
	}
	return fmt.Sprintf("Acara \"%s\" dibuat (id=%s).", event.Summary, event.ID), false
}

func (a *Agent) toolSearchDriveFiles(ctx context.Context, input json.RawMessage) (string, bool) {
	client, errMsg, isErr := a.requireGoogle()
	if isErr {
		return errMsg, true
	}
	var args struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return err.Error(), true
	}
	if args.MaxResults <= 0 {
		args.MaxResults = 10
	}

	files, err := client.SearchFiles(ctx, args.Query, args.MaxResults)
	if err != nil {
		return err.Error(), true
	}
	if len(files) == 0 {
		return "Tidak ada file yang cocok.", false
	}
	out := ""
	for _, f := range files {
		out += fmt.Sprintf("- %s (%s) id=%s\n", f.Name, f.MimeType, f.ID)
	}
	return out, false
}

func (a *Agent) toolListRecentEmails(ctx context.Context, input json.RawMessage) (string, bool) {
	client, errMsg, isErr := a.requireGoogle()
	if isErr {
		return errMsg, true
	}
	var args struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	_ = json.Unmarshal(input, &args)
	if args.MaxResults <= 0 {
		args.MaxResults = 5
	}

	emails, err := client.ListRecentMessages(ctx, args.Query, args.MaxResults)
	if err != nil {
		return err.Error(), true
	}
	if len(emails) == 0 {
		return "Tidak ada email yang cocok.", false
	}
	out := ""
	for _, e := range emails {
		out += fmt.Sprintf("- Dari: %s | Subjek: %s | %s\n", e.From, e.Subject, e.Snippet)
	}
	return out, false
}

func (a *Agent) toolSendEmail(ctx context.Context, input json.RawMessage) (string, bool) {
	client, errMsg, isErr := a.requireGoogle()
	if isErr {
		return errMsg, true
	}
	var args struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return err.Error(), true
	}
	if err := client.SendMessage(ctx, args.To, args.Subject, args.Body); err != nil {
		return err.Error(), true
	}
	return fmt.Sprintf("Email ke %s terkirim.", args.To), false
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
