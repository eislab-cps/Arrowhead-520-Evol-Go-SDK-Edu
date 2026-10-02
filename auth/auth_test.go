package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
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
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	c, _ := transport.NewPlain(srv.URL)
	return New(c)
}

const policyJSON = `{"instanceId":"PR|LOCAL|TemperatureProvider|SERVICE_DEF|temperatureService","authorizationLevel":"PR","cloud":"LOCAL",
"provider":"TemperatureProvider","targetType":"SERVICE_DEF","target":"temperatureService",
"defaultPolicy":{"policyType":"WHITELIST","policyList":["ConsumerApp"]},"scopedPolicies":{},"createdAt":"2026-09-30T00:00:00Z"}`

func TestGrantWire(t *testing.T) {
	var rec recorder
	c := fake(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(policyJSON))
	})
	p, err := c.Grant(context.Background(), AllowConsumers("TemperatureProvider", "temperatureService", "ConsumerApp"))
	if err != nil || p.InstanceID != models.PolicyInstanceID("TemperatureProvider", models.TargetTypeServiceDef, "temperatureService") {
		t.Fatalf("grant: %+v %v", p, err)
	}
	want := map[string]any{
		"provider": "TemperatureProvider", "targetType": "SERVICE_DEF", "target": "temperatureService",
		"defaultPolicy": map[string]any{"policyType": "WHITELIST", "policyList": []any{"ConsumerApp"}},
	}
	if rec.method != "POST" || rec.rawPath != GrantPath || rec.auth != "" || !reflect.DeepEqual(rec.body, want) {
		t.Errorf("wire: %s %s auth=%q body=%v", rec.method, rec.rawPath, rec.auth, rec.body)
	}
}

func TestGrantConflict(t *testing.T) {
	var rec recorder
	c := fake(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"errorMessage":"authorization policy already exists","errorCode":409,"exceptionType":"ARROWHEAD_EXCEPTION","origin":"consumerauth"}`))
	})
	if _, err := c.Grant(context.Background(), AllowConsumers("P", "s", "C")); transport.StatusCode(err) != 409 {
		t.Errorf("want 409, got %v", err)
	}
}

func TestRevokeEncodesPipes(t *testing.T) {
	var rec recorder
	status := http.StatusOK
	c := fake(t, &rec, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) })
	id := models.PolicyInstanceID("TemperatureProvider", models.TargetTypeServiceDef, "temperatureService")
	if err := c.Revoke(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if want := RevokePathBase + "PR%7CLOCAL%7CTemperatureProvider%7CSERVICE_DEF%7CtemperatureService"; rec.method != "DELETE" || rec.rawPath != want {
		t.Errorf("wire: %s %s, want DELETE %s", rec.method, rec.rawPath, want)
	}
	status = http.StatusNotFound
	if err := c.Revoke(context.Background(), id); transport.StatusCode(err) != 404 {
		t.Errorf("want 404, got %v", err)
	}
	if err := c.Revoke(context.Background(), ""); err == nil {
		t.Error("empty ID accepted")
	}
}

func TestLookup(t *testing.T) {
	var rec recorder
	c := fake(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"policies":[` + policyJSON + `],"count":1,"totalCount":1}`))
	})
	res, err := c.Lookup(context.Background(), models.PolicyLookup{TargetNames: []string{"temperatureService"}})
	if err != nil || res.Count != 1 || res.Policies[0].DefaultPolicy.PolicyList[0] != "ConsumerApp" {
		t.Fatalf("lookup: %+v %v", res, err)
	}
	if !reflect.DeepEqual(rec.body, map[string]any{"targetNames": []any{"temperatureService"}}) {
		t.Errorf("body %v", rec.body)
	}
}
