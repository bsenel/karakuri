package agent

import "context"

type Factory interface {
	New(ctx context.Context, def Definition) (Agent, error)
}
