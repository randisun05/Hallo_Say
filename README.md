# Hallo_Say

Asisten AI pribadi & kerja yang berjalan sebagai bot Telegram. Ditulis di Go
tanpa dependency eksternal (hanya standard library).

## Fitur

- **Tanya jawab & ringkas** — chat bebas ditangani langsung oleh Claude.
- **Reminder / to-do** — `set_reminder`, `list_reminders`, `cancel_reminder`,
  dikirim ulang otomatis saat waktunya tiba.
- **Bantuan kerja/coding** — bagian dari chat biasa, tidak perlu tool khusus.
- **Automation sederhana** — tool `call_webhook` untuk memanggil API/webhook
  eksternal atas permintaan user (diblokir untuk target jaringan privat).

## Setup

1. Buat bot Telegram lewat [@BotFather](https://t.me/BotFather) dan ambil tokennya.
2. Siapkan Anthropic API key.
3. Salin `.env.example` menjadi `.env` dan isi nilainya, lalu export:
   ```sh
   export $(grep -v '^#' .env | xargs)
   ```
4. Jalankan bot:
   ```sh
   go run ./cmd/bot
   ```
5. Kirim pesan apa saja ke bot dari Telegram. Chat ID kamu akan muncul di log
   kalau `ALLOWED_CHAT_IDS` belum diisi — isi env itu lalu restart bot supaya
   hanya kamu yang bisa memakainya.

## Struktur

```
cmd/bot/            entry point: polling Telegram, wiring semua komponen
internal/telegram/  klien Telegram Bot API (long polling + kirim pesan)
internal/claude/    klien Anthropic Messages API (tool use)
internal/agent/     orkestrasi percakapan + tool loop + system prompt
internal/reminder/  penyimpanan reminder berbasis file JSON
internal/webhook/   eksekusi tool call_webhook dengan guard SSRF dasar
```

## Menambah kemampuan baru

Tambahkan tool baru di `internal/agent/agent.go` (definisi di slice `tools`
dan implementasinya di `executeTool`) — tidak perlu ubah bagian lain.
