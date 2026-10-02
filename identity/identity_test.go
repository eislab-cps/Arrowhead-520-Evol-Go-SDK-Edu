package identity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/internal/testpki"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/transport"
)

// fakeAuth imitates Authentication's login handler over mTLS and records the
// raw request body.
func fakeAuth(t *testing.T, raw *map[string]any) *transport.Client {
	t.Helper()
	ca := testpki.New(t)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/authentication/identity/login" {
			http.NotFound(w, r)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, raw)
		w.Header().Set("Content-Type", "application/json")
		creds, _ := (*raw)["credentials"].(map[string]any)
		if (*raw)["systemName"] != "TCons1" || creds["password"] != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"errorMessage":"invalid credentials","errorCode":401,"exceptionType":"AUTH_EXCEPTION","origin":"authentication"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"token":"tok-1","systemName":"TCons1","expirationTime":"2030-01-01T00:00:00Z","sysop":false}`))
	}))
	srv.TLS = ca.ServerTLS(t, "authentication")
	srv.StartTLS()
	t.Cleanup(srv.Close)

	cert, key := ca.Issue(t, "TestHarness", "sy")
	creds, err := transport.CredentialsFromPEM(cert, key, ca.PEM)
	if err != nil {
		t.Fatal(err)
	}
	c, err := transport.NewMTLS(srv.URL, "authentication", creds)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLogin(t *testing.T) {
	var raw map[string]any
	id := New(fakeAuth(t, &raw))
	got, err := id.Login(context.Background(), "TCons1", "secret")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"systemName": "TCons1", "credentials": map[string]any{"password": "secret"}}
	if !reflect.DeepEqual(raw, want) {
		t.Errorf("wire body = %v, want %v", raw, want)
	}
	if got.Token != "tok-1" || got.SystemName != "TCons1" || got.Sysop || got.ExpirationTime != "2030-01-01T00:00:00Z" {
		t.Errorf("response = %+v", got)
	}
}

func TestLoginWrongPasswordIs401(t *testing.T) {
	var raw map[string]any
	id := New(fakeAuth(t, &raw))
	_, err := id.Login(context.Background(), "TCons1", "wrong")
	if transport.StatusCode(err) != http.StatusUnauthorized {
		t.Fatalf("want 401, got %v", err)
	}
}

func TestLoginRefusesEmptyName(t *testing.T) {
	c, _ := transport.NewPlain("http://localhost:1")
	if _, err := New(c).Login(context.Background(), "", "p"); err == nil {
		t.Fatal("empty system name accepted")
	}
}
