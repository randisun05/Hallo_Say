// Package telegram is a minimal long-polling Telegram Bot API client,
// implemented with the standard library only.
package telegram

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const maxMessageLength = 4096

type Client struct {
	token      string
	httpClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		token:      token,
		httpClient: &http.Client{Timeout: 40 * time.Second},
	}
}

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	Text      string `json:"text"`
	Chat      Chat   `json:"chat"`
	From      *User  `json:"from"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type apiResponse[T any] struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	Result      T      `json:"result"`
}

func (c *Client) methodURL(method string) string {
	return fmt.Sprintf("https://api.telegram.org/bot%s/%s", c.token, method)
}

// GetUpdates long-polls for new updates starting after offset.
func (c *Client) GetUpdates(offset int64, timeoutSeconds int) ([]Update, error) {
	q := url.Values{}
	q.Set("offset", fmt.Sprintf("%d", offset))
	q.Set("timeout", fmt.Sprintf("%d", timeoutSeconds))

	httpClient := &http.Client{Timeout: time.Duration(timeoutSeconds+10) * time.Second}
	resp, err := httpClient.Get(c.methodURL("getUpdates") + "?" + q.Encode())
	if err != nil {
		return nil, fmt.Errorf("getUpdates: %w", err)
	}
	defer resp.Body.Close()

	var parsed apiResponse[[]Update]
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode getUpdates response: %w", err)
	}
	if !parsed.OK {
		return nil, fmt.Errorf("getUpdates failed: %s", parsed.Description)
	}
	return parsed.Result, nil
}

// SendMessage sends text to a chat, splitting it into multiple messages if
// it exceeds Telegram's per-message length limit.
func (c *Client) SendMessage(chatID int64, text string) error {
	if text == "" {
		return nil
	}
	for len(text) > 0 {
		chunk := text
		if len(chunk) > maxMessageLength {
			chunk = chunk[:maxMessageLength]
		}
		text = text[len(chunk):]

		q := url.Values{}
		q.Set("chat_id", fmt.Sprintf("%d", chatID))
		q.Set("text", chunk)

		resp, err := c.httpClient.PostForm(c.methodURL("sendMessage"), q)
		if err != nil {
			return fmt.Errorf("sendMessage: %w", err)
		}
		var parsed apiResponse[json.RawMessage]
		decodeErr := json.NewDecoder(resp.Body).Decode(&parsed)
		resp.Body.Close()
		if decodeErr != nil {
			return fmt.Errorf("decode sendMessage response: %w", decodeErr)
		}
		if !parsed.OK {
			return fmt.Errorf("sendMessage failed: %s", parsed.Description)
		}
	}
	return nil
}
