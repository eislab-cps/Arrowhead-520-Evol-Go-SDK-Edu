package models

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

// keysOf marshals v and returns the top-level JSON keys, sorted. Tests compare
// them with the literal keys in CONTRACT.md, never with the SDK's own structs.
func keysOf(t *testing.T, v any) []string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func wantKeys(t *testing.T, name string, v any, want ...string) {
	t.Helper()
	sort.Strings(want)
	if got := keysOf(t, v); !reflect.DeepEqual(got, want) {
		t.Errorf("%s keys = %v, want %v", name, got, want)
	}
}

func TestRequestKeysMatchContract(t *testing.T) {
	wantKeys(t, "C1 LoginRequest", LoginRequest{SystemName: "A", Credentials: Credentials{Password: "p"}},
		"systemName", "credentials")
	wantKeys(t, "C1 credentials", Credentials{Password: "p"}, "password")

	wantKeys(t, "C2 ServiceRegistration", ServiceRegistration{
		SystemName: "TemperatureProvider", ServiceDefinitionName: "temperatureService", Version: "1.0.0",
		ExpiresAt: "2027-01-01T00:00:00Z", Metadata: map[string]string{"zone": "B"},
		Interfaces: []Interface{{TemplateName: "generic_http", Protocol: "http", Policy: PolicyNone}},
	}, "systemName", "serviceDefinitionName", "version", "expiresAt", "metadata", "interfaces")
	wantKeys(t, "C2 interface", Interface{TemplateName: "t", Protocol: "http", Policy: PolicyNone, Properties: map[string]string{"accessPort": "9000"}},
		"templateName", "protocol", "policy", "properties")

	wantKeys(t, "C3 ServiceLookup", ServiceLookup{
		InstanceIDs: []string{"i"}, ProviderNames: []string{"P"}, ServiceDefinitionNames: []string{"s"},
		Versions: []string{"1"}, InterfaceTemplateNames: []string{"t"},
	}, "instanceIds", "providerNames", "serviceDefinitionNames", "versions", "interfaceTemplateNames")
	wantKeys(t, "C3 ServiceLookup (only definitions)", ServiceLookup{ServiceDefinitionNames: []string{"s"}},
		"serviceDefinitionNames")

	pull := OrchestrationRequest{
		RequesterSystem:  OrchestrationSystem{SystemName: "ConsumerApp"},
		RequestedService: RequestedService{ServiceDefinition: "temperatureService"},
	}
	wantKeys(t, "C5 OrchestrationRequest", pull, "requesterSystem", "requestedService")
	wantKeys(t, "C5 requesterSystem", pull.RequesterSystem, "systemName")
	wantKeys(t, "C5 requestedService", pull.RequestedService, "serviceDefinition")

	wantKeys(t, "C6 SubscriptionRequest", SubscriptionRequest{
		OwnerSystemName: "ConsumerApp", TargetSystemName: "TemperatureProvider", OrchestrationRequest: pull,
		NotifyInterface: &NotifyInterface{NotifyURI: "http://h:1/notify"}, ExpiredAt: "2027-01-01T00:00:00Z",
	}, "ownerSystemName", "targetSystemName", "orchestrationRequest", "notifyInterface", "expiredAt")
	wantKeys(t, "C6 notifyInterface", NotifyInterface{NotifyURI: "http://h:1/notify"}, "notifyUri")

	wantKeys(t, "C8 PushTriggerRequest", PushTriggerRequest{SubscriptionID: "x"}, "subscriptionId")

	wantKeys(t, "C10 GrantRequest", GrantRequest{
		Provider: "TemperatureProvider", TargetType: TargetTypeServiceDef, Target: "temperatureService",
		DefaultPolicy: PolicyDef{PolicyType: PolicyTypeWhitelist, PolicyList: []string{"ConsumerApp"}},
	}, "provider", "targetType", "target", "defaultPolicy")
	wantKeys(t, "C10 defaultPolicy", PolicyDef{PolicyType: PolicyTypeWhitelist, PolicyList: []string{"C"}},
		"policyType", "policyList")

	wantKeys(t, "C12 PolicyLookup", PolicyLookup{
		InstanceIDs: []string{"i"}, CloudIdentifiers: []string{"LOCAL"}, TargetNames: []string{"t"}, TargetType: TargetTypeServiceDef,
	}, "instanceIds", "cloudIdentifiers", "targetNames", "targetType")

	wantKeys(t, "C14 CertificateRequest", CertificateRequest{SystemName: "TemperatureProvider"}, "systemName")
}

