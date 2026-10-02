package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/models"
)

// Error is a non-2xx answer. Body is always the raw body. Envelope is set when
// the body is the AH5 error envelope; Message holds the best human-readable
// text whatever the body shape (envelope, profile-ca {"error"}, or plain text).
type Error struct {
	Method     string
	URL        string
	StatusCode int
	Body       []byte
	Envelope   *models.ErrorEnvelope
	Message    string
}

func newError(method, url string, status int, body []byte) *Error {
	e := &Error{Method: method, URL: url, StatusCode: status, Body: body}
	var probe map[string]json.RawMessage
	if json.Unmarshal(body, &probe) == nil {
		if _, ok := probe["errorMessage"]; ok {
			var env models.ErrorEnvelope
			if json.Unmarshal(body, &env) == nil {
				e.Envelope = &env
				e.Message = env.ErrorMessage
			}
		} else if _, ok := probe["error"]; ok {
			var se models.SimpleError
			if json.Unmarshal(body, &se) == nil {
				e.Message = se.Error
			}
		}
	}
	if e.Message == "" {
		e.Message = strings.TrimSpace(string(body))
	}
	return e
}

func (e *Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("%s %s: status %d", e.Method, e.URL, e.StatusCode)
	}
	return fmt.Sprintf("%s %s: status %d: %s", e.Method, e.URL, e.StatusCode, e.Message)
}

// StatusCode returns the HTTP status carried by err, or 0 when err is not
// (and does not wrap) a *Error.
func StatusCode(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.StatusCode
	}
	return 0
}
