// Package identity logs a system in to the Arrowhead Authentication system
// (CONTRACT.md C1) and returns the token.
//
// The token is a plain value. The SDK never stores, attaches or refreshes it:
// pass it explicitly to the calls that need it (registration, management).
// When it expires, those calls answer 401 and you call Login again.
package identity

import (
	"context"
	"errors"
	"net/http"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/models"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/transport"
)

// LoginPath is the Authentication endpoint of CONTRACT.md C1.
const LoginPath = "/authentication/identity/login"

// Client talks to the Authentication system. In the course kit Authentication
// serves TLS only, so t is normally a transport.NewMTLS client with server
// name "authentication".
type Client struct {
	t *transport.Client
}

// New returns an identity client that sends its requests through t.
func New(t *transport.Client) *Client {
	return &Client{t: t}
}

// Login exchanges a system name and password for a token (C1).
// A wrong password or unknown system is a *transport.Error with status 401.
func (c *Client) Login(ctx context.Context, systemName, password string) (models.LoginResponse, error) {
	if systemName == "" {
		return models.LoginResponse{}, errors.New("identity: system name is empty")
	}
	resp, err := c.t.Do(ctx, transport.Request{
		Method: http.MethodPost,
		Path:   LoginPath,
		Body:   models.LoginRequest{SystemName: systemName, Credentials: models.Credentials{Password: password}},
	})
	if err != nil {
		return models.LoginResponse{}, err
	}
	var out models.LoginResponse
	if err := resp.DecodeJSON(&out); err != nil {
		return models.LoginResponse{}, err
	}
	return out, nil
}
