package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/internal/testpki"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/models"
)

func TestConstructorsRefuseTheWrongScheme(t *testing.T) {
	if _, err := NewPlain("https://localhost:8083"); err == nil {
		t.Error("NewPlain accepted an https URL")
	}
	if _, err := NewPlain("localhost:8083"); err == nil {
		t.Error("NewPlain accepted a URL without scheme")
	}
	pki := testpki.New(t)
	c, k := pki.Issue(t, "TestHarness", "sy")
	creds, err := CredentialsFromPEM(c, k, pki.PEM)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewMTLS("http://localhost:8490", "serviceregistry", creds); err == nil {
		t.Error("NewMTLS accepted an http URL")
	}
	if _, err := NewMTLS("https://localhost:8490", "", creds); err == nil {
		t.Error("NewMTLS accepted an empty server name")
	}
	if _, err := NewMTLS("https://localhost:8490", "serviceregistry", Credentials{}); err == nil {
		t.Error("NewMTLS accepted empty credentials")
	}
	m, err := NewMTLS("https://localhost:8490/", "serviceregistry", creds)
	if err != nil {
		t.Fatal(err)
	}
	if m.Mode() != MutualTLS || m.ServerName() != "serviceregistry" || m.BaseURL() != "https://localhost:8490" {
		t.Errorf("mtls client: mode=%v server=%q base=%q", m.Mode(), m.ServerName(), m.BaseURL())
	}
	p, _ := NewPlain("http://localhost:8083")
	if p.Mode() != PlainHTTP || p.ServerName() != "" {
		t.Errorf("plain client: mode=%v server=%q", p.Mode(), p.ServerName())
	}
}

