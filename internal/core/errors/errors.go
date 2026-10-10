package errors

import (
	"errors"
	"time"
)

var (
	ErrNotImplemented      = errors.New("not implemented")
	ErrNotFound            = errors.New("not found")
	ErrInvalidInput        = errors.New("invalid input")
	ErrConflict            = errors.New("conflict")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrCapabilityNotFound  = errors.New("capability not found")
	ErrObjectiveNotFound   = errors.New("objective not found")
	ErrTwinNotFound        = errors.New("twin not found")
	ErrCheckpointNotFound  = errors.New("checkpoint not found")
	ErrContainerNotFound   = errors.New("container not found")
	ErrConstraintViolation = errors.New("constraint violation")
	ErrLoopMaxIter         = errors.New("loop max iterations reached")
	ErrProviderUnavailable = errors.New("provider unavailable")
	// ErrProviderRateLimited is the model provider answering a call with a
	// rate limit. Match with errors.Is; the provider and any retry-after are
	// on *ProviderRateLimitError, read with errors.As.
	ErrProviderRateLimited = errors.New("provider rate limited")
)

// ProviderRateLimitError says which provider refused and, when its answer
// said so, how long it asked the caller to wait. RetryAfter is zero when the
// answer gave none.
type ProviderRateLimitError struct {
	Provider   string
	RetryAfter time.Duration
	Cause      error
}

func (e *ProviderRateLimitError) Error() string {
	msg := e.Provider + ": " + ErrProviderRateLimited.Error()
	if e.RetryAfter > 0 {
		msg += " (retry after " + e.RetryAfter.String() + ")"
	}
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

func (e *ProviderRateLimitError) Is(target error) bool { return target == ErrProviderRateLimited }

func (e *ProviderRateLimitError) Unwrap() error { return e.Cause }
