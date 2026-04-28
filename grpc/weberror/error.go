package weberror

import (
	"fmt"
	"net/http"
)

type RequestError struct {
	StatusCode int
	Message    string
	Err        error
}

func (e *RequestError) Error() string {
	switch {
	case e == nil:
		return ""
	case e.Err != nil && e.Message != "":
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	case e.Message != "":
		return e.Message
	case e.Err != nil:
		return e.Err.Error()
	default:
		return "bank request failed"
	}
}

func (e *RequestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// This will determine only the retrybale errors which are either 5xx or 429 or network errors
func (e *RequestError) Retryable() bool {
	if e == nil {
		return false
	}
	if e.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if e.StatusCode >= http.StatusInternalServerError {
		return true
	}
	return false
}
