// Package models holds the request and response types of every endpoint listed
// in CONTRACT.md, one set per row (C1–C17).
//
// The types are the SDK's own definitions of the wire format. JSON field names
// follow the stack SPECs exactly. Timestamps are kept as the strings the stack
// sends; parse them with time.Parse(time.RFC3339, …) when you need a time.Time.
//
// This package performs no I/O.
package models
