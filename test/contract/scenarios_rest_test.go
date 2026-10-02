//go:build contract

package contract

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/auth"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/events"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/models"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/orchestration"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/registry"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/transport"
)

// ---- shared helpers -------------------------------------------------------

func (h *harness) prov() string  { return "TProv" + h.sfx }
func (h *harness) cons() string  { return "TCons" + h.sfx }
func (h *harness) other() string { return "TOther" + h.sfx }

// svc returns a unique camelCase service definition name for a scenario.
func (h *harness) svc(label string) string { return "t" + label + h.sfx }

func (h *harness) sr(t *testing.T) *registry.Client {
	return registry.New(h.mtls(t, "SR_URL", "serviceregistry"))
}
func (h *harness) az(t *testing.T) *auth.Client {
	return auth.New(h.mtls(t, "CAUTH_URL", "consumerauth"))
}
func (h *harness) orch(t *testing.T) *orchestration.Client {
	return orchestration.New(plain(t, "ORCH_URL"))
}

func httpReg(provider, service string) models.ServiceRegistration {
	return models.ServiceRegistration{
		SystemName: provider, ServiceDefinitionName: service, Version: "1.0.0",
		Metadata:   map[string]string{"zone": "B"},
		Interfaces: []models.Interface{registry.HTTPInterface("generic_http", "10.0.0.5", 9000, "/temperature")},
	}
}

// register registers provider/service with the provider's own token and
// revokes the instance at the end of the test.
func (h *harness) register(t *testing.T, provider, service string) models.ServiceInstance {
	t.Helper()
	inst, _, err := h.sr(t).Register(ctx(t), h.tokens[provider], httpReg(provider, service))
	if err != nil {
		t.Fatalf("register %s/%s: %v", provider, service, err)
	}
	t.Cleanup(func() { _, _ = h.sr(t).Revoke(ctx(t), inst.InstanceID) })
	return inst
}

// grant lets consumers use provider/service and revokes the policy at the end.
func (h *harness) grant(t *testing.T, provider, service string, consumers ...string) models.AuthPolicy {
	t.Helper()
	p, err := h.az(t).Grant(ctx(t), auth.AllowConsumers(provider, service, consumers...))
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	t.Cleanup(func() { _ = h.az(t).Revoke(ctx(t), p.InstanceID) })
	return p
}

func (h *harness) pull(t *testing.T, requester, service string) models.OrchestrationResponse {
	t.Helper()
	res, err := h.orch(t).Pull(ctx(t), orchestration.PullRequest(requester, service))
	if err != nil {
		t.Fatalf("pull %s as %s: %v", service, requester, err)
	}
	return res
}

// rawPull returns the raw pull response body.
func rawPull(t *testing.T, requester, service string) []byte {
	t.Helper()
	resp, err := plain(t, "ORCH_URL").Do(ctx(t), transport.Request{Method: http.MethodPost, Path: orchestration.PullPath,
		Body: orchestration.PullRequest(requester, service)})
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("raw pull: %v", err)
	}
	return resp.Body
}

func (h *harness) certArgs() []string {
	return []string{"-cert", filepath.Join(h.certDir, "TestHarnessExample.crt"),
		"-key", filepath.Join(h.certDir, "TestHarnessExample.key"), "-ca", filepath.Join(h.certDir, "ca.crt")}
}

// listener is a notify endpoint on the host, reachable from the orchestrator
// container as host.docker.internal.
type listener struct {
	url    string
	mu     sync.Mutex
	bodies [][]byte
	got    chan struct{}
}

func newListener(t *testing.T, status int) *listener {
	t.Helper()
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	l := &listener{got: make(chan struct{}, 16)}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		l.mu.Lock()
		l.bodies = append(l.bodies, b)
		l.mu.Unlock()
		w.WriteHeader(status)
		l.got <- struct{}{}
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	l.url = fmt.Sprintf("http://host.docker.internal:%d/notify", ln.Addr().(*net.TCPAddr).Port)
	return l
}

