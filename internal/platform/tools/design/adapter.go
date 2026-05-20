package design

import "context"

type DesignFile struct {
	ID   string
	Name string
	URL  string
}

type DesignAdapter interface {
	Name() string
	GetFile(ctx context.Context, id string) (DesignFile, error)
	Active() bool
}
