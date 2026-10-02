package models

// Interface security policies accepted by C2 (CONTRACT.md C2).
const (
	PolicyNone                  = "NONE"
	PolicyCertAuth              = "CERT_AUTH"
	PolicyTimeLimitedToken      = "TIME_LIMITED_TOKEN_AUTH"
	PolicyUsageLimitedToken     = "USAGE_LIMITED_TOKEN_AUTH"
	PolicyBase64SelfContained   = "BASE64_SELF_CONTAINED_TOKEN_AUTH"
	PolicyRSASHA256JSONWebToken = "RSA_SHA256_JSON_WEB_TOKEN_AUTH"
	PolicyRSASHA512JSONWebToken = "RSA_SHA512_JSON_WEB_TOKEN_AUTH"
)

// Interface property keys the orchestrator reads when it builds a pull result
// (CONTRACT.md C2). Property values are always strings.
const (
	PropertyAccessAddresses = "accessAddresses" // → provider.address (first value if comma-separated)
	PropertyAccessPort      = "accessPort"      // → provider.port, e.g. "9000"
	PropertyBasePath        = "basePath"        // → service.serviceUri
)

// Address is an AH5 address object. It is never a bare string.
type Address struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

// Device is an AH5 device record.
type Device struct {
	Name      string            `json:"name"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Addresses []Address         `json:"addresses,omitempty"`
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
}

// System is an AH5 system record, as returned inside a service instance.
// Metadata, Version, Addresses and Device are omitted by the stack when empty.
type System struct {
	Name      string            `json:"name"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Version   string            `json:"version,omitempty"`
	Addresses []Address         `json:"addresses,omitempty"`
	Device    *Device           `json:"device,omitempty"`
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
}

// Interface is one interface of a service instance. Properties values are
// strings: "9000", not 9000.
type Interface struct {
	TemplateName string            `json:"templateName"`
	Protocol     string            `json:"protocol"`
	Policy       string            `json:"policy"`
	Properties   map[string]string `json:"properties,omitempty"`
}

// ServiceRegistration is the body of C2,
// POST /serviceregistry/service-discovery/register.
type ServiceRegistration struct {
	SystemName            string            `json:"systemName"`
	ServiceDefinitionName string            `json:"serviceDefinitionName"`
	Version               string            `json:"version,omitempty"`
	ExpiresAt             string            `json:"expiresAt,omitempty"`
	Metadata              map[string]string `json:"metadata,omitempty"`
	Interfaces            []Interface       `json:"interfaces"`
}

// ServiceInstance is a registered service instance (C2 response, C3 entries).
type ServiceInstance struct {
	InstanceID            string            `json:"instanceId"`
	Provider              *System           `json:"provider,omitempty"`
	ServiceDefinitionName string            `json:"serviceDefinitionName"`
	Version               string            `json:"version,omitempty"`
	ExpiresAt             string            `json:"expiresAt,omitempty"`
	Metadata              map[string]string `json:"metadata,omitempty"`
	Interfaces            []Interface       `json:"interfaces,omitempty"`
	CreatedAt             string            `json:"createdAt"`
	UpdatedAt             string            `json:"updatedAt"`
}

// ServiceLookup is the body of C3, POST /serviceregistry/service-discovery/lookup.
// At least one of InstanceIDs, ProviderNames or ServiceDefinitionNames must be
// non-empty, or the ServiceRegistry answers 400.
type ServiceLookup struct {
	InstanceIDs            []string `json:"instanceIds,omitempty"`
	ProviderNames          []string `json:"providerNames,omitempty"`
	ServiceDefinitionNames []string `json:"serviceDefinitionNames,omitempty"`
	Versions               []string `json:"versions,omitempty"`
	InterfaceTemplateNames []string `json:"interfaceTemplateNames,omitempty"`
}

// ServiceLookupResult is the 200 body of C3.
type ServiceLookupResult struct {
	Entries    []ServiceInstance `json:"entries"`
	Count      int               `json:"count"`
	TotalCount int               `json:"totalCount"`
}
