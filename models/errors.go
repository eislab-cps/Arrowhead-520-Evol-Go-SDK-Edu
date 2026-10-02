package models

// ErrorEnvelope is the AH5 error body sent by the foundation systems
// (ServiceRegistry, Authentication, ConsumerAuthorization) and by the
// orchestrator's management guard (CONTRACT.md, "Error bodies").
//
// Origin may be empty: the stack's shared helpers send "" for invalid-JSON 400
// and wrong-method 405 errors. Do not rely on it.
type ErrorEnvelope struct {
	ErrorMessage  string `json:"errorMessage"`
	ErrorCode     int    `json:"errorCode"`
	ExceptionType string `json:"exceptionType"`
	Origin        string `json:"origin"`
}

// SimpleError is the error body sent by profile-ca: {"error": "..."}.
type SimpleError struct {
	Error string `json:"error"`
}

// Exception types carried in ErrorEnvelope.ExceptionType.
const (
	ExceptionInvalidParameter = "INVALID_PARAMETER"   // 400
	ExceptionAuth             = "AUTH_EXCEPTION"      // 401
	ExceptionForbidden        = "FORBIDDEN"           // 403
	ExceptionDataNotFound     = "DATA_NOT_FOUND"      // 404
	ExceptionLocked           = "LOCKED"              // 423
	ExceptionNotImplemented   = "NOT_IMPLEMENTED"     // 501
	ExceptionArrowhead        = "ARROWHEAD_EXCEPTION" // any other status
)
