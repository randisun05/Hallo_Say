// Package appsetup builds the shared LLM provider and Google client from
// environment variables, used by both cmd/bot and cmd/chat so the two
// entry points stay configured identically.
package appsetup

import (
	"log"
	"os"
	"path/filepath"

	"github.com/randisun05/Hallo_Say/internal/google"
	"github.com/randisun05/Hallo_Say/internal/llm"
	"github.com/randisun05/Hallo_Say/internal/llm/claude"
	"github.com/randisun05/Hallo_Say/internal/llm/gemini"
	"github.com/randisun05/Hallo_Say/internal/llm/ollama"
)

func RequireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("env %s wajib diset", key)
	}
	return v
}

func BuildProvider() llm.Provider {
	name := os.Getenv("LLM_PROVIDER")
	if name == "" {
		name = "gemini"
	}

	switch name {
	case "gemini":
		apiKey := RequireEnv("GEMINI_API_KEY")
		model := os.Getenv("GEMINI_MODEL")
		if model == "" {
			model = "gemini-3.6-flash"
		}
		log.Printf("LLM provider: gemini (%s)", model)
		return gemini.NewClient(apiKey, model)

	case "anthropic":
		apiKey := RequireEnv("ANTHROPIC_API_KEY")
		model := os.Getenv("CLAUDE_MODEL")
		if model == "" {
			model = "claude-sonnet-5"
		}
		log.Printf("LLM provider: anthropic (%s)", model)
		return claude.NewClient(apiKey, model)

	case "ollama":
		baseURL := os.Getenv("OLLAMA_BASE_URL")
		model := os.Getenv("OLLAMA_MODEL")
		if model == "" {
			model = "qwen2.5:7b"
		}
		log.Printf("LLM provider: ollama (%s @ %s)", model, baseURL)
		return ollama.NewClient(baseURL, model)

	default:
		log.Fatalf("LLM_PROVIDER tidak dikenal: %q (pakai \"gemini\", \"anthropic\", atau \"ollama\")", name)
		return nil
	}
}

// BuildGoogleClient wires up Calendar/Drive/Gmail tools if a token from
// `go run ./cmd/oauth-setup` is present. Returns nil (feature disabled) if
// not configured, rather than failing startup.
func BuildGoogleClient(dataDir string) *google.Client {
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

func DataDir() string {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	return dataDir
}