func TestPlainRequestOnTheWire(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotCT string
	var gotBody []byte
	var authPresent bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotCT = r.Method, r.URL.EscapedPath(), r.Header.Get("Content-Type")
		gotAuth = r.Header.Get("Authorization")
		_, authPresent = r.Header["Authorization"]
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"triggered"}`))
	}))
	defer srv.Close()

	c, err := NewPlain(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Do(context.Background(), Request{
		Method: http.MethodPost, Path: "/serviceorchestration/orchestration/mgmt/push/trigger",
		Token: "tok", Body: models.PushTriggerRequest{SubscriptionID: "s1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "POST" || gotPath != "/serviceorchestration/orchestration/mgmt/push/trigger" ||
		gotAuth != "Bearer tok" || gotCT != "application/json" || string(gotBody) != `{"subscriptionId":"s1"}` {
		t.Errorf("wire: %s %s auth=%q ct=%q body=%s", gotMethod, gotPath, gotAuth, gotCT, gotBody)
	}
	var out models.PushTriggerResponse
	if err := resp.DecodeJSON(&out); err != nil || out.Status != "triggered" || resp.StatusCode != 201 {
		t.Errorf("response: %+v status=%d err=%v", out, resp.StatusCode, err)
	}

	// No token, no body: no Authorization header and no Content-Type.
	if _, err := c.Do(context.Background(), Request{Method: http.MethodDelete, Path: "/x/PR%7CLOCAL%7CP"}); err != nil {
		t.Fatal(err)
	}
	if authPresent || gotCT != "" || len(gotBody) != 0 || gotPath != "/x/PR%7CLOCAL%7CP" {
		t.Errorf("token-less call: authPresent=%v ct=%q body=%q path=%q", authPresent, gotCT, gotBody, gotPath)
	}
}

func TestMTLSServerNameAndClientCertificate(t *testing.T) {
	pki := testpki.New(t)
	var seenCN string
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenCN = r.TLS.PeerCertificates[0].Subject.CommonName
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	srv.TLS = pki.ServerTLS(t, "serviceregistry") // SAN is the service name only, as in the stack
	srv.StartTLS()
	defer srv.Close()
	base := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)

	c, k := pki.Issue(t, "TestHarness", "sy")
	creds, err := CredentialsFromPEM(c, k, pki.PEM)
	if err != nil {
		t.Fatal(err)
	}
	if creds.Leaf.Subject.CommonName != "TestHarness" || creds.Leaf.Subject.OrganizationalUnit[0] != "sy" {
		t.Errorf("leaf: %v", creds.Leaf.Subject)
	}

	good, _ := NewMTLS(base, "serviceregistry", creds)
	if _, err := good.Do(context.Background(), Request{Method: "GET", Path: "/health"}); err != nil {
		t.Fatalf("explicit server name: %v", err)
	}
	if seenCN != "TestHarness" {
		t.Errorf("server saw client CN %q", seenCN)
	}

	wrong, _ := NewMTLS(base, "localhost", creds)
	_, err = wrong.Do(context.Background(), Request{Method: "GET", Path: "/health"})
	if err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("server name localhost should fail verification, got %v", err)
	}

	// A plain client cannot talk to the TLS listener at all.
	plain, _ := NewPlain(strings.Replace(base, "https", "http", 1))
	if _, err := plain.Do(context.Background(), Request{Method: "GET", Path: "/health"}); err == nil {
		t.Error("plain client reached a TLS-only server")
	}
}

func TestMTLSRefusedWithoutAClientCertificate(t *testing.T) {
	pki := testpki.New(t)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.TLS = pki.ServerTLS(t, "profile-ca")
	srv.StartTLS()
	defer srv.Close()

	// Credentials issued by a different CA: the handshake fails, no HTTP status.
	other := testpki.New(t)
	c, k := other.Issue(t, "Stranger", "on")
	creds, _ := CredentialsFromPEM(c, k, pki.PEM)
	cl, _ := NewMTLS(srv.URL, "profile-ca", creds)
	_, err := cl.Do(context.Background(), Request{Method: "POST", Path: "/ca/device-cert", Body: models.CertificateRequest{SystemName: "X"}})
	if err == nil || StatusCode(err) != 0 {
		t.Errorf("want a transport error without HTTP status, got %v (status %d)", err, StatusCode(err))
	}
}

func TestErrorBodies(t *testing.T) {
	cases := []struct {
		name, body  string
		status      int
		wantMsg     string
		wantEnvelop bool
	}{
		{"AH5 envelope", `{"errorMessage":"systemName must be PascalCase","errorCode":400,"exceptionType":"INVALID_PARAMETER","origin":"serviceregistry"}`, 400, "systemName must be PascalCase", true},
		{"envelope with empty origin", `{"errorMessage":"invalid JSON","errorCode":400,"exceptionType":"INVALID_PARAMETER","origin":""}`, 400, "invalid JSON", true},
		{"profile-ca", `{"error":"requester profile: not on"}`, 403, "requester profile: not on", false},
		{"orchestrator plain text", "404 page not found\n", 404, "404 page not found", false},
		{"empty body", "", 401, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c, _ := NewPlain(srv.URL)
			resp, err := c.Do(context.Background(), Request{Method: "POST", Path: "/p", Body: struct{}{}})
			if resp != nil {
				t.Fatalf("non-2xx returned a response")
			}
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("want *Error, got %T %v", err, err)
			}
			if e.StatusCode != tc.status || StatusCode(err) != tc.status || e.Message != tc.wantMsg ||
				(e.Envelope != nil) != tc.wantEnvelop || string(e.Body) != tc.body {
				t.Errorf("error = %+v", e)
			}
			if tc.wantEnvelop && e.Envelope.ExceptionType != models.ExceptionInvalidParameter {
				t.Errorf("envelope = %+v", e.Envelope)
			}
		})
	}
}

func TestNoRedirectsNoRetries(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/moved" {
			http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c, _ := NewPlain(srv.URL)
	_, err := c.Do(context.Background(), Request{Method: "POST", Path: "/moved", Body: struct{}{}})
	if StatusCode(err) != http.StatusTemporaryRedirect {
		t.Errorf("redirect: want status 307 as an error, got %v", err)
	}
	calls = 0
	_, err = c.Do(context.Background(), Request{Method: "POST", Path: "/busy", Body: struct{}{}})
	if StatusCode(err) != 503 || calls != 1 {
		t.Errorf("want exactly one call and 503, got %d calls, err %v", calls, err)
	}
}

func TestTimeoutOption(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)
	c, _ := NewPlain(srv.URL, WithTimeout(50*time.Millisecond))
	start := time.Now()
	if _, err := c.Do(context.Background(), Request{Method: "GET", Path: "/slow"}); err == nil {
		t.Fatal("want timeout error")
	}
	if time.Since(start) > 2*time.Second {
		t.Error("timeout not applied")
	}
}

func TestPathMustBeAbsolute(t *testing.T) {
	c, _ := NewPlain("http://localhost:1")
	if _, err := c.Do(context.Background(), Request{Method: "GET", Path: "health"}); err == nil {
		t.Error("relative path accepted")
	}
}

func TestLoadCredentials(t *testing.T) {
	pki := testpki.New(t)
	c, k := pki.Issue(t, "TestHarness", "sy")
	dir := t.TempDir()
	cf, kf, af := filepath.Join(dir, "c.pem"), filepath.Join(dir, "k.pem"), filepath.Join(dir, "ca.pem")
	for f, b := range map[string][]byte{cf: c, kf: k, af: pki.PEM} {
		if err := os.WriteFile(f, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	creds, err := LoadCredentials(cf, kf, af)
	if err != nil || creds.Leaf.Subject.CommonName != "TestHarness" || creds.Roots == nil {
		t.Fatalf("LoadCredentials: %+v %v", creds.Leaf, err)
	}
	if _, err := LoadCredentials(cf, kf, filepath.Join(dir, "missing.pem")); err == nil {
		t.Error("missing CA file accepted")
	}
	if _, err := CredentialsFromPEM(c, k, []byte("not pem")); err == nil {
		t.Error("CA bundle without certificates accepted")
	}
	_, otherKey := pki.Issue(t, "Other", "sy")
	if _, err := CredentialsFromPEM(c, otherKey, pki.PEM); err == nil {
		t.Error("mismatched key accepted")
	}
}
