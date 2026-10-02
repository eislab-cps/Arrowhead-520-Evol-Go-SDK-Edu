package orchestration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/models"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/transport"
)

type recorder struct {
	method, rawPath, auth string
	body                  map[string]any
}

func fake(t *testing.T, rec *recorder, h func(w http.ResponseWriter, r *http.Request)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method, rec.rawPath, rec.auth = r.Method, r.URL.EscapedPath(), r.Header.Get("Authorization")
		rec.body = nil
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			_ = json.Unmarshal(b, &rec.body)
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	c, _ := transport.NewPlain(srv.URL)
	return New(c)
}

func TestPullWireAndEmpty(t *testing.T) {
	var rec recorder
	body := `{"response":[{"provider":{"systemName":"TemperatureProvider","address":"10.0.0.5","port":9000},
"service":{"serviceDefinition":"temperatureService","serviceUri":"/temperature","interfaces":["generic_http"],"version":1},"cloudIdentifier":"LOCAL"}]}`
	c := fake(t, &rec, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) })
	res, err := c.Pull(context.Background(), PullRequest("ConsumerApp", "temperatureService"))
	if err != nil || len(res.Response) != 1 || res.Response[0].Provider.Port != 9000 || res.Response[0].Service.ServiceURI != "/temperature" {
		t.Fatalf("pull: %+v %v", res, err)
	}
	want := map[string]any{
		"requesterSystem":  map[string]any{"systemName": "ConsumerApp"},
		"requestedService": map[string]any{"serviceDefinition": "temperatureService"},
	}
	if rec.method != "POST" || rec.rawPath != PullPath || rec.auth != "" || !reflect.DeepEqual(rec.body, want) {
		t.Errorf("wire: %s %s auth=%q body=%v", rec.method, rec.rawPath, rec.auth, rec.body)
	}
	body = `{"response":[]}`
	res, err = c.Pull(context.Background(), PullRequest("Other", "temperatureService"))
	if err != nil || len(res.Response) != 0 {
		t.Errorf("empty pull: %+v %v", res, err)
	}
}

func TestSubscribeUnsubscribe(t *testing.T) {
	var rec recorder
	status := http.StatusCreated
	c := fake(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(status)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"id":"sub-1","ownerSystemName":"ConsumerApp","targetSystemName":"TemperatureProvider",
"orchestrationRequest":{"requesterSystem":{"systemName":"ConsumerApp"},"requestedService":{"serviceDefinition":"temperatureService"}},
"notifyInterface":{"notifyUri":"http://h:9100/notify"},"createdAt":"c"}`))
	})
	req := models.SubscriptionRequest{
		OwnerSystemName: "ConsumerApp", TargetSystemName: "TemperatureProvider",
		OrchestrationRequest: PullRequest("ConsumerApp", "temperatureService"),
		NotifyInterface:      &models.NotifyInterface{NotifyURI: "http://h:9100/notify"},
	}
	sub, created, err := c.Subscribe(context.Background(), req)
	if err != nil || !created || sub.ID != "sub-1" {
		t.Fatalf("subscribe: %+v %v %v", sub, created, err)
	}
	if rec.rawPath != SubscribePath || rec.body["notifyInterface"].(map[string]any)["notifyUri"] != "http://h:9100/notify" {
		t.Errorf("wire: %s %v", rec.rawPath, rec.body)
	}
	status = http.StatusOK
	if _, created, _ = c.Subscribe(context.Background(), req); created {
		t.Error("200 reported as created")
	}

	removed, err := c.Unsubscribe(context.Background(), "sub-1")
	if err != nil || !removed || rec.method != "DELETE" || rec.rawPath != UnsubscribePathBase+"sub-1" {
		t.Errorf("unsubscribe: %v %v %s %s", removed, err, rec.method, rec.rawPath)
	}
	status = http.StatusNoContent
	if removed, err = c.Unsubscribe(context.Background(), "gone"); err != nil || removed {
		t.Errorf("204: %v %v", removed, err)
	}
}

func TestManagementCallsCarryTheToken(t *testing.T) {
	var rec recorder
	c := fake(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"errorMessage":"Authorization: Bearer token required for management access","errorCode":401,"exceptionType":"AUTH_EXCEPTION","origin":"dynamicorch-xacml"}`))
			return
		}
		if r.URL.Path == TriggerPath {
			_, _ = w.Write([]byte(`{"status":"triggered"}`))
			return
		}
		_, _ = w.Write([]byte(`{"entries":[{"id":"h","status":"DONE","type":"PULL","createdAt":"c"}],"count":1}`))
	})
	tr, err := c.TriggerPush(context.Background(), "sysop-tok", "sub-1")
	if err != nil || tr.Status != "triggered" || rec.auth != "Bearer sysop-tok" ||
		!reflect.DeepEqual(rec.body, map[string]any{"subscriptionId": "sub-1"}) {
		t.Errorf("trigger: %+v %v auth=%q body=%v", tr, err, rec.auth, rec.body)
	}
	h, err := c.History(context.Background(), "sysop-tok")
	if err != nil || h.Count != 1 || rec.rawPath != HistoryPath || rec.auth != "Bearer sysop-tok" {
		t.Errorf("history: %+v %v", h, err)
	}
	_, err = c.TriggerPush(context.Background(), "", "sub-1")
	var e *transport.Error
	if transport.StatusCode(err) != 401 {
		t.Fatalf("no token: want 401, got %v", err)
	}
	e = err.(*transport.Error)
	if e.Envelope == nil || e.Envelope.Origin != "dynamicorch-xacml" {
		t.Errorf("envelope: %+v", e.Envelope)
	}
}

func TestDecodeNotification(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/notify",
		strings.NewReader(`{"subscriptionId":"s","ownerSystemName":"o","targetSystemName":"t"}`))
	n, err := DecodeNotification(r)
	if err != nil || n.SubscriptionID != "s" || n.OwnerSystemName != "o" || n.TargetSystemName != "t" {
		t.Errorf("%+v %v", n, err)
	}
	if _, err := DecodeNotification(httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(`{}`))); err == nil {
		t.Error("notification without id accepted")
	}
	if _, err := DecodeNotification(httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(`nope`))); err == nil {
		t.Error("non-JSON accepted")
	}
}
