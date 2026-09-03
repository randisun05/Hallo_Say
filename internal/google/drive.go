package google

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const driveBase = "https://www.googleapis.com/drive/v3"

type DriveFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
}

type driveFilesListResponse struct {
	Files []DriveFile `json:"files"`
}

// SearchFiles finds files whose name contains query (case-insensitive),
// most recently modified first.
func (c *Client) SearchFiles(ctx context.Context, query string, maxResults int) ([]DriveFile, error) {
	escaped := strings.ReplaceAll(query, "'", "\\'")
	q := url.Values{
		"q":        {fmt.Sprintf("name contains '%s' and trashed = false", escaped)},
		"pageSize": {fmt.Sprintf("%d", maxResults)},
		"fields":   {"files(id,name,mimeType)"},
		"orderBy":  {"modifiedTime desc"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, driveBase+"/files?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var out driveFilesListResponse
	if err := c.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out.Files, nil
}
