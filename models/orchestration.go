package models

// OrchestrationSystem identifies a system in an orchestration request or
// result. In a request only SystemName is needed; the orchestrator does not
// authenticate it (CONTRACT.md, "Transports").
type OrchestrationSystem struct {
	SystemName string `json:"systemName"`
	Address    string `json:"address,omitempty"`
	Port       int    `json:"port,omitempty"`
}

// RequestedService names the service a consumer asks for. Interfaces and
// Metadata are optional filters.
type RequestedService struct {
	ServiceDefinition string            `json:"serviceDefinition"`
	Interfaces        []string          `json:"interfaces,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

// OrchestrationRequest is the body of C5,
// POST /serviceorchestration/orchestration/pull.
type OrchestrationRequest struct {
	RequesterSystem  OrchestrationSystem `json:"requesterSystem"`
	RequestedService RequestedService    `json:"requestedService"`
}

// ServiceInfo is the service part of one orchestration result.
type ServiceInfo struct {
	ServiceDefinition string            `json:"serviceDefinition"`
	ServiceURI        string            `json:"serviceUri"`
	Interfaces        []string          `json:"interfaces"`
	Version           int               `json:"version"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

// OrchestrationResult is one provider the requester may use. The shape is the
// stack's nested one, not the AH5 OrchestrationResult.
type OrchestrationResult struct {
	Provider        OrchestrationSystem `json:"provider"`
	Service         ServiceInfo         `json:"service"`
	CloudIdentifier string              `json:"cloudIdentifier,omitempty"`
	ExclusiveUntil  string              `json:"exclusiveUntil,omitempty"`
}

// OrchestrationResponse is the 200 body of C5. An empty Response is a normal
// answer: no provider is registered, or none is authorized for the requester.
type OrchestrationResponse struct {
	Response []OrchestrationResult `json:"response"`
}

// NotifyInterface says where the orchestrator delivers push notifications.
// The SDK always sets NotifyURI, a plain http:// URL reachable from the
// orchestrator.
type NotifyInterface struct {
	NotifyURI string `json:"notifyUri"`
}

// SubscriptionRequest is the body of C6,
// POST /serviceorchestration/orchestration/subscribe.
type SubscriptionRequest struct {
	OwnerSystemName      string               `json:"ownerSystemName"`
	TargetSystemName     string               `json:"targetSystemName"`
	OrchestrationRequest OrchestrationRequest `json:"orchestrationRequest"`
	NotifyInterface      *NotifyInterface     `json:"notifyInterface,omitempty"`
	ExpiredAt            string               `json:"expiredAt,omitempty"`
}

// Subscription is the stored subscription returned by C6.
type Subscription struct {
	ID                   string               `json:"id"`
	OwnerSystemName      string               `json:"ownerSystemName"`
	TargetSystemName     string               `json:"targetSystemName"`
	OrchestrationRequest OrchestrationRequest `json:"orchestrationRequest"`
	NotifyInterface      *NotifyInterface     `json:"notifyInterface,omitempty"`
	ExpiredAt            string               `json:"expiredAt,omitempty"`
	CreatedAt            string               `json:"createdAt"`
}

// PushTriggerRequest is the body of C8,
// POST /serviceorchestration/orchestration/mgmt/push/trigger.
type PushTriggerRequest struct {
	SubscriptionID string `json:"subscriptionId"`
}

// PushTriggerResponse is the 200 body of C8.
type PushTriggerResponse struct {
	Status string `json:"status"`
}

// PushNotification is what the orchestrator POSTs to a subscriber's notify
// URL after C8. It carries no provider list: the subscriber pulls (C5) next.
type PushNotification struct {
	SubscriptionID   string `json:"subscriptionId"`
	OwnerSystemName  string `json:"ownerSystemName"`
	TargetSystemName string `json:"targetSystemName"`
}

// History entry types and statuses (C9).
const (
	HistoryTypePull = "PULL"
	HistoryTypePush = "PUSH"

	HistoryStatusDone      = "DONE"
	HistoryStatusError     = "ERROR"
	HistoryStatusPending   = "PENDING"
	HistoryStatusDelivered = "DELIVERED"
	HistoryStatusFailed    = "FAILED"
)

// HistoryEntry is one orchestration history record (C9).
type HistoryEntry struct {
	ID                string `json:"id"`
	Status            string `json:"status"`
	Type              string `json:"type"`
	RequesterSystem   string `json:"requesterSystem,omitempty"`
	ServiceDefinition string `json:"serviceDefinition,omitempty"`
	Message           string `json:"message,omitempty"`
	CreatedAt         string `json:"createdAt"`
	FinishedAt        string `json:"finishedAt,omitempty"`
}

// HistoryResponse is the 200 body of C9.
type HistoryResponse struct {
	Entries []HistoryEntry `json:"entries"`
	Count   int            `json:"count"`
}
