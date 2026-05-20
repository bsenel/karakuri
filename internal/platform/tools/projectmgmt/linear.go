package projectmgmt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Linear is a ProjectManagementAdapter backed by Linear's GraphQL API.
type Linear struct {
	apiKey string
	teamID string
	client *http.Client
}

func NewLinear(apiKey, teamID string) *Linear {
	return &Linear{
		apiKey: apiKey,
		teamID: teamID,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (l *Linear) Name() string { return "linear" }

func (l *Linear) Active() bool { return l.apiKey != "" }

const linearGetIssueQuery = `query($id: String!) {
  issue(id: $id) {
    id
    title
    description
  }
}`

const linearCreateIssueMutation = `mutation($input: IssueCreateInput!) {
  issueCreate(input: $input) {
    success
    issue { id }
  }
}`

func (l *Linear) GetTicket(ctx context.Context, id string) (Ticket, error) {
	var resp struct {
		Data struct {
			Issue *struct {
				ID          string `json:"id"`
				Title       string `json:"title"`
				Description string `json:"description"`
			} `json:"issue"`
		} `json:"data"`
		Errors []linearError `json:"errors"`
	}
	if err := l.do(ctx, linearGetIssueQuery, map[string]any{"id": id}, &resp); err != nil {
		return Ticket{}, err
	}
	if len(resp.Errors) > 0 {
		return Ticket{}, fmt.Errorf("linear: %s", resp.Errors[0].Message)
	}
	if resp.Data.Issue == nil {
		return Ticket{}, fmt.Errorf("linear: ticket %s not found", id)
	}
	return Ticket{ID: resp.Data.Issue.ID, Title: resp.Data.Issue.Title, Body: resp.Data.Issue.Description}, nil
}

func (l *Linear) CreateTicket(ctx context.Context, ticket Ticket) (string, error) {
	if l.teamID == "" {
		return "", fmt.Errorf("linear: team_id required to create tickets")
	}
	var resp struct {
		Data struct {
			IssueCreate struct {
				Success bool `json:"success"`
				Issue   *struct {
					ID string `json:"id"`
				} `json:"issue"`
			} `json:"issueCreate"`
		} `json:"data"`
		Errors []linearError `json:"errors"`
	}
	input := map[string]any{
		"teamId":      l.teamID,
		"title":       ticket.Title,
		"description": ticket.Body,
	}
	if err := l.do(ctx, linearCreateIssueMutation, map[string]any{"input": input}, &resp); err != nil {
		return "", err
	}
	if len(resp.Errors) > 0 {
		return "", fmt.Errorf("linear: %s", resp.Errors[0].Message)
	}
	if !resp.Data.IssueCreate.Success || resp.Data.IssueCreate.Issue == nil {
		return "", fmt.Errorf("linear: issueCreate did not succeed")
	}
	return resp.Data.IssueCreate.Issue.ID, nil
}

type linearError struct {
	Message string `json:"message"`
}

func (l *Linear) do(ctx context.Context, query string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.linear.app/graphql", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", l.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("linear: HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return json.Unmarshal(respBody, out)
}
