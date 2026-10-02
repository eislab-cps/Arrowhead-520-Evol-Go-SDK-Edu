package models

import "strings"

// Policy types and target types of ConsumerAuthorization (C10).
const (
	PolicyTypeAll       = "ALL"
	PolicyTypeWhitelist = "WHITELIST"
	PolicyTypeBlacklist = "BLACKLIST"

	TargetTypeServiceDef = "SERVICE_DEF"
	TargetTypeEventType  = "EVENT_TYPE"
)

// PolicyDef says which consumers a policy admits.
type PolicyDef struct {
	PolicyType string   `json:"policyType"`
	PolicyList []string `json:"policyList,omitempty"`
}

// GrantRequest is the body of C10,
// POST /consumerauthorization/authorization/grant.
type GrantRequest struct {
	Provider       string               `json:"provider"`
	TargetType     string               `json:"targetType"`
	Target         string               `json:"target"`
	Description    string               `json:"description,omitempty"`
	DefaultPolicy  PolicyDef            `json:"defaultPolicy"`
	ScopedPolicies map[string]PolicyDef `json:"scopedPolicies,omitempty"`
	CreatedBy      string               `json:"createdBy,omitempty"`
}

// AuthPolicy is a stored ConsumerAuthorization policy (C10 response, C12 entries).
type AuthPolicy struct {
	InstanceID         string               `json:"instanceId"`
	AuthorizationLevel string               `json:"authorizationLevel"`
	Cloud              string               `json:"cloud"`
	Provider           string               `json:"provider"`
	TargetType         string               `json:"targetType"`
	Target             string               `json:"target"`
	Description        string               `json:"description,omitempty"`
	DefaultPolicy      PolicyDef            `json:"defaultPolicy"`
	ScopedPolicies     map[string]PolicyDef `json:"scopedPolicies"`
	CreatedBy          string               `json:"createdBy,omitempty"`
	CreatedAt          string               `json:"createdAt,omitempty"`
}

// PolicyLookup is the body of C12,
// POST /consumerauthorization/authorization/lookup. At least one of
// InstanceIDs, CloudIdentifiers or TargetNames must be non-empty.
type PolicyLookup struct {
	InstanceIDs      []string `json:"instanceIds,omitempty"`
	CloudIdentifiers []string `json:"cloudIdentifiers,omitempty"`
	TargetNames      []string `json:"targetNames,omitempty"`
	TargetType       string   `json:"targetType,omitempty"`
}

// PolicyLookupResult is the 200 body of C12.
type PolicyLookupResult struct {
	Policies   []AuthPolicy `json:"policies"`
	Count      int          `json:"count"`
	TotalCount int          `json:"totalCount"`
}

// PolicyInstanceID returns the instanceId ConsumerAuthorization gives a local
// provider-level policy: "PR|LOCAL|<provider>|<targetType>|<target>". It is the
// documented format (CONTRACT.md C10), computed without contacting the stack.
func PolicyInstanceID(provider, targetType, target string) string {
	return strings.Join([]string{"PR", "LOCAL", provider, targetType, target}, "|")
}