func (l *listener) wait(d time.Duration) ([]byte, bool) {
	select {
	case <-l.got:
		l.mu.Lock()
		defer l.mu.Unlock()
		return l.bodies[len(l.bodies)-1], true
	case <-time.After(d):
		return nil, false
	}
}

func (h *harness) subscribe(t *testing.T, owner, target, service, notifyURL string) models.Subscription {
	t.Helper()
	sub, _, err := h.orch(t).Subscribe(ctx(t), models.SubscriptionRequest{
		OwnerSystemName: owner, TargetSystemName: target,
		OrchestrationRequest: orchestration.PullRequest(owner, service),
		NotifyInterface:      &models.NotifyInterface{NotifyURI: notifyURL},
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _, _ = h.orch(t).Unsubscribe(ctx(t), sub.ID) })
	return sub
}

// ---- T2–T4 registry -------------------------------------------------------

func t2(t *testing.T, h *harness) {
	svc := h.svc("Reg")
	srT := h.mtls(t, "SR_URL", "serviceregistry")
	resp, err := srT.Do(ctx(t), transport.Request{Method: http.MethodPost, Path: registry.RegisterPath,
		Token: h.tokens[h.prov()], Body: httpReg(h.prov(), svc)})
	if err != nil || resp.StatusCode != 201 {
		t.Fatalf("first register: %v", err)
	}
	m := wire(t, resp.Body)
	hasKeys(t, "C2", m, "instanceId", "provider", "serviceDefinitionName", "version", "interfaces", "createdAt", "updatedAt")
	if m["provider"].(map[string]any)["name"] != h.prov() || m["version"] != "1.0.0" {
		t.Errorf("C2 body %v", m)
	}
	port := m["interfaces"].([]any)[0].(map[string]any)["properties"].(map[string]any)["accessPort"]
	if port != "9000" {
		t.Errorf("accessPort = %#v, want the string \"9000\"", port)
	}
	id := m["instanceId"].(string)
	t.Cleanup(func() { _, _ = h.sr(t).Revoke(ctx(t), id) })

	reg := httpReg(h.prov(), svc)
	reg.Metadata = map[string]string{"zone": "C"}
	inst, created, err := h.sr(t).Register(ctx(t), h.tokens[h.prov()], reg)
	if err != nil || created || inst.InstanceID != id || inst.Metadata["zone"] != "C" {
		t.Errorf("second register: created=%v id=%s (want %s) meta=%v err=%v", created, inst.InstanceID, id, inst.Metadata, err)
	}

	_, _, err = h.sr(t).Register(ctx(t), "", httpReg(h.prov(), svc))
	envelope(t, "no token", apiErr(t, "register without token", err, 401), models.ExceptionAuth)
	_, _, err = h.sr(t).Register(ctx(t), "bogus", httpReg(h.prov(), svc))
	envelope(t, "bogus token", apiErr(t, "register with bogus token", err, 401), models.ExceptionAuth)
	_, _, err = h.sr(t).Register(ctx(t), h.tokens[h.prov()], httpReg(h.prov(), "TSvc"))
	envelope(t, "TSvc", apiErr(t, "PascalCase service definition", err, 400), models.ExceptionInvalidParameter)
	bad := httpReg(h.prov(), svc)
	bad.Interfaces[0].Policy = "BOGUS"
	_, _, err = h.sr(t).Register(ctx(t), h.tokens[h.prov()], bad)
	apiErr(t, "policy BOGUS", err, 400)
	_, err = srT.Do(ctx(t), transport.Request{Method: http.MethodPost, Path: registry.RegisterPath, Token: h.tokens[h.prov()],
		Body: map[string]any{"systemName": h.prov(), "serviceDefinitionName": svc, "version": "1.0.0",
			"interfaces": []any{map[string]any{"templateName": "generic_http", "protocol": "http", "policy": "NONE",
				"properties": map[string]any{"accessPort": 9000}}}}})
	apiErr(t, "numeric property value", err, 400)

	h.runExample(t, "registry", append(h.certArgs(), "-sr-url", env(t, "SR_URL"), "-auth-url", env(t, "AUTH_URL"),
		"-system", h.prov(), "-password", h.passwords[h.prov()], "-service", h.svc("Example"))...)
}

