package google

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const calendarBase = "https://www.googleapis.com/calendar/v3"

type CalendarEvent struct {
	ID      string    `json:"id,omitempty"`
	Summary string    `json:"summary"`
	Start   eventTime `json:"start"`
	End     eventTime `json:"end"`
}

type eventTime struct {
	DateTime string `json:"dateTime,omitempty"`
}

type eventsListResponse struct {
	Items []CalendarEvent `json:"items"`
}

// ListUpcomingEvents returns up to maxResults events on the primary
// calendar starting from now.
func (c *Client) ListUpcomingEvents(ctx context.Context, maxResults int) ([]CalendarEvent, error) {
	q := url.Values{
		"timeMin":      {time.Now().Format(time.RFC3339)},
		"maxResults":   {fmt.Sprintf("%d", maxResults)},
		"singleEvents": {"true"},
		"orderBy":      {"startTime"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, calendarBase+"/calendars/primary/events?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var out eventsListResponse
	if err := c.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// CreateEvent creates an event on the primary calendar.
func (c *Client) CreateEvent(ctx context.Context, summary string, start, end time.Time) (CalendarEvent, error) {
	body, err := json.Marshal(CalendarEvent{
		Summary: summary,
		Start:   eventTime{DateTime: start.Format(time.RFC3339)},
		End:     eventTime{DateTime: end.Format(time.RFC3339)},
	})
	if err != nil {
		return CalendarEvent{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, calendarBase+"/calendars/primary/events", strings.NewReader(string(body)))
	if err != nil {
		return CalendarEvent{}, err
	}
	req.Header.Set("content-type", "application/json")

	var out CalendarEvent
	if err := c.doJSON(req, &out); err != nil {
		return CalendarEvent{}, err
	}
	return out, nil
}
