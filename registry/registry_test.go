package registry

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

// recorder captures the last request as the server saw it.
type recorder struct {
	method, path, rawPath, auth string
	body                        map[string]any
}

func fakeSR(t *testing.T, rec *recorder, h func(w http.ResponseWriter, r *http.Request)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method, rec.path, rec.rawPath, rec.auth = r.Method, r.URL.Path, r.URL.EscapedPath(), r.Header.Get("Authorization")
		rec.body = nil
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			_ = json.Unmarshal(b, &rec.body)
		}
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	c, _ := transport.NewPlain(srv.URL) // the unit tests use plain HTTP; mTLS is covered in transport
	return New(c)
}

const instanceJSON = `{"instanceId":"i1","provider":{"name":"TemperatureProvider","createdAt":"a","updatedAt":"b"},
"serviceDefinitionName":"temperatureService","version":"1.0.0",
"interfaces":[{"templateName":"generic_http","protocol":"http","policy":"NONE","properties":{"accessAddresses":"10.0.0.5","accessPort":"9000","basePath":"/temperature"}}],
"createdAt":"c","updatedAt":"u"}`

func TestRegisterWireAndStatus(t *testing.T) {
	var rec recorder
	status := http.StatusCreated
	c := fakeSR(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(instanceJSON))
	})
	reg := models.ServiceRegistration{
		SystemName: "TemperatureProvider", ServiceDefinitionName: "temperatureService", Version: "1.0.0",
		Interfaces: []models.Interface{HTTPInterface("generic_http", "10.0.0.5", 9000, "/temperature")},
	}
	inst, created, err := c.Register(context.Background(), "tok", reg)
	if err != nil || !created || inst.InstanceID != "i1" {
		t.Fatalf("register: %+v created=%v err=%v", inst, created, err)
	}
	if rec.method != "POST" || rec.path != "/serviceregistry/service-discovery/register" || rec.auth != "Bearer tok" {
		t.Errorf("wire: %s %s auth=%q", rec.method, rec.path, rec.auth)
	}
	want := map[string]any{
		"systemName": "TemperatureProvider", "serviceDefinitionName": "temperatureService", "version": "1.0.0",
		"interfaces": []any{map[string]any{
			"templateName": "generic_http", "protocol": "http", "policy": "NONE",
			"properties": map[string]any{"accessAddresses": "10.0.0.5", "accessPort": "9000", "basePath": "/temperature"},
		}},
	}
	if !reflect.DeepEqual(rec.body, want) {
		t.Errorf("body = %v\nwant   %v", rec.body, want)
	}

	status = http.StatusOK
	if _, created, err = c.Register(context.Background(), "tok", reg); err != nil || created {
		t.Errorf("update: created=%v err=%v", created, err)
	}
}

func TestRegisterWithoutTokenSendsNoHeader(t *testing.T) {
	var rec recorder
	c := fakeSR(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorMessage":"registration identity check failed","errorCode":401,"exceptionType":"AUTH_EXCEPTION","origin":"serviceregistry"}`))
	})
	_, _, err := c.Register(context.Background(), "", models.ServiceRegistration{SystemName: "P", ServiceDefinitionName: "s"})
	if transport.StatusCode(err) != 401 || rec.auth != "" {
		t.Errorf("want 401 and no Authorization header, got %v auth=%q", err, rec.auth)
	}
}

func TestLookup(t *testing.T) {
	var rec recorder
	c := fakeSR(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"entries":[` + instanceJSON + `],"count":1,"totalCount":1}`))
	})
	res, err := c.Lookup(context.Background(), models.ServiceLookup{ServiceDefinitionNames: []string{"temperatureService"}})
	if err != nil || res.Count != 1 || res.Entries[0].Interfaces[0].Properties["accessPort"] != "9000" {
		t.Fatalf("lookup: %+v %v", res, err)
	}
	if !reflect.DeepEqual(rec.body, map[string]any{"serviceDefinitionNames": []any{"temperatureService"}}) || rec.auth != "" {
		t.Errorf("body = %v auth=%q", rec.body, rec.auth)
	}
}

func TestRevoke(t *testing.T) {
	var rec recorder
	status := http.StatusOK
	c := fakeSR(t, &rec, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) })
	removed, err := c.Revoke(context.Background(), "a/b c")
	if err != nil || !removed || rec.method != "DELETE" || rec.rawPath != "/serviceregistry/service-discovery/revoke/a%2Fb%20c" {
		t.Errorf("revoke: removed=%v err=%v %s %s", removed, err, rec.method, rec.rawPath)
	}
	status = http.StatusNoContent
	if removed, err = c.Revoke(context.Background(), "gone"); err != nil || removed {
		t.Errorf("204: removed=%v err=%v", removed, err)
	}
	if _, err := c.Revoke(context.Background(), ""); err == nil {
		t.Error("empty instance ID accepted")
	}
}
