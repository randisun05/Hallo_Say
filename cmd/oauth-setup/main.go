// Command oauth-setup runs the one-time interactive Google OAuth2
// authorization flow. Run this on a machine where you have a browser
// available (your own computer, not a headless server) — it opens a local
// callback server and prints a URL for you to visit.
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/randisun05/Hallo_Say/internal/google"
)

var scopes = []string{
	"https://www.googleapis.com/auth/calendar",
	"https://www.googleapis.com/auth/drive.readonly",
	"https://www.googleapis.com/auth/gmail.readonly",
	"https://www.googleapis.com/auth/gmail.send",
}

func main() {
	clientID := requireEnv("GOOGLE_CLIENT_ID")
	clientSecret := requireEnv("GOOGLE_CLIENT_SECRET")

	port := 8765
	if p := os.Getenv("OAUTH_REDIRECT_PORT"); p != "" {
		parsed, err := strconv.Atoi(p)
		if err != nil {
			log.Fatalf("OAUTH_REDIRECT_PORT tidak valid: %v", err)
		}
		port = parsed
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	tokenPath := filepath.Join(dataDir, "google-token.json")

	cfg := google.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       scopes,
		RedirectPort: port,
	}

	log.Println("Pastikan redirect URI berikut sudah didaftarkan di OAuth client kamu di Google Cloud Console:")
	log.Printf("  http://localhost:%d/callback\n", port)

	if err := google.Authorize(context.Background(), cfg, tokenPath); err != nil {
		log.Fatalf("otorisasi gagal: %v", err)
	}
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("env %s wajib diset", key)
	}
	return v
}