func t3(t *testing.T, h *harness) {
	svc := h.svc("Look")
	inst := h.register(t, h.prov(), svc)
	for name, q := range map[string]models.ServiceLookup{
		"serviceDefinitionNames": {ServiceDefinitionNames: []string{svc}},
		"providerNames":          {ProviderNames: []string{h.prov()}, ServiceDefinitionNames: []string{svc}},
		"instanceIds":            {InstanceIDs: []string{inst.InstanceID}},
	} {
		resp, err := h.mtls(t, "SR_URL", "serviceregistry").Do(ctx(t), transport.Request{Method: http.MethodPost, Path: registry.LookupPath, Body: q})
		if err != nil {
			t.Fatalf("lookup by %s: %v", name, err)
		}
		m := wire(t, resp.Body)
		exactKeys(t, "C3", m, "entries", "count", "totalCount")
		var res models.ServiceLookupResult
		_ = resp.DecodeJSON(&res)
		if res.Count != 1 || len(res.Entries) != 1 || res.Entries[0].InstanceID != inst.InstanceID {
			t.Errorf("lookup by %s: %+v", name, res)
		}
	}
	_, err := h.sr(t).Lookup(ctx(t), models.ServiceLookup{Versions: []string{"1.0.0"}})
	apiErr(t, "lookup without a primary filter", err, 400)
	res, err := h.sr(t).Lookup(ctx(t), models.ServiceLookup{ServiceDefinitionNames: []string{h.svc("Nothing")}})
	if err != nil || len(res.Entries) != 0 {
		t.Errorf("unknown service: %+v %v", res, err)
	}
}

func t4(t *testing.T, h *harness) {
	svc := h.svc("Rev")
	inst, _, err := h.sr(t).Register(ctx(t), h.tokens[h.prov()], httpReg(h.prov(), svc))
	if err != nil {
		t.Fatal(err)
	}
	removed, err := h.sr(t).Revoke(ctx(t), inst.InstanceID) // no token: C4 is unguarded
	if err != nil || !removed {
		t.Fatalf("revoke: removed=%v err=%v", removed, err)
	}
	if removed, err = h.sr(t).Revoke(ctx(t), inst.InstanceID); err != nil || removed {
		t.Errorf("second revoke: want 204 (removed=false), got removed=%v err=%v", removed, err)
	}
	res, _ := h.sr(t).Lookup(ctx(t), models.ServiceLookup{InstanceIDs: []string{inst.InstanceID}})
	if len(res.Entries) != 0 {
		t.Errorf("instance still found: %+v", res)
	}
}

// ---- T5–T9 orchestration --------------------------------------------------

func t5(t *testing.T, h *harness) {
	svc := h.svc("Pull")
	h.register(t, h.prov(), svc)
	h.grant(t, h.prov(), svc, h.cons())

	m := wire(t, rawPull(t, h.cons(), svc))
	exactKeys(t, "C5", m, "response")
	r0 := m["response"].([]any)[0].(map[string]any)
	hasKeys(t, "C5 result", r0, "provider", "service", "cloudIdentifier")
	if _, flat := r0["serviceUri"]; flat {
		t.Error("C5: serviceUri at top level; want the nested shape")
	}
	if p := r0["provider"].(map[string]any)["port"]; p != float64(9000) {
		t.Errorf("provider.port = %#v, want the number 9000", p)
	}

	res := h.pull(t, h.cons(), svc)
	if len(res.Response) != 1 {
		t.Fatalf("pull: %+v", res)
	}
	r := res.Response[0]
	if r.Provider.SystemName != h.prov() || r.Provider.Address != "10.0.0.5" || r.Provider.Port != 9000 ||
		r.Service.ServiceURI != "/temperature" || r.Service.Version != 1 || r.CloudIdentifier != "LOCAL" {
		t.Errorf("pull result %+v", r)
	}
	req := orchestration.PullRequest(h.cons(), svc)
	req.RequestedService.Interfaces = []string{"generic_http"}
	if res, err := h.orch(t).Pull(ctx(t), req); err != nil || len(res.Response) != 1 {
		t.Errorf("pull with interface filter: %+v %v", res, err)
	}
	req.RequestedService.Metadata = map[string]string{"zone": "Z"}
	if res, err := h.orch(t).Pull(ctx(t), req); err != nil || len(res.Response) != 0 {
		t.Errorf("pull with unmatched metadata: %+v %v", res, err)
	}

	if got := string(rawPull(t, h.cons(), h.svc("Unregistered"))); strings.TrimSpace(got) != `{"response":[]}` {
		t.Errorf("unregistered service: %s, want {\"response\":[]}", got)
	}
	_, err := h.orch(t).Pull(ctx(t), orchestration.PullRequest("", svc))
	apiErr(t, "pull without requester", err, 400)

	h.runExample(t, "orchestration", append(h.certArgs(), "-url", env(t, "ORCH_URL"), "-auth-url", env(t, "AUTH_URL"),
		"-consumer", h.cons(), "-service", svc, "-sysop-password", env(t, "SYSOP_PASSWORD"))...)
}

