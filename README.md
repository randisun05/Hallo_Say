# Hallo_Say

Asisten AI pribadi & kerja yang berjalan sebagai bot Telegram. Ditulis di Go
tanpa dependency eksternal (hanya standard library).

## Fitur

- **Tanya jawab & ringkas** — chat bebas ditangani langsung oleh LLM.
- **Reminder / to-do** — `set_reminder`, `list_reminders`, `cancel_reminder`,
  dikirim ulang otomatis saat waktunya tiba.
- **Bantuan kerja/coding** — bagian dari chat biasa, tidak perlu tool khusus.
- **Automation sederhana** — tool `call_webhook` untuk memanggil API/webhook
  eksternal atas permintaan user (diblokir untuk target jaringan privat).
- **Google Calendar** — lihat acara mendatang, buat acara baru (opsional).
- **Google Drive** — cari file berdasarkan nama (opsional).
- **Gmail** — lihat email terbaru, kirim email (opsional).

LLM yang dipakai bisa Google Gemini (ada free tier bulanan, jadi bisa dites
tanpa biaya), Anthropic Claude, atau Ollama yang di-self-host di VM/komputer
sendiri (mis. model Qwen2.5) — dipilih lewat `LLM_PROVIDER`.

## Setup dasar (chat & reminder)

1. Buat bot Telegram lewat [@BotFather](https://t.me/BotFather) dan ambil tokennya.
2. Ambil Gemini API key gratis di https://aistudio.google.com/apikey.
3. Salin `.env.example` menjadi `.env`, isi `TELEGRAM_BOT_TOKEN` dan
   `GEMINI_API_KEY`, lalu export:
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

Ganti `LLM_PROVIDER=anthropic` (plus `ANTHROPIC_API_KEY`) kapan pun kamu mau
pindah ke Claude — tidak ada perubahan kode yang dibutuhkan.

## Setup Ollama/Qwen self-hosted — jalur aktif saat ini

`.env.example` sudah di-set `LLM_PROVIDER=ollama` dengan `OLLAMA_BASE_URL`
dikosongkan — isi begitu VM-nya siap (`http://<ip-vm>:11434`), sisanya
tidak perlu diubah. Jalur ini dipakai kalau kamu mau LLM jalan di
VM/komputer sendiri (gratis tanpa kuota, data tidak keluar ke pihak
ketiga), bukan lewat API cloud.

Cocok untuk VM 8-core/16GB tanpa GPU dengan model **Qwen2.5 7B** — model
lebih besar (14B+) akan terlalu lambat/berat di RAM segitu.

1. Install Ollama di VM:
   ```sh
   curl -fsSL https://ollama.com/install.sh | sh
   ```
2. Unduh model Qwen2.5 (sekali saja, ~5GB):
   ```sh
   ollama pull qwen2.5:7b
   ```
3. Pastikan servernya jalan (`ollama serve`, biasanya otomatis jalan sebagai
   service setelah install) dan bisa diakses bot — default di
   `http://localhost:11434`.
4. Di `.env`, set:
   ```
   LLM_PROVIDER=ollama
   OLLAMA_BASE_URL=http://localhost:11434
   OLLAMA_MODEL=qwen2.5:7b
   ```
5. Jalankan bot seperti biasa (`go run ./cmd/bot`).

Catatan realistis: kualitas jawaban & akurasi tool-calling (reminder,
Calendar, dll.) Qwen2.5 7B di CPU jelas di bawah Gemini/Claude, dan
responsnya lebih lambat (beberapa detik–puluhan detik per balasan,
tergantung berapa kali tool dipanggil). Ini trade-off yang wajar untuk
dapat privasi penuh + tanpa biaya/kuota. Kalau butuh kualitas lebih baik
dengan hardware yang sama, coba juga `ollama pull qwen2.5:14b` (lebih berat,
lebih pintar) atau tetap pakai Gemini untuk kasus yang butuh akurasi tinggi.

## Setup Google (Calendar/Drive/Gmail) — opsional

Fitur ini butuh project Google Cloud sendiri karena harus login sebagai
akun Google kamu. Langkah-langkahnya:

1. Buka [Google Cloud Console](https://console.cloud.google.com/) → buat
   project baru (nama bebas).
2. Di **APIs & Services → Library**, aktifkan tiga API ini satu per satu:
   - Google Calendar API
   - Google Drive API
   - Gmail API
3. Di **APIs & Services → OAuth consent screen**:
   - User type: **External**
   - Isi nama app & email seperlunya
   - Di bagian **Test users**, tambahkan alamat Gmail kamu sendiri
     (selama app belum di-publish, cuma test user yang bisa login)
4. Di **APIs & Services → Credentials → Create Credentials → OAuth client ID**:
   - Application type: **Web application**
   - Authorized redirect URIs: tambahkan `http://localhost:8765/callback`
   - Setelah dibuat, catat **Client ID** dan **Client Secret**
5. Isi `GOOGLE_CLIENT_ID` dan `GOOGLE_CLIENT_SECRET` di `.env`.
6. Jalankan flow otorisasi satu kali **di komputer kamu sendiri** (bukan
   server headless), karena langkah ini butuh buka browser:
   ```sh
   export $(grep -v '^#' .env | xargs)
   go run ./cmd/oauth-setup
   ```
   Buka URL yang dicetak di terminal, login, izinkan akses. Token hasil
   login tersimpan di `data/google-token.json`.
7. Jalankan bot seperti biasa (`go run ./cmd/bot`) dari direktori yang sama
   (atau salin `data/google-token.json` ke server tempat bot dijalankan).
   Kalau token & client ID/secret terbaca, log akan menampilkan
   "Integrasi Google Calendar/Drive/Gmail aktif."

Tanpa `GOOGLE_CLIENT_ID`/`GOOGLE_CLIENT_SECRET`, bot tetap jalan normal
dengan fitur Google dinonaktifkan.

## Struktur

```
cmd/bot/               entry point: polling Telegram, wiring semua komponen
cmd/oauth-setup/        setup interaktif sekali-jalan untuk otorisasi Google
internal/telegram/      klien Telegram Bot API (long polling + kirim pesan)
internal/llm/            tipe & interface provider LLM yang generik
internal/llm/claude/     implementasi provider untuk Anthropic Claude
internal/llm/gemini/     implementasi provider untuk Google Gemini
internal/llm/ollama/     implementasi provider untuk Ollama self-hosted (Qwen, dll.)
internal/agent/          orkestrasi percakapan + tool loop + system prompt
internal/reminder/       penyimpanan reminder berbasis file JSON
internal/webhook/        eksekusi tool call_webhook dengan guard SSRF dasar
internal/google/         OAuth2 + REST client untuk Calendar/Drive/Gmail
```

## Menambah kemampuan baru

Tambahkan tool baru di `internal/agent/agent.go` (definisi di `baseTools`
atau `googleTools`, dan implementasinya di `executeTool`) — tidak perlu ubah
bagian lain. Untuk provider LLM baru, implementasikan `llm.Provider` di
`internal/llm/<nama>/` mengikuti pola `internal/llm/gemini`.

## Testing

```sh
go build ./...
go vet ./...
go test ./...
```

Unit test yang ada mengecek logika inti tanpa perlu API key/server sungguhan:
penyimpanan reminder, guard `call_webhook` terhadap target jaringan privat,
dan konversi format tool-calling Gemini maupun Ollama. Pengujian end-to-end
lewat Telegram tetap butuh `TELEGRAM_BOT_TOKEN` dan LLM asli (API key untuk
Gemini/Claude, atau server Ollama yang benar-benar jalan).
