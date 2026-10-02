// Package auth manages ConsumerAuthorization policies: which consumers may use
// a provider's service (CONTRACT.md C10–C12).
//
// In consumerauth mode the orchestrator asks ConsumerAuthorization about every
// candidate provider; a provider without a policy naming the consumer is left
// out of the pull result. A policy exists only after you call Grant, and is
// gone after Revoke. At the pinned stack these calls need no token: any
// system with a profile-ca certificate may grant, revoke and look up
// policies (CONTRACT.md, "Tokens").
package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/models"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/transport"
)

// ConsumerAuthorization endpoints used by this package.
const (
	GrantPath      = "/consumerauthorization/authorization/grant"
	RevokePathBase = "/consumerauthorization/authorization/revoke/"
	LookupPath     = "/consumerauthorization/authorization/lookup"
)

// Client talks to ConsumerAuthorization. In the course kit it serves TLS
// only, so t is normally a transport.NewMTLS client with server name
// "consumerauth".
type Client struct {
	t *transport.Client
}

// New returns an auth client that sends its requests through t.
func New(t *transport.Client) *Client {
	return &Client{t: t}
}

// AllowConsumers returns a grant request that lets exactly the named consumers
// use provider's serviceDefinition (a WHITELIST policy on SERVICE_DEF).
// It only builds the request; nothing is sent until Grant.
func AllowConsumers(provider, serviceDefinition string, consumers ...string) models.GrantRequest {
	return models.GrantRequest{
		Provider:      provider,
		TargetType:    models.TargetTypeServiceDef,
		Target:        serviceDefinition,
		DefaultPolicy: models.PolicyDef{PolicyType: models.PolicyTypeWhitelist, PolicyList: consumers},
	}
}

// Grant creates a policy (C10) and returns it as stored. A second policy for
// the same provider, target type and target is refused with 409.
func (c *Client) Grant(ctx context.Context, req models.GrantRequest) (models.AuthPolicy, error) {
	resp, err := c.t.Do(ctx, transport.Request{Method: http.MethodPost, Path: GrantPath, Body: req})
	if err != nil {
		return models.AuthPolicy{}, err
	}
	var out models.AuthPolicy
	if err := resp.DecodeJSON(&out); err != nil {
		return models.AuthPolicy{}, err
	}
	return out, nil
}

// Revoke removes the policy with the given instance ID (C11), for example
// models.PolicyInstanceID(provider, models.TargetTypeServiceDef, service).
// The "|" separators are percent-encoded. An unknown ID is a 404 error.
func (c *Client) Revoke(ctx context.Context, instanceID string) error {
	if instanceID == "" {
		return errors.New("auth: instance ID is empty")
	}
	_, err := c.t.Do(ctx, transport.Request{Method: http.MethodDelete, Path: RevokePathBase + url.PathEscape(instanceID)})
	return err
}

// Lookup returns the policies matching q (C12). At least one of
// q.InstanceIDs, q.CloudIdentifiers or q.TargetNames must be set, or
// ConsumerAuthorization answers 400.
func (c *Client) Lookup(ctx context.Context, q models.PolicyLookup) (models.PolicyLookupResult, error) {
	resp, err := c.t.Do(ctx, transport.Request{Method: http.MethodPost, Path: LookupPath, Body: q})
	if err != nil {
		return models.PolicyLookupResult{}, err
	}
	var out models.PolicyLookupResult
	if err := resp.DecodeJSON(&out); err != nil {
		return models.PolicyLookupResult{}, err
	}
	return out, nil
}