func t6(t *testing.T, h *harness) {
	req := models.SubscriptionRequest{
		OwnerSystemName: h.cons(), TargetSystemName: h.prov(),
		OrchestrationRequest: orchestration.PullRequest(h.cons(), h.svc("Sub")),
		NotifyInterface:      &models.NotifyInterface{NotifyURI: "http://host.docker.internal:1/notify"},
	}
	resp, err := plain(t, "ORCH_URL").Do(ctx(t), transport.Request{Method: http.MethodPost, Path: orchestration.SubscribePath, Body: req})
	if err != nil || resp.StatusCode != 201 {
		t.Fatalf("first subscribe: %v", err)
	}
	m := wire(t, resp.Body)
	hasKeys(t, "C6", m, "id", "ownerSystemName", "targetSystemName", "orchestrationRequest", "notifyInterface", "createdAt")
	id, _ := m["id"].(string)
	t.Cleanup(func() { _, _ = h.orch(t).Unsubscribe(ctx(t), id) })

	req.ExpiredAt = "2030-01-01T00:00:00Z"
	sub, created, err := h.orch(t).Subscribe(ctx(t), req)
	if err != nil || created || sub.ID != id || sub.ExpiredAt != req.ExpiredAt {
		t.Errorf("second subscribe: created=%v id=%s (want %s) expiredAt=%q err=%v", created, sub.ID, id, sub.ExpiredAt, err)
	}
	_, err = plain(t, "ORCH_URL").Do(ctx(t), transport.Request{Method: http.MethodPost, Path: orchestration.SubscribePath,
		Body: map[string]any{"ownerSystemName": h.cons(), "targetSystemName": "X", "notifyInterface": "http://x/notify"}})
	apiErr(t, "notifyInterface as a string", err, 400)
}

func t7(t *testing.T, h *harness) {
	sub := h.subscribe(t, h.cons(), h.other(), h.svc("Unsub"), "http://host.docker.internal:1/notify")
	if removed, err := h.orch(t).Unsubscribe(ctx(t), sub.ID); err != nil || !removed {
		t.Errorf("unsubscribe: removed=%v err=%v", removed, err)
	}
	if removed, err := h.orch(t).Unsubscribe(ctx(t), sub.ID); err != nil || removed {
		t.Errorf("second unsubscribe: want 204, got removed=%v err=%v", removed, err)
	}
}

