package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Google is a CalendarAdapter backed by the Google Calendar API v3.
// Auth uses a Bearer OAuth 2.0 access token — minted upstream (gcloud,
// oauth2l, your own OAuth dance) and supplied via env or config.
type Google struct {
	oauthToken         string
	defaultCalendarID  string
	client             *http.Client
}

func NewGoogle(oauthToken, defaultCalendarID string) *Google {
	if defaultCalendarID == "" {
		defaultCalendarID = "primary"
	}
	return &Google{
		oauthToken:        oauthToken,
		defaultCalendarID: defaultCalendarID,
		client:            &http.Client{Timeout: 30 * time.Second},
	}
}

func (g *Google) Name() string { return "google" }

func (g *Google) Active() bool { return g.oauthToken != "" }

func (g *Google) ListEvents(ctx context.Context, calendarID string, from, to time.Time) ([]Event, error) {
	if calendarID == "" {
		calendarID = g.defaultCalendarID
	}
	q := url.Values{}
	q.Set("singleEvents", "true")
	q.Set("orderBy", "startTime")
	if !from.IsZero() {
		q.Set("timeMin", from.UTC().Format(time.RFC3339))
	}
	if !to.IsZero() {
		q.Set("timeMax", to.UTC().Format(time.RFC3339))
	}
	path := fmt.Sprintf("/calendar/v3/calendars/%s/events?%s", url.PathEscape(calendarID), q.Encode())

	var raw struct {
		Items []googleEvent `json:"items"`
	}
	if err := g.do(ctx, "GET", path, nil, &raw); err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(raw.Items))
	for _, ge := range raw.Items {
		out = append(out, ge.toEvent())
	}
	return out, nil
}

func (g *Google) CreateEvent(ctx context.Context, calendarID string, event Event) (string, error) {
	if calendarID == "" {
		calendarID = g.defaultCalendarID
	}
	body := googleEventInput(event)
	var resp googleEvent
	path := fmt.Sprintf("/calendar/v3/calendars/%s/events", url.PathEscape(calendarID))
	if err := g.do(ctx, "POST", path, body, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

// ── HTTP helper ──────────────────────────────────────────────────────────────

func (g *Google) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://www.googleapis.com"+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.oauthToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("google-calendar: %s %s -> %d: %s", method, path, resp.StatusCode, string(respBody))
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}

// ── Google Calendar v3 wire types ────────────────────────────────────────────

type googleEvent struct {
	ID          string             `json:"id,omitempty"`
	Summary     string             `json:"summary,omitempty"`
	Description string             `json:"description,omitempty"`
	Location    string             `json:"location,omitempty"`
	Start       googleEventTime    `json:"start,omitempty"`
	End         googleEventTime    `json:"end,omitempty"`
	Attendees   []googleAttendee   `json:"attendees,omitempty"`
}

type googleEventTime struct {
	DateTime string `json:"dateTime,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}

type googleAttendee struct {
	Email string `json:"email"`
}

func (ge googleEvent) toEvent() Event {
	start, _ := time.Parse(time.RFC3339, ge.Start.DateTime)
	end, _ := time.Parse(time.RFC3339, ge.End.DateTime)
	attendees := make([]string, 0, len(ge.Attendees))
	for _, a := range ge.Attendees {
		attendees = append(attendees, a.Email)
	}
	return Event{
		ID:          ge.ID,
		Title:       ge.Summary,
		Description: ge.Description,
		Location:    ge.Location,
		Start:       start,
		End:         end,
		Attendees:   attendees,
	}
}

func googleEventInput(e Event) googleEvent {
	attendees := make([]googleAttendee, 0, len(e.Attendees))
	for _, a := range e.Attendees {
		attendees = append(attendees, googleAttendee{Email: a})
	}
	return googleEvent{
		Summary:     e.Title,
		Description: e.Description,
		Location:    e.Location,
		Start:       googleEventTime{DateTime: e.Start.UTC().Format(time.RFC3339)},
		End:         googleEventTime{DateTime: e.End.UTC().Format(time.RFC3339)},
		Attendees:   attendees,
	}
}
