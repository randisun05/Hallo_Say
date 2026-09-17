// Command chat is a terminal REPL for testing the agent (LLM + reminders +
// Google tools) without needing a Telegram bot token. Useful for verifying
// a provider (e.g. a self-hosted Ollama server) actually works end to end.
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/randisun05/Hallo_Say/internal/agent"
	"github.com/randisun05/Hallo_Say/internal/appsetup"
	"github.com/randisun05/Hallo_Say/internal/reminder"
)

// localChatID is a fixed chat ID for the terminal session — reminders and
// conversation history are scoped to it just like a real Telegram chat.
const localChatID = 1

func main() {
	dataDir := appsetup.DataDir()

	store, err := reminder.NewStore(filepath.Join(dataDir, "reminders.json"))
	if err != nil {
		log.Fatalf("gagal inisialisasi reminder store: %v", err)
	}

	provider := appsetup.BuildProvider()
	googleClient := appsetup.BuildGoogleClient(dataDir)

	notifier := func(chatID int64, text string) {
		fmt.Printf("\n[reminder] %s\n> ", text)
	}

	ag := agent.New(provider, store, googleClient, notifier)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go ag.RunReminderScheduler(ctx, 15*time.Second)

	fmt.Println("Mode chat terminal — ngobrol langsung ke LLM yang dikonfigurasi (LLM_PROVIDER).")
	fmt.Println("Ctrl+C atau Ctrl+D untuk keluar.")

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")
	for scanner.Scan() {
		text := scanner.Text()
		if text == "" {
			fmt.Print("> ")
			continue
		}

		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		reply, err := ag.Handle(reqCtx, localChatID, text)
		cancel()

		if err != nil {
			fmt.Printf("error: %v\n", err)
		} else {
			fmt.Println(reply)
		}
		fmt.Print("> ")
	}
}