func t8(t *testing.T, h *harness) {
	svc := h.svc("Push")
	h.register(t, h.prov(), svc)
	h.grant(t, h.prov(), svc, h.cons())
	l := newListener(t, http.StatusOK)
	sub := h.subscribe(t, h.cons(), h.prov(), svc, l.url)

	resp, err := plain(t, "ORCH_URL").Do(ctx(t), transport.Request{Method: http.MethodPost, Path: orchestration.TriggerPath,
		Token: h.sysopToken, Body: models.PushTriggerRequest{SubscriptionID: sub.ID}})
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if m := wire(t, resp.Body); !reflect.DeepEqual(m, map[string]any{"status": "triggered"}) {
		t.Errorf("trigger body %v", m)
	}
	body, ok := l.wait(5 * time.Second)
	if !ok {
		t.Fatal("no notification within 5 s")
	}
	n := wire(t, body)
	exactKeys(t, "C8 notification (no provider list)", n, "subscriptionId", "ownerSystemName", "targetSystemName")
	if n["subscriptionId"] != sub.ID {
		t.Errorf("notification %v", n)
	}
	if res := h.pull(t, h.cons(), svc); len(res.Response) != 1 || res.Response[0].Provider.SystemName != h.prov() {
		t.Errorf("pull after notification: %+v", res)
	}

	_, err = h.orch(t).TriggerPush(ctx(t), h.sysopToken, "no-such-subscription")
	apiErr(t, "unknown subscription", err, 404)

	fail := newListener(t, http.StatusInternalServerError)
	fsub := h.subscribe(t, h.cons(), h.other(), svc, fail.url)
	if _, err := h.orch(t).TriggerPush(ctx(t), h.sysopToken, fsub.ID); err != nil {
		t.Fatalf("trigger to failing listener: %v", err)
	}
	if _, ok := fail.wait(5 * time.Second); !ok {
		t.Error("failing listener got no notification")
	}
}

