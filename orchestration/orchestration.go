// Package orchestration asks the orchestrator (dynamicorch-xacml) for
// providers and manages push subscriptions (CONTRACT.md C5–C9).
//
// The orchestrator has no TLS listener at the pinned stack, so t is a
// transport.NewPlain client. The requester name in a pull is asserted, not
// authenticated: the orchestrator believes whatever name you send.
//
// Every call is explicit. Pull does not cache, Subscribe does not start a
// listener, and a push notification carries no providers: when one arrives,
// your code decides to call Pull.
package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/models"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/transport"
)

// Orchestrator endpoints used by this package.
const (
	PullPath             = "/serviceorchestration/orchestration/pull"
	SubscribePath        = "/serviceorchestration/orchestration/subscribe"
	UnsubscribePathBase  = "/serviceorchestration/orchestration/unsubscribe/"
	TriggerPath          = "/serviceorchestration/orchestration/mgmt/push/trigger"
	HistoryPath          = "/serviceorchestration/orchestration/mgmt/history/query"
	maxNotificationBytes = 64 << 10
)

// Client talks to the orchestrator.
type Client struct {
	t *transport.Client
}

// New returns an orchestration client that sends its requests through t.
func New(t *transport.Client) *Client {
	return &Client{t: t}
}

// PullRequest builds a pull request for requester asking for
// serviceDefinition. Add interface or metadata filters to the result if needed.
func PullRequest(requester, serviceDefinition string) models.OrchestrationRequest {
	return models.OrchestrationRequest{
		RequesterSystem:  models.OrchestrationSystem{SystemName: requester},
		RequestedService: models.RequestedService{ServiceDefinition: serviceDefinition},
	}
}

// Pull returns the providers the requester may use (C5). An empty
// Response is a normal answer: no provider is registered for the service, or
// ConsumerAuthorization allows none of them for this requester.
func (c *Client) Pull(ctx context.Context, req models.OrchestrationRequest) (models.OrchestrationResponse, error) {
	resp, err := c.t.Do(ctx, transport.Request{Method: http.MethodPost, Path: PullPath, Body: req})
	if err != nil {
		return models.OrchestrationResponse{}, err
	}
	var out models.OrchestrationResponse
	if err := resp.DecodeJSON(&out); err != nil {
		return models.OrchestrationResponse{}, err
	}
	return out, nil
}

// Subscribe stores a push subscription (C6). created is true for 201 and
// false for 200 (the owner/target pair existed and was overwritten; the id is
// kept). The orchestrator delivers notifications only when TriggerPush is
// called, to the plain-HTTP URL in req.NotifyInterface.
func (c *Client) Subscribe(ctx context.Context, req models.SubscriptionRequest) (sub models.Subscription, created bool, err error) {
	resp, err := c.t.Do(ctx, transport.Request{Method: http.MethodPost, Path: SubscribePath, Body: req})
	if err != nil {
		return models.Subscription{}, false, err
	}
	if err := resp.DecodeJSON(&sub); err != nil {
		return models.Subscription{}, false, err
	}
	return sub, resp.StatusCode == http.StatusCreated, nil
}

// Unsubscribe removes a subscription (C7). removed is true for 200 and false
// for 204 (no such subscription, not an error).
func (c *Client) Unsubscribe(ctx context.Context, subscriptionID string) (removed bool, err error) {
	if subscriptionID == "" {
		return false, errors.New("orchestration: subscription ID is empty")
	}
	resp, err := c.t.Do(ctx, transport.Request{Method: http.MethodDelete, Path: UnsubscribePathBase + url.PathEscape(subscriptionID)})
	if err != nil {
		return false, err
	}
	return resp.StatusCode == http.StatusOK, nil
}

// TriggerPush asks the orchestrator to notify one subscriber now (C8).
// sysopToken is a token of a sysop identity (identity.Login as Sysop); without
// one the orchestrator answers 401, with a non-sysop token 403. Delivery
// happens after the 200, asynchronously; History shows DELIVERED or FAILED.
func (c *Client) TriggerPush(ctx context.Context, sysopToken, subscriptionID string) (models.PushTriggerResponse, error) {
	resp, err := c.t.Do(ctx, transport.Request{Method: http.MethodPost, Path: TriggerPath, Token: sysopToken,
		Body: models.PushTriggerRequest{SubscriptionID: subscriptionID}})
	if err != nil {
		return models.PushTriggerResponse{}, err
	}
	var out models.PushTriggerResponse
	if err := resp.DecodeJSON(&out); err != nil {
		return models.PushTriggerResponse{}, err
	}
	return out, nil
}

// History returns every orchestration history entry (C9). It needs a sysop
// token, like TriggerPush.
func (c *Client) History(ctx context.Context, sysopToken string) (models.HistoryResponse, error) {
	resp, err := c.t.Do(ctx, transport.Request{Method: http.MethodPost, Path: HistoryPath, Token: sysopToken, Body: struct{}{}})
	if err != nil {
		return models.HistoryResponse{}, err
	}
	var out models.HistoryResponse
	if err := resp.DecodeJSON(&out); err != nil {
		return models.HistoryResponse{}, err
	}
	return out, nil
}

// DecodeNotification reads a push notification from the request the
// orchestrator sent to your notify URL. Use it inside your own HTTP handler;
// the SDK does not run a server for you.
func DecodeNotification(r *http.Request) (models.PushNotification, error) {
	b, err := io.ReadAll(io.LimitReader(r.Body, maxNotificationBytes))
	if err != nil {
		return models.PushNotification{}, fmt.Errorf("orchestration: read notification: %w", err)
	}
	var n models.PushNotification
	if err := json.Unmarshal(b, &n); err != nil {
		return models.PushNotification{}, fmt.Errorf("orchestration: decode notification: %w", err)
	}
	if n.SubscriptionID == "" {
		return models.PushNotification{}, errors.New("orchestration: notification has no subscriptionId")
	}
	return n, nil
}
