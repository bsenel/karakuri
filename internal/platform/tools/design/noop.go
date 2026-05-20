package design

import (
	"context"
	"log/slog"
)

type NoOp struct{}

func NewNoOp() *NoOp { return &NoOp{} }

func (n *NoOp) Name() string { return "noop" }

func (n *NoOp) Active() bool { return false }

func (n *NoOp) GetFile(ctx context.Context, id string) (DesignFile, error) {
	slog.WarnContext(ctx, "DesignAdapter not configured", "id", id)
	return DesignFile{}, nil
}
