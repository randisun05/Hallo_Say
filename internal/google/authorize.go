package google

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Authorize runs the one-time interactive OAuth2 flow: it starts a local
// callback server, prints the consent URL for the user to open in their own
// browser, waits for the redirect, exchanges the code for tokens, and saves
// them to tokenPath. Meant to be run manually (cmd/oauth-setup), on the
// machine where the user has a browser available.
func Authorize(ctx context.Context, cfg Config, tokenPath string) error {
	state, err := randomState()
	if err != nil {
		return err
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "state tidak cocok", http.StatusBadRequest)
			errCh <- fmt.Errorf("state mismatch pada callback")
			return
		}
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			http.Error(w, "otorisasi ditolak: "+errMsg, http.StatusBadRequest)
			errCh <- fmt.Errorf("otorisasi ditolak: %s", errMsg)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "kode otorisasi tidak ditemukan", http.StatusBadRequest)
			errCh <- fmt.Errorf("kode otorisasi tidak ditemukan pada callback")
			return
		}
		fmt.Fprint(w, "Otorisasi berhasil, kamu boleh menutup tab ini dan kembali ke terminal.")
		codeCh <- code
	})

	server := &http.Server{Addr: fmt.Sprintf(":%d", cfg.RedirectPort), Handler: mux}
	serverErrCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrCh <- err
		}
	}()
	defer server.Close()

	authURL := buildAuthURL(cfg, state)
	fmt.Println("Buka URL berikut di browser kamu, login, dan izinkan akses:")
	fmt.Println()
	fmt.Println(authURL)
	fmt.Println()
	fmt.Println("Menunggu otorisasi...")

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return err
	case err := <-serverErrCh:
		return fmt.Errorf("gagal menjalankan local callback server: %w", err)
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Minute):
		return fmt.Errorf("timeout menunggu otorisasi (5 menit)")
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"redirect_uri":  {cfg.redirectURI()},
	}
	tok, err := requestToken(ctx, form)
	if err != nil {
		return fmt.Errorf("tukar kode dengan token: %w", err)
	}
	if tok.RefreshToken == "" {
		return fmt.Errorf("tidak dapat refresh_token dari Google — cabut akses aplikasi ini di myaccount.google.com/permissions lalu coba lagi (Google hanya mengirim refresh_token pada otorisasi pertama)")
	}

	if err := saveToken(tokenPath, tok); err != nil {
		return err
	}
	fmt.Println("Token tersimpan di", tokenPath)
	return nil
}

func buildAuthURL(cfg Config, state string) string {
	q := url.Values{
		"client_id":     {cfg.ClientID},
		"redirect_uri":  {cfg.redirectURI()},
		"response_type": {"code"},
		"scope":         {strings.Join(cfg.Scopes, " ")},
		"access_type":   {"offline"},
		"prompt":        {"consent"},
		"state":         {state},
	}
	return authEndpoint + "?" + q.Encode()
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
