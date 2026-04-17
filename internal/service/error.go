package service

import (
	"errors"
	"fmt"
)

type ServiceError struct {
	Err     error
	Code    int
	Message string
}

// Error represents an error that occurred in the community package.
func (e *ServiceError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("error code: %d, message: %s, err: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("error code: %d, message: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error, if any.
func (e *ServiceError) Unwrap() error {
	return e.Err
}

var (
	ErrInvalidJobID      = errors.New("invalid job ID")
	ErrEmptyJobID        = errors.New("job ID cannot be empty")
	ErrInvalidTransition = errors.New("invalid status transition")
)
