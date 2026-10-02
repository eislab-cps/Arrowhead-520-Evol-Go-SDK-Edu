package ca

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/internal/testpki"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/models"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/transport"
)

// fakeProfileCA imitates profile-ca: a plain server for /ca/info and
// onboarding, and an mTLS server for device and system certificates that
// checks the OU of the presented certificate. issueProfile overrides the
// profile written into responses, to test mismatch handling.
type fakeProfileCA struct {
	ca           *testpki.CA
	plain, tls   *httptest.Server
	lastBody     map[string]any
	issueProfile string
}

func newFake(t *testing.T) *fakeProfileCA {
	t.Helper()
	f := &fakeProfileCA{ca: testpki.New(t)}
	issue := func(w http.ResponseWriter, r *http.Request, profile string) {
		b, _ := io.ReadAll(r.Body)
		f.lastBody = map[string]any{}
		if json.Unmarshal(b, &f.lastBody) != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid JSON"}`))
			return
		}
		name, _ := f.lastBody["systemName"].(string)
		cert, key := f.ca.Issue(t, name, profile)
		out := profile
		if f.issueProfile != "" {
			out = f.issueProfile
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(models.IssuedCertificate{
			SystemName: name, Certificate: string(cert), PrivateKey: string(key), Profile: out, IssuedAt: "2026-09-30T00:00:00Z",
		})
	}
	requireOU := func(want string, next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if ou := r.TLS.PeerCertificates[0].Subject.OrganizationalUnit; len(ou) == 0 || ou[0] != want {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"requester profile: certificate is not OU=` + want + `"}`))
				return
			}
			next(w, r)
		}
	}
	pm := http.NewServeMux()
	pm.HandleFunc("GET /ca/info", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(models.CAInfo{CommonName: "Arrowhead Local Cloud CA", Certificate: string(f.ca.PEM)})
	})
	pm.HandleFunc("POST /bootstrap/onboarding-cert", func(w http.ResponseWriter, r *http.Request) { issue(w, r, "on") })
	f.plain = httptest.NewServer(pm)
	t.Cleanup(f.plain.Close)

	tm := http.NewServeMux()
	tm.HandleFunc("POST /ca/device-cert", requireOU("on", func(w http.ResponseWriter, r *http.Request) { issue(w, r, "de") }))
	tm.HandleFunc("POST /ca/system-cert", requireOU("de", func(w http.ResponseWriter, r *http.Request) { issue(w, r, "sy") }))
	f.tls = httptest.NewUnstartedServer(tm)
	f.tls.TLS = f.ca.ServerTLS(t, "profile-ca")
	f.tls.StartTLS()
	t.Cleanup(f.tls.Close)
	return f
}

func ouOf(t *testing.T, certPEM string) string {
	t.Helper()
	b, _ := pem.Decode([]byte(certPEM))
	c, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return c.Subject.OrganizationalUnit[0]
}

func mtls(t *testing.T, f *fakeProfileCA, issued models.IssuedCertificate, caPEM string) *Client {
	t.Helper()
	creds, err := Credentials(issued, caPEM)
	if err != nil {
		t.Fatal(err)
	}
	tc, err := transport.NewMTLS(f.tls.URL, "profile-ca", creds)
	if err != nil {
		t.Fatal(err)
	}
	return New(tc)
}

func TestChainOnboardingDeviceSystem(t *testing.T) {
	f := newFake(t)
	ctx := context.Background()
	plain, _ := transport.NewPlain(f.plain.URL)
	pc := New(plain)

	info, err := pc.Info(ctx)
	if err != nil || info.CommonName != "Arrowhead Local Cloud CA" {
		t.Fatalf("info: %+v %v", info, err)
	}

	on, err := pc.RequestCertificate(ctx, models.ProfileOnboarding, "TestHarness")
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]any{"systemName": "TestHarness"}; len(f.lastBody) != 1 || f.lastBody["systemName"] != want["systemName"] {
		t.Errorf("wire body = %v", f.lastBody)
	}
	if on.Profile != "on" || ouOf(t, on.Certificate) != "on" {
		t.Errorf("onboarding: profile %q OU %q", on.Profile, ouOf(t, on.Certificate))
	}

	de, err := mtls(t, f, on, info.Certificate).RequestCertificate(ctx, models.ProfileDevice, "TestHarness")
	if err != nil || ouOf(t, de.Certificate) != "de" {
		t.Fatalf("device: %+v %v", de.Profile, err)
	}
	sy, err := mtls(t, f, de, info.Certificate).RequestCertificate(ctx, models.ProfileSystem, "TestHarness")
	if err != nil || ouOf(t, sy.Certificate) != "sy" {
		t.Fatalf("system: %+v %v", sy.Profile, err)
	}

	// The "sy" certificate is usable as mTLS credentials.
	creds, err := Credentials(sy, info.Certificate)
	if err != nil || creds.Leaf.Subject.CommonName != "TestHarness" {
		t.Fatalf("credentials: %v", err)
	}
}

func TestWrongProfilePresentedIs403(t *testing.T) {
	f := newFake(t)
	ctx := context.Background()
	plain, _ := transport.NewPlain(f.plain.URL)
	info, _ := New(plain).Info(ctx)
	on, _ := New(plain).RequestCertificate(ctx, models.ProfileOnboarding, "TestHarness")
	de, _ := mtls(t, f, on, info.Certificate).RequestCertificate(ctx, models.ProfileDevice, "TestHarness")

	// system-cert with the "on" certificate, device-cert with the "de" certificate.
	if _, err := mtls(t, f, on, info.Certificate).RequestCertificate(ctx, models.ProfileSystem, "X"); transport.StatusCode(err) != 403 {
		t.Errorf("system-cert with on: want 403, got %v", err)
	}
	_, err := mtls(t, f, de, info.Certificate).RequestCertificate(ctx, models.ProfileDevice, "X")
	var e *transport.Error
	if transport.StatusCode(err) != 403 || !errors.As(err, &e) || e.Message == "" {
		t.Errorf("device-cert with de: want 403 with message, got %v", err)
	}
}

func TestModeMustMatchProfile(t *testing.T) {
	f := newFake(t)
	plain, _ := transport.NewPlain(f.plain.URL)
	if _, err := New(plain).RequestCertificate(context.Background(), models.ProfileDevice, "X"); err == nil || transport.StatusCode(err) != 0 {
		t.Errorf("device over plain HTTP: want a local error, got %v", err)
	}
	if _, err := New(plain).RequestCertificate(context.Background(), "xx", "X"); err == nil {
		t.Error("unknown profile accepted")
	}
	if _, err := New(plain).RequestCertificate(context.Background(), models.ProfileOnboarding, ""); err == nil {
		t.Error("empty name accepted")
	}
}

func TestIssuedProfileMismatchIsAnError(t *testing.T) {
	f := newFake(t)
	f.issueProfile = "sy"
	plain, _ := transport.NewPlain(f.plain.URL)
	got, err := New(plain).RequestCertificate(context.Background(), models.ProfileOnboarding, "TestHarness")
	if err == nil || got.Profile != "sy" {
		t.Errorf("want error and the returned certificate, got %+v %v", got.Profile, err)
	}
}

func TestSave(t *testing.T) {
	dir := t.TempDir()
	cf, kf := filepath.Join(dir, "sys.crt"), filepath.Join(dir, "sys.key")
	if err := Save(models.IssuedCertificate{Certificate: "CERT", PrivateKey: "KEY"}, cf, kf); err != nil {
		t.Fatal(err)
	}
	c, _ := os.ReadFile(cf)
	k, _ := os.ReadFile(kf)
	st, _ := os.Stat(kf)
	if string(c) != "CERT" || string(k) != "KEY" || st.Mode().Perm() != 0o600 {
		t.Errorf("cert=%q key=%q mode=%v", c, k, st.Mode().Perm())
	}
}
