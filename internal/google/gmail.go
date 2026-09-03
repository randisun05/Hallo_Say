package google

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const gmailBase = "https://gmail.googleapis.com/gmail/v1/users/me"

type EmailSummary struct {
	ID      string
	Subject string
	From    string
	Snippet string
}

type messageListResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

type messageHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type messageMetadataResponse struct {
	Snippet string `json:"snippet"`
	Payload struct {
		Headers []messageHeader `json:"headers"`
	} `json:"payload"`
}

// ListRecentMessages returns up to maxResults recent messages matching the
// optional Gmail search query (empty query = inbox default).
func (c *Client) ListRecentMessages(ctx context.Context, query string, maxResults int) ([]EmailSummary, error) {
	q := url.Values{"maxResults": {fmt.Sprintf("%d", maxResults)}}
	if query != "" {
		q.Set("q", query)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, gmailBase+"/messages?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var list messageListResponse
	if err := c.doJSON(req, &list); err != nil {
		return nil, err
	}

	summaries := make([]EmailSummary, 0, len(list.Messages))
	for _, m := range list.Messages {
		metaReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
			gmailBase+"/messages/"+m.ID+"?format=metadata&metadataHeaders=Subject&metadataHeaders=From", nil)
		if err != nil {
			return nil, err
		}
		var meta messageMetadataResponse
		if err := c.doJSON(metaReq, &meta); err != nil {
			return nil, err
		}
		summary := EmailSummary{ID: m.ID, Snippet: meta.Snippet}
		for _, h := range meta.Payload.Headers {
			switch h.Name {
			case "Subject":
				summary.Subject = h.Value
			case "From":
				summary.From = h.Value
			}
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

// SendMessage sends a plain-text email from the authenticated account.
func (c *Client) SendMessage(ctx context.Context, to, subject, body string) error {
	raw := fmt.Sprintf("To: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=\"UTF-8\"\r\n\r\n%s", to, subject, body)
	encoded := base64.RawURLEncoding.EncodeToString([]byte(raw))

	payload := fmt.Sprintf(`{"raw":%q}`, encoded)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gmailBase+"/messages/send", strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")

	return c.doJSON(req, nil)
}
