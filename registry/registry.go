// Package registry registers, looks up and revokes service instances in the
// Arrowhead ServiceRegistry through the AH5 service-discovery API
// (CONTRACT.md C2–C4).
//
// Nothing happens implicitly: a service is registered when you call Register,
// stays registered until you call Revoke (or it expires), and is found only
// when you call Lookup. There is no heartbeat, no re-registration and no cache.
package registry

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/models"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/transport"
)

// ServiceRegistry endpoints used by this package.
const (
	RegisterPath   = "/serviceregistry/service-discovery/register"
	LookupPath     = "/serviceregistry/service-discovery/lookup"
	RevokePathBase = "/serviceregistry/service-discovery/revoke/"
)

// Client talks to the ServiceRegistry. In the course kit the ServiceRegistry
// serves TLS only, so t is normally a transport.NewMTLS client with server
// name "serviceregistry".
type Client struct {
	t *transport.Client
}

// New returns a registry client that sends its requests through t.
func New(t *transport.Client) *Client {
	return &Client{t: t}
}

// Register registers reg, or updates it when an instance with the same system,
// service definition and version exists (C2). created is true for 201 and
// false for 200.
//
// token is the Authentication token of the system named in reg.SystemName
// (identity.Login). With registration auth on, no token or an invalid or
// expired token is a 401, and a token of another system is a 403. Pass "" only
// against a ServiceRegistry that has registration auth off.
//
// The ServiceRegistry refuses names that break the AH5 rules with 400; check
// them first with models.ValidateSystemName and
// models.ValidateServiceDefinitionName if you want a local error.
func (c *Client) Register(ctx context.Context, token string, reg models.ServiceRegistration) (inst models.ServiceInstance, created bool, err error) {
	resp, err := c.t.Do(ctx, transport.Request{Method: http.MethodPost, Path: RegisterPath, Token: token, Body: reg})
	if err != nil {
		return models.ServiceInstance{}, false, err
	}
	if err := resp.DecodeJSON(&inst); err != nil {
		return models.ServiceInstance{}, false, err
	}
	return inst, resp.StatusCode == http.StatusCreated, nil
}

// Lookup returns the service instances matching q (C3). At least one of
// q.InstanceIDs, q.ProviderNames or q.ServiceDefinitionNames must be set, or
// the ServiceRegistry answers 400. No match is a result with no entries.
func (c *Client) Lookup(ctx context.Context, q models.ServiceLookup) (models.ServiceLookupResult, error) {
	resp, err := c.t.Do(ctx, transport.Request{Method: http.MethodPost, Path: LookupPath, Body: q})
	if err != nil {
		return models.ServiceLookupResult{}, err
	}
	var out models.ServiceLookupResult
	if err := resp.DecodeJSON(&out); err != nil {
		return models.ServiceLookupResult{}, err
	}
	return out, nil
}

// Revoke removes the service instance with the given ID (C4). removed is true
// for 200 and false for 204 (no such instance, not an error).
func (c *Client) Revoke(ctx context.Context, instanceID string) (removed bool, err error) {
	if instanceID == "" {
		return false, errors.New("registry: instance ID is empty")
	}
	resp, err := c.t.Do(ctx, transport.Request{Method: http.MethodDelete, Path: RevokePathBase + url.PathEscape(instanceID)})
	if err != nil {
		return false, err
	}
	return resp.StatusCode == http.StatusOK, nil
}

// HTTPInterface returns an interface description the orchestrator can turn
// into a pull result: accessAddresses → provider.address, accessPort →
// provider.port, basePath → service.serviceUri (CONTRACT.md C2). The port is
// written as a string, as the ServiceRegistry requires.
func HTTPInterface(templateName, address string, port int, basePath string) models.Interface {
	return models.Interface{
		TemplateName: templateName,
		Protocol:     "http",
		Policy:       models.PolicyNone,
		Properties: map[string]string{
			models.PropertyAccessAddresses: address,
			models.PropertyAccessPort:      strconv.Itoa(port),
			models.PropertyBasePath:        basePath,
		},
	}
}
