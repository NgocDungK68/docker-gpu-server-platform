package domain

import "errors"

var (
	ErrNotFound        = errors.New("resource not found")
	ErrConflict        = errors.New("resource state conflict")
	ErrUnauthorized    = errors.New("unauthorized agent")
	ErrInvalidInput    = errors.New("invalid input")
	ErrStaleInventory  = errors.New("stale inventory report")
	ErrInsufficientGPU = errors.New("no server satisfies the GPU request")
	ErrJobNotStoppable = errors.New("job cannot be stopped in its current state")
)