func t9(t *testing.T, h *harness) {
	resp, err := plain(t, "ORCH_URL").Do(ctx(t), transport.Request{Method: http.MethodPost, Path: orchestration.HistoryPath,
		Token: h.sysopToken, Body: struct{}{}})
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	exactKeys(t, "C9", wire(t, resp.Body), "entries", "count")

	var hist models.HistoryResponse
	deadline := time.Now().Add(5 * time.Second)
	for {
		hist, err = h.orch(t).History(ctx(t), h.sysopToken)
		if err != nil {
			t.Fatal(err)
		}
		if has(hist, models.HistoryTypePush, models.HistoryStatusDelivered, "") &&
			has(hist, models.HistoryTypePush, models.HistoryStatusFailed, "") || time.Now().After(deadline) {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if hist.Count != len(hist.Entries) {
		t.Errorf("count %d, entries %d", hist.Count, len(hist.Entries))
	}
	if !has(hist, models.HistoryTypePull, models.HistoryStatusDone, h.cons()) {
		t.Error("no PULL/DONE entry for the consumer")
	}
	if !has(hist, models.HistoryTypePush, models.HistoryStatusDelivered, h.cons()) {
		t.Error("no PUSH/DELIVERED entry (T8)")
	}
	if !has(hist, models.HistoryTypePush, models.HistoryStatusFailed, h.cons()) {
		t.Error("no PUSH/FAILED entry (T8 failing listener)")
	}
}

func has(hist models.HistoryResponse, typ, status, requester string) bool {
	for _, e := range hist.Entries {
		if e.Type == typ && e.Status == status && (requester == "" || e.RequesterSystem == requester) {
			return true
		}
	}
	return false
}

// ---- T10–T12 ConsumerAuthorization ---------------------------------------

func t10(t *testing.T, h *harness) {
	svc := h.svc("Grant")
	resp, err := h.mtls(t, "CAUTH_URL", "consumerauth").Do(ctx(t), transport.Request{Method: http.MethodPost, Path: auth.GrantPath,
		Body: auth.AllowConsumers(h.prov(), svc, h.cons())}) // no token: not under MGMT_AUTH_URL
	if err != nil || resp.StatusCode != 201 {
		t.Fatalf("grant: %v", err)
	}
	m := wire(t, resp.Body)
	hasKeys(t, "C10", m, "instanceId", "authorizationLevel", "cloud", "provider", "targetType", "target", "defaultPolicy", "scopedPolicies")
	want := models.PolicyInstanceID(h.prov(), models.TargetTypeServiceDef, svc)
	if m["instanceId"] != want || m["cloud"] != "LOCAL" || m["defaultPolicy"].(map[string]any)["policyType"] != "WHITELIST" {
		t.Errorf("C10 body %v", m)
	}
	t.Cleanup(func() { _ = h.az(t).Revoke(ctx(t), want) })
	_, err = h.az(t).Grant(ctx(t), auth.AllowConsumers(h.prov(), svc, h.cons()))
	apiErr(t, "duplicate grant", err, 409)

	h.runExample(t, "auth", append(h.certArgs(), "-url", env(t, "CAUTH_URL"),
		"-provider", h.prov(), "-service", h.svc("AuthExample"), "-consumer", h.cons())...)
}

func t11(t *testing.T, h *harness) {
	p := h.grant(t, h.prov(), h.svc("Revoke"), h.cons())
	if err := h.az(t).Revoke(ctx(t), p.InstanceID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	apiErr(t, "second revoke", h.az(t).Revoke(ctx(t), p.InstanceID), 404)
}

func t12(t *testing.T, h *harness) {
	svc := h.svc("Policies")
	p := h.grant(t, h.prov(), svc, h.cons())
	for name, q := range map[string]models.PolicyLookup{
		"instanceIds": {InstanceIDs: []string{p.InstanceID}},
		"targetNames": {TargetNames: []string{svc}},
	} {
		resp, err := h.mtls(t, "CAUTH_URL", "consumerauth").Do(ctx(t), transport.Request{Method: http.MethodPost, Path: auth.LookupPath, Body: q})
		if err != nil {
			t.Fatalf("lookup by %s: %v", name, err)
		}
		exactKeys(t, "C12", wire(t, resp.Body), "policies", "count", "totalCount")
		var res models.PolicyLookupResult
		_ = resp.DecodeJSON(&res)
		if len(res.Policies) != 1 || res.Policies[0].InstanceID != p.InstanceID {
			t.Errorf("lookup by %s: %+v", name, res)
		}
	}
	_, err := h.az(t).Lookup(ctx(t), models.PolicyLookup{TargetType: models.TargetTypeServiceDef})
	apiErr(t, "lookup without filter", err, 400)
}

// ---- T17 MQTT --------------------------------------------------------------

func t17(t *testing.T, h *harness) {
	pub, svc, topic := "TPub"+h.sfx, h.svc("Evt"), "sensors/"+h.sfx
	inst, _, err := h.sr(t).Register(ctx(t), h.tokens[pub], models.ServiceRegistration{
		SystemName: pub, ServiceDefinitionName: svc, Version: "1.0.0",
		Interfaces: []models.Interface{events.Interface("localhost", 1883, topic)},
	})
	if err != nil {
		t.Fatalf("register MQTT publisher: %v", err)
	}
	t.Cleanup(func() { _, _ = h.sr(t).Revoke(ctx(t), inst.InstanceID) })
	res, err := h.sr(t).Lookup(ctx(t), models.ServiceLookup{ServiceDefinitionNames: []string{svc}})
	if err != nil || len(res.Entries) != 1 {
		t.Fatalf("lookup: %+v %v", res, err)
	}
	i0 := res.Entries[0].Interfaces[0]
	if i0.TemplateName != models.MQTTInterfaceName || i0.Protocol != "mqtt" || i0.Properties["basePath"] != topic {
		t.Errorf("interface %+v", i0)
	}

	c, err := events.Connect(ctx(t), env(t, "MQTT_URL"), "contract-"+h.sfx)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	got := make(chan events.Message, 1)
	if err := c.Subscribe(ctx(t), topic, func(m events.Message) { got <- m }); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"sensor": "t1", "value": 21.5}
	want, _ := json.Marshal(payload)
	if err := c.Publish(ctx(t), topic, payload); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if string(m.Payload) != string(want) || m.Topic != topic {
			t.Errorf("received %s on %s, want %s on %s", m.Payload, m.Topic, want, topic)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message within 2 s")
	}
	h.runExample(t, "events", "-broker", env(t, "MQTT_URL"), "-topic", "example/"+h.sfx)
}

// ---- T18 consumerauth access denial (R4) ------------------------------------

func t18(t *testing.T, h *harness) {
	svc := h.svc("Deny")
	h.register(t, h.prov(), svc)

	if got := strings.TrimSpace(string(rawPull(t, h.cons(), svc))); got != `{"response":[]}` {
		t.Fatalf("step 2, no policy: %s, want {\"response\":[]}", got)
	}
	res, err := h.sr(t).Lookup(ctx(t), models.ServiceLookup{ServiceDefinitionNames: []string{svc}})
	if err != nil || len(res.Entries) != 1 {
		t.Fatalf("step 2: the provider must be registered (lookup %+v %v)", res, err)
	}

	p, err := h.az(t).Grant(ctx(t), auth.AllowConsumers(h.prov(), svc, h.cons()))
	if err != nil {
		t.Fatal(err)
	}
	if r := h.pull(t, h.cons(), svc); len(r.Response) != 1 || r.Response[0].Provider.SystemName != h.prov() {
		t.Errorf("step 4, granted consumer: %+v", r)
	}
	if r := h.pull(t, h.other(), svc); len(r.Response) != 0 {
		t.Errorf("step 4, other consumer: %+v", r)
	}
	if err := h.az(t).Revoke(ctx(t), p.InstanceID); err != nil {
		t.Fatal(err)
	}
	if r := h.pull(t, h.cons(), svc); len(r.Response) != 0 {
		t.Errorf("step 6, after revoke: %+v", r)
	}

	resp, err := plain(t, "ORCH_URL").Do(ctx(t), transport.Request{Method: http.MethodGet, Path: "/status"})
	if err != nil {
		t.Fatal(err)
	}
	if m := wire(t, resp.Body); m["authBackend"] != "consumerauth" || m["enableAuth"] != true {
		t.Errorf("/status %v", m)
	}
}

// ---- T19 foreign token -------------------------------------------------------

func t19(t *testing.T, h *harness) {
	svc := h.svc("Foreign")
	_, _, err := h.sr(t).Register(ctx(t), h.tokens[h.cons()], httpReg(h.prov(), svc))
	e := apiErr(t, "register with another system's token", err, 403)
	envelope(t, "foreign token", e, models.ExceptionForbidden)
	if e.Message != "registration identity check failed" {
		t.Errorf("errorMessage %q", e.Message)
	}
	res, _ := h.sr(t).Lookup(ctx(t), models.ServiceLookup{ProviderNames: []string{h.prov()}, ServiceDefinitionNames: []string{svc}})
	if len(res.Entries) != 0 {
		t.Errorf("instance created despite 403: %+v", res)
	}
}

// ---- T20 orchestrator management guard --------------------------------------

func t20(t *testing.T, h *harness) {
	svc := h.svc("Guard")
	l := newListener(t, http.StatusOK)
	sub := h.subscribe(t, h.cons(), h.prov(), svc, l.url) // subscribe needs no token

	before, err := h.orch(t).History(ctx(t), h.sysopToken)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.orch(t).TriggerPush(ctx(t), "", sub.ID)
	e := apiErr(t, "trigger without token", err, 401)
	envelope(t, "trigger without token", e, models.ExceptionAuth)
	if e.Envelope.Origin != "dynamicorch-xacml" {
		t.Errorf("origin %q", e.Envelope.Origin)
	}
	_, err = h.orch(t).TriggerPush(ctx(t), h.tokens[h.cons()], sub.ID)
	envelope(t, "trigger with non-sysop token", apiErr(t, "trigger with non-sysop token", err, 403), models.ExceptionForbidden)
	_, err = h.orch(t).History(ctx(t), "")
	apiErr(t, "history without token", err, 401)

	if _, ok := l.wait(5 * time.Second); ok {
		t.Error("refused trigger still delivered a notification")
	}
	after, err := h.orch(t).History(ctx(t), h.sysopToken)
	if err != nil {
		t.Fatal(err)
	}
	if count(after, models.HistoryTypePush) != count(before, models.HistoryTypePush) {
		t.Error("refused trigger created a PUSH history entry")
	}
	if _, err := h.orch(t).Pull(ctx(t), orchestration.PullRequest(h.cons(), svc)); err != nil {
		t.Errorf("pull without token: %v", err)
	}
}

func count(hist models.HistoryResponse, typ string) int {
	n := 0
	for _, e := range hist.Entries {
		if e.Type == typ {
			n++
		}
	}
	return n
}
