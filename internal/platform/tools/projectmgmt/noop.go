package projectmgmt

import (
	"context"
	"log/slog"
)

type NoOp struct{}

func NewNoOp() *NoOp { return &NoOp{} }

func (n *NoOp) Name() string { return "noop" }

func (n *NoOp) Active() bool { return false }

func (n *NoOp) GetTicket(ctx context.Context, id string) (Ticket, error) {
	slog.WarnContext(ctx, "ProjectManagementAdapter not configured", "id", id)
	return Ticket{ID: id}, nil
}

func (n *NoOp) CreateTicket(ctx context.Context, ticket Ticket) (string, error) {
	slog.WarnContext(ctx, "ProjectManagementAdapter not configured: ticket creation skipped", "title", ticket.Title)
	return "", nil
}
