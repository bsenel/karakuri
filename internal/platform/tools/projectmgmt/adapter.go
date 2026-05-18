package projectmgmt

import "context"

type Ticket struct {
	ID    string
	Title string
	Body  string
}

type ProjectManagementAdapter interface {
	GetTicket(ctx context.Context, id string) (Ticket, error)
	CreateTicket(ctx context.Context, ticket Ticket) (string, error)
	Active() bool
}