func TestPropertyValuesAreStrings(t *testing.T) {
	b, _ := json.Marshal(Interface{TemplateName: "t", Protocol: "http", Policy: PolicyNone,
		Properties: map[string]string{PropertyAccessPort: "9000"}})
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	port := m["properties"].(map[string]any)["accessPort"]
	if _, ok := port.(string); !ok {
		t.Fatalf("accessPort = %#v (%T), want a JSON string", port, port)
	}
}

// Response examples copied from CONTRACT.md decode into the SDK types.
func TestResponsesDecodeFromContractExamples(t *testing.T) {
	var login LoginResponse
	mustDecode(t, `{"token":"t","systemName":"S","expirationTime":"2006-01-02T15:04:05Z","sysop":true}`, &login)
	if login.Token != "t" || !login.Sysop || login.ExpirationTime != "2006-01-02T15:04:05Z" {
		t.Errorf("C1: %+v", login)
	}

	var inst ServiceInstance
	mustDecode(t, `{"instanceId":"i1","provider":{"name":"TemperatureProvider","createdAt":"a","updatedAt":"b"},
		"serviceDefinitionName":"temperatureService","version":"1.0.0","expiresAt":"e","metadata":{"zone":"B"},
		"interfaces":[{"templateName":"generic_http","protocol":"http","policy":"NONE","properties":{"accessPort":"9000"}}],
		"createdAt":"c","updatedAt":"u"}`, &inst)
	if inst.InstanceID != "i1" || inst.Provider == nil || inst.Provider.Name != "TemperatureProvider" ||
		inst.Interfaces[0].Properties["accessPort"] != "9000" {
		t.Errorf("C2: %+v", inst)
	}

	var lookup ServiceLookupResult
	mustDecode(t, `{"entries":[{"instanceId":"i1","serviceDefinitionName":"s","createdAt":"c","updatedAt":"u"}],"count":1,"totalCount":1}`, &lookup)
	if lookup.Count != 1 || lookup.TotalCount != 1 || lookup.Entries[0].InstanceID != "i1" {
		t.Errorf("C3: %+v", lookup)
	}

	var pull OrchestrationResponse
	mustDecode(t, `{"response":[{"provider":{"systemName":"TemperatureProvider","address":"10.0.0.5","port":9000},
		"service":{"serviceDefinition":"temperatureService","serviceUri":"/temperature","interfaces":["generic_http"],"version":1,"metadata":{"zone":"B"}},
		"cloudIdentifier":"LOCAL"}]}`, &pull)
	r := pull.Response[0]
	if r.Provider.Port != 9000 || r.Service.ServiceURI != "/temperature" || r.Service.Version != 1 || r.CloudIdentifier != "LOCAL" {
		t.Errorf("C5: %+v", r)
	}
	var empty OrchestrationResponse
	mustDecode(t, `{"response":[]}`, &empty)
	if empty.Response == nil || len(empty.Response) != 0 {
		t.Errorf("C5 empty: %+v", empty)
	}

	var sub Subscription
	mustDecode(t, `{"id":"3f1c","ownerSystemName":"ConsumerApp","targetSystemName":"TemperatureProvider",
		"orchestrationRequest":{"requesterSystem":{"systemName":"ConsumerApp"},"requestedService":{"serviceDefinition":"temperatureService"}},
		"notifyInterface":{"notifyUri":"http://h/notify"},"expiredAt":"x","createdAt":"c"}`, &sub)
	if sub.ID != "3f1c" || sub.NotifyInterface.NotifyURI != "http://h/notify" || sub.OrchestrationRequest.RequesterSystem.SystemName != "ConsumerApp" {
		t.Errorf("C6: %+v", sub)
	}

	var trig PushTriggerResponse
	mustDecode(t, `{"status":"triggered"}`, &trig)
	if trig.Status != "triggered" {
		t.Errorf("C8: %+v", trig)
	}
	var note PushNotification
	mustDecode(t, `{"subscriptionId":"s","ownerSystemName":"o","targetSystemName":"t"}`, &note)
	if note.SubscriptionID != "s" || note.OwnerSystemName != "o" || note.TargetSystemName != "t" {
		t.Errorf("C8 notification: %+v", note)
	}

	var hist HistoryResponse
	mustDecode(t, `{"entries":[{"id":"h","status":"DONE","type":"PULL","requesterSystem":"ConsumerApp","serviceDefinition":"s","createdAt":"c","finishedAt":"f"}],"count":1}`, &hist)
	if hist.Count != 1 || hist.Entries[0].Status != HistoryStatusDone || hist.Entries[0].Type != HistoryTypePull {
		t.Errorf("C9: %+v", hist)
	}

	var pol AuthPolicy
	mustDecode(t, `{"instanceId":"PR|LOCAL|TemperatureProvider|SERVICE_DEF|temperatureService","authorizationLevel":"PR","cloud":"LOCAL",
		"provider":"TemperatureProvider","targetType":"SERVICE_DEF","target":"temperatureService","description":"optional",
		"defaultPolicy":{"policyType":"WHITELIST","policyList":["ConsumerApp"]},"scopedPolicies":{},"createdBy":"x","createdAt":"2006-01-02T15:04:05Z"}`, &pol)
	if pol.InstanceID != PolicyInstanceID("TemperatureProvider", TargetTypeServiceDef, "temperatureService") ||
		pol.DefaultPolicy.PolicyList[0] != "ConsumerApp" {
		t.Errorf("C10: %+v", pol)
	}

	var pols PolicyLookupResult
	mustDecode(t, `{"policies":[],"count":0,"totalCount":5}`, &pols)
	if pols.TotalCount != 5 {
		t.Errorf("C12: %+v", pols)
	}

	var info CAInfo
	mustDecode(t, `{"commonName":"Arrowhead Local Cloud CA","certificate":"-----BEGIN CERTIFICATE-----\n"}`, &info)
	if info.CommonName != "Arrowhead Local Cloud CA" {
		t.Errorf("C13: %+v", info)
	}

	var cert IssuedCertificate
	mustDecode(t, `{"systemName":"TemperatureProvider","certificate":"c","privateKey":"k","profile":"on","issuedAt":"2026-06-25T00:00:00Z"}`, &cert)
	if cert.Profile != ProfileOnboarding || cert.PrivateKey != "k" {
		t.Errorf("C14: %+v", cert)
	}

	var env ErrorEnvelope
	mustDecode(t, `{"errorMessage":"m","errorCode":400,"exceptionType":"INVALID_PARAMETER","origin":""}`, &env)
	if env.ErrorCode != 400 || env.ExceptionType != ExceptionInvalidParameter || env.Origin != "" {
		t.Errorf("error envelope: %+v", env)
	}
}

func mustDecode(t *testing.T, s string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), v); err != nil {
		t.Fatalf("decode %T: %v", v, err)
	}
}

func TestPolicyInstanceID(t *testing.T) {
	got := PolicyInstanceID("TemperatureProvider", TargetTypeServiceDef, "temperatureService")
	if want := "PR|LOCAL|TemperatureProvider|SERVICE_DEF|temperatureService"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNameValidation(t *testing.T) {
	for _, ok := range []string{"TemperatureProvider", "A", "TProv1727700000"} {
		if err := ValidateSystemName(ok); err != nil {
			t.Errorf("system %q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "temp-provider", "tempProvider", "Temp_Provider", "T" + string(make([]byte, 63))} {
		if ValidateSystemName(bad) == nil {
			t.Errorf("system %q accepted", bad)
		}
	}
	for _, ok := range []string{"temperatureService", "t", "tSvc1727700000"} {
		if err := ValidateServiceDefinitionName(ok); err != nil {
			t.Errorf("service %q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "temperature-service", "Telemetry", "temp_service"} {
		if ValidateServiceDefinitionName(bad) == nil {
			t.Errorf("service %q accepted", bad)
		}
	}
}
