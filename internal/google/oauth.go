// Package google provides a minimal OAuth2 "installed app" flow for
// Google APIs (Calendar, Drive, Gmail) plus thin REST wrappers, using the
// standard library only.
package google

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	authEndpoint  = "https://accounts.google.com/o/oauth2/v2/auth"
	tokenEndpoint = "https://oauth2.googleapis.com/token"
)

type Config struct {
	ClientID     string
	ClientSecret string
	Scopes       []string
	RedirectPort int
}

func (c Config) redirectURI() string {
	return fmt.Sprintf("http://localhost:%d/callback", c.RedirectPort)
}

type token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
}

// Client is an authenticated HTTP client for Google APIs that transparently
// refreshes its access token using a stored refresh token.
type Client struct {
	cfg       Config
	tokenPath string
	http      *http.Client

	mu    sync.Mutex
	token token
}

// NewClient loads a previously saved token (created by Authorize) and
// returns a client ready to call Google APIs.
func NewClient(cfg Config, tokenPath string) (*Client, error) {
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("baca token file %s: %w (jalankan `go run ./cmd/oauth-setup` dulu)", tokenPath, err)
	}
	var tok token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("parse token file: %w", err)
	}

	c := &Client{cfg: cfg, tokenPath: tokenPath, token: tok}
	c.http = &http.Client{
		Timeout:   20 * time.Second,
		Transport: &authTransport{base: http.DefaultTransport, client: c},
	}
	return c, nil
}

// HTTPClient returns an *http.Client that injects a valid access token into
// every request, refreshing it as needed.
func (c *Client) HTTPClient() *http.Client {
	return c.http
}

type authTransport struct {
	base   http.RoundTripper
	client *Client
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	accessToken, err := t.client.validAccessToken(req.Context())
	if err != nil {
		return nil, err
	}
	cloned := req.Clone(req.Context())
	cloned.Header.Set("Authorization", "Bearer "+accessToken)
	return t.base.RoundTrip(cloned)
}

func (c *Client) validAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token.AccessToken != "" && time.Now().Before(c.token.Expiry.Add(-1*time.Minute)) {
		return c.token.AccessToken, nil
	}
	if c.token.RefreshToken == "" {
		return "", fmt.Errorf("tidak ada refresh token tersimpan, jalankan `go run ./cmd/oauth-setup` ulang")
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {c.token.RefreshToken},
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
	}
	newTok, err := requestToken(ctx, form)
	if err != nil {
		return "", fmt.Errorf("refresh token gagal: %w", err)
	}
	if newTok.RefreshToken == "" {
		newTok.RefreshToken = c.token.RefreshToken
	}
	c.token = newTok
	if err := saveToken(c.tokenPath, c.token); err != nil {
		return "", err
	}
	return c.token.AccessToken, nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func requestToken(ctx context.Context, form url.Values) (token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return token{}, err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return token{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return token{}, err
	}

	var parsed tokenResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return token{}, fmt.Errorf("parse token response: %w", err)
	}
	if parsed.Error != "" {
		return token{}, fmt.Errorf("%s: %s", parsed.Error, parsed.ErrorDesc)
	}

	return token{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(parsed.ExpiresIn) * time.Second),
	}, nil
}

func saveToken(path string, tok token) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("buat data dir: %w", err)
		}
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
