package errors

import "errors"

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
	ErrConstraintViolation = errors.New("constraint violation")
	ErrLoopMaxIter         = errors.New("loop max iterations reached")
	ErrProviderUnavailable = errors.New("provider unavailable")
)
