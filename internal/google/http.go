package google

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type googleAPIError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// doJSON executes req with the client's authenticated HTTP client and
// decodes a successful JSON response into out (which may be nil to
// discard the body).
func (c *Client) doJSON(req *http.Request, out interface{}) error {
	resp, err := c.HTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("request ke google api gagal: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("baca response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr googleAPIError
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Error.Message != "" {
			return fmt.Errorf("google api error (%d): %s", resp.StatusCode, apiErr.Error.Message)
		}
		return fmt.Errorf("google api error (%d): %s", resp.StatusCode, string(body))
	}

	if out == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}
