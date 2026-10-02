//go:build contract

// Package contract is tier 3: every CONTRACT.md row exercised against the
// pinned stack started by run.sh. Run it only through run.sh.
package contract

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/transport"
)

// harness is the state shared by the ordered scenarios.
type harness struct {
	sfx     string // per-run suffix for unique AH5 names
	root    string // repository root, for go run ./examples/...
	certDir string // where examples/ca wrote the harness certificates

	caPEM string
	creds transport.Credentials // TestHarness, OU=sy after T16

	sysopToken string
	passwords  map[string]string
	tokens     map[string]string
}

func env(t *testing.T, name string) string {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		t.Fatalf("%s is not set; run tier 3 through test/contract/run.sh", name)
	}
	return v
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return c
}

// mtls returns an mTLS client with the harness credentials.
func (h *harness) mtls(t *testing.T, urlEnv, serverName string) *transport.Client {
	t.Helper()
	c, err := transport.NewMTLS(env(t, urlEnv), serverName, h.creds)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func plain(t *testing.T, urlEnv string) *transport.Client {
	t.Helper()
	c, err := transport.NewPlain(env(t, urlEnv))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// wire decodes a raw body into a generic map, for assertions on literal keys.
func wire(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("body is not a JSON object: %v: %q", err, body)
	}
	return m
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// hasKeys fails unless m has at least the given keys.
func hasKeys(t *testing.T, what string, m map[string]any, want ...string) {
	t.Helper()
	for _, k := range want {
		if _, ok := m[k]; !ok {
			t.Errorf("%s: key %q missing; keys %v", what, k, keys(m))
		}
	}
}

// exactKeys fails unless m has exactly the given keys.
func exactKeys(t *testing.T, what string, m map[string]any, want ...string) {
	t.Helper()
	sort.Strings(want)
	if got := keys(m); !reflect.DeepEqual(got, want) {
		t.Errorf("%s: keys %v, want exactly %v", what, got, want)
	}
}

// apiErr returns the *transport.Error in err, failing the test if absent or
// if its status differs from want.
func apiErr(t *testing.T, what string, err error, want int) *transport.Error {
	t.Helper()
	var e *transport.Error
	if !errors.As(err, &e) {
		t.Fatalf("%s: want HTTP %d, got %v", what, want, err)
	}
	if e.StatusCode != want {
		t.Fatalf("%s: status %d, want %d; body %s", what, e.StatusCode, want, e.Body)
	}
	return e
}

// envelope asserts the AH5 error envelope with the given exception type.
func envelope(t *testing.T, what string, e *transport.Error, exceptionType string) {
	t.Helper()
	m := wire(t, e.Body)
	exactKeys(t, what+" error body", m, "errorMessage", "errorCode", "exceptionType", "origin")
	if m["exceptionType"] != exceptionType || int(m["errorCode"].(float64)) != e.StatusCode {
		t.Errorf("%s: envelope %v, want exceptionType %s", what, m, exceptionType)
	}
}

// handshakeRefused asserts that an https request presenting no client
// certificate fails in the TLS handshake, with no HTTP status at all.
func (h *harness) handshakeRefused(t *testing.T, url, serverName, path string) {
	t.Helper()
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM([]byte(h.caPEM))
	c := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: serverName, MinVersion: tls.VersionTLS12},
	}}
	resp, err := c.Post(url+path, "application/json", strings.NewReader(`{}`))
	if err == nil {
		resp.Body.Close()
		t.Fatalf("%s%s without client certificate: got HTTP %d, want a handshake failure", url, path, resp.StatusCode)
	}
	t.Logf("no client certificate on %s: %v", url, err)
}

func parseCert(t *testing.T, certPEM string) *x509.Certificate {
	t.Helper()
	b, _ := pem.Decode([]byte(certPEM))
	if b == nil {
		t.Fatalf("not PEM: %.40q", certPEM)
	}
	c, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// runExample runs go run ./examples/<name> with args and requires exit 0.
func (h *harness) runExample(t *testing.T, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "./examples/" + name}, args...)...)
	cmd.Dir = h.root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("examples/%s: %v\n%s", name, err, out)
	}
	t.Logf("examples/%s:\n%s", name, out)
	return string(out)
}

func TestContract(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{
		sfx:       fmt.Sprint(time.Now().Unix()),
		root:      root,
		certDir:   t.TempDir(),
		passwords: map[string]string{},
		tokens:    map[string]string{},
	}
	steps := []struct {
		name string
		fn   func(*testing.T, *harness)
	}{
		{"T0_harness_matches_stack", t0},
		{"T13_ca_info", t13},
		{"T14_onboarding", t14},
		{"T15_device_cert", t15},
		{"T16_system_cert", t16},
		{"T1_login", t1},
		{"T2_register", t2},
		{"T3_lookup", t3},
		{"T4_revoke", t4},
		{"T5_pull", t5},
		{"T6_subscribe", t6},
		{"T7_unsubscribe", t7},
		{"T8_push_trigger", t8},
		{"T9_history", t9},
		{"T10_grant", t10},
		{"T11_revoke_policy", t11},
		{"T12_lookup_policies", t12},
		{"T17_mqtt", t17},
		{"T18_consumerauth_denial", t18},
		{"T19_foreign_token", t19},
		{"T20_orchestrator_mgmt_guard", t20},
	}
	for i, s := range steps {
		ok := t.Run(s.name, func(t *testing.T) { s.fn(t, h) })
		// T0, T13–T16 and T1 build the credentials and identities every later
		// scenario uses; after them, scenarios are independent.
		if !ok && i < 6 {
			t.Fatalf("%s failed; later scenarios depend on it", s.name)
		}
	}
}
