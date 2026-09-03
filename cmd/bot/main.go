// Command bot runs the Telegram-based personal/work assistant.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/randisun05/Hallo_Say/internal/agent"
	"github.com/randisun05/Hallo_Say/internal/google"
	"github.com/randisun05/Hallo_Say/internal/llm"
	"github.com/randisun05/Hallo_Say/internal/llm/claude"
	"github.com/randisun05/Hallo_Say/internal/llm/gemini"
	"github.com/randisun05/Hallo_Say/internal/reminder"
	"github.com/randisun05/Hallo_Say/internal/telegram"
)

func main() {
	telegramToken := requireEnv("TELEGRAM_BOT_TOKEN")

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	allowedChatIDs := parseAllowedChatIDs(os.Getenv("ALLOWED_CHAT_IDS"))
	if len(allowedChatIDs) == 0 {
		log.Println("PERINGATAN: ALLOWED_CHAT_IDS tidak diset — bot akan merespons SEMUA chat yang mengirim pesan. Set env ini untuk membatasi ke chat pribadi kamu.")
	}

	tgClient := telegram.NewClient(telegramToken)

	store, err := reminder.NewStore(filepath.Join(dataDir, "reminders.json"))
	if err != nil {
		log.Fatalf("gagal inisialisasi reminder store: %v", err)
	}

	provider := buildProvider()
	googleClient := buildGoogleClient(dataDir)

	notifier := func(chatID int64, text string) {
		if err := tgClient.SendMessage(chatID, text); err != nil {
			log.Printf("gagal kirim reminder ke chat %d: %v", chatID, err)
		}
	}

	ag := agent.New(provider, store, googleClient, notifier)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go ag.RunReminderScheduler(ctx, 15*time.Second)

	log.Println("Bot berjalan, menunggu pesan...")
	runPollLoop(ctx, tgClient, ag, allowedChatIDs)
}

func runPollLoop(ctx context.Context, tgClient *telegram.Client, ag *agent.Agent, allowedChatIDs map[int64]bool) {
	var offset int64
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		updates, err := tgClient.GetUpdates(offset, 30)
		if err != nil {
			log.Printf("gagal ambil update: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}

		for _, u := range updates {
			offset = u.UpdateID + 1

			if u.Message == nil || strings.TrimSpace(u.Message.Text) == "" {
				continue
			}
			chatID := u.Message.Chat.ID

			if len(allowedChatIDs) > 0 && !allowedChatIDs[chatID] {
				log.Printf("pesan diabaikan dari chat tidak diizinkan: %d", chatID)
				continue
			}

			go handleMessage(ctx, tgClient, ag, chatID, u.Message.Text)
		}
	}
}

func handleMessage(ctx context.Context, tgClient *telegram.Client, ag *agent.Agent, chatID int64, text string) {
	reqCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	reply, err := ag.Handle(reqCtx, chatID, text)
	if err != nil {
		log.Printf("agent error untuk chat %d: %v", chatID, err)
		_ = tgClient.SendMessage(chatID, "Maaf, terjadi error saat memproses pesanmu. Coba lagi sebentar.")
		return
	}
	if err := tgClient.SendMessage(chatID, reply); err != nil {
		log.Printf("gagal kirim balasan ke chat %d: %v", chatID, err)
	}
}

func buildProvider() llm.Provider {
	name := os.Getenv("LLM_PROVIDER")
	if name == "" {
		name = "gemini"
	}

	switch name {
	case "gemini":
		apiKey := requireEnv("GEMINI_API_KEY")
		model := os.Getenv("GEMINI_MODEL")
		if model == "" {
			model = "gemini-2.5-flash"
		}
		log.Printf("LLM provider: gemini (%s)", model)
		return gemini.NewClient(apiKey, model)

	case "anthropic":
		apiKey := requireEnv("ANTHROPIC_API_KEY")
		model := os.Getenv("CLAUDE_MODEL")
		if model == "" {
			model = "claude-sonnet-5"
		}
		log.Printf("LLM provider: anthropic (%s)", model)
		return claude.NewClient(apiKey, model)

	default:
		log.Fatalf("LLM_PROVIDER tidak dikenal: %q (pakai \"gemini\" atau \"anthropic\")", name)
		return nil
	}
}

// buildGoogleClient wires up Calendar/Drive/Gmail tools if a token from
// `go run ./cmd/oauth-setup` is present. Returns nil (feature disabled) if
// not configured, rather than failing startup.
func buildGoogleClient(dataDir string) *google.Client {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		log.Println("GOOGLE_CLIENT_ID/GOOGLE_CLIENT_SECRET tidak diset — fitur Calendar/Drive/Gmail dimatikan.")
		return nil
	}

	tokenPath := filepath.Join(dataDir, "google-token.json")
	client, err := google.NewClient(google.Config{ClientID: clientID, ClientSecret: clientSecret}, tokenPath)
	if err != nil {
		log.Printf("Google client tidak aktif (%v) — fitur Calendar/Drive/Gmail dimatikan.", err)
		return nil
	}
	log.Println("Integrasi Google Calendar/Drive/Gmail aktif.")
	return client
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("env %s wajib diset", key)
	}
	return v
}

func parseAllowedChatIDs(raw string) map[int64]bool {
	out := make(map[int64]bool)
	if raw == "" {
		return out
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			log.Printf("mengabaikan ALLOWED_CHAT_IDS tidak valid: %q", part)
			continue
		}
		out[id] = true
	}
	return out
}
