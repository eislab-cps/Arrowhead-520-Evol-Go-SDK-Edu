//go:build contract

package contract

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/ca"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/identity"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/models"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/transport"
)

// stackServices are the six services compared by T0.
var stackServices = []string{"profile-ca", "cert-provisioner", "serviceregistry", "authentication", "consumerauth", "dynamicorch-xacml"}

// allowedExtraEnv is the only environment the harness adds to the stack's
// consumerauth compose (TEST_PLAN T0: token settings).
var allowedExtraEnv = map[string][]string{
	"serviceregistry":   {"REGISTER_AUTH_URL", "MGMT_AUTH_URL"},
	"authentication":    {"MGMT_AUTH_URL", "SYSOP_PASSWORD"},
	"consumerauth":      {"MGMT_AUTH_URL"},
	"dynamicorch-xacml": {"MGMT_AUTH_URL"},
}

func composeConfig(t *testing.T, file string) map[string]map[string]any {
	t.Helper()
	cmd := exec.Command("docker", "compose", "-f", file, "config", "--format", "json")
	cmd.Env = append(os.Environ(), "SYSOP_PASSWORD=x")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("docker compose config %s: %v", file, err)
	}
	var cfg struct {
		Services map[string]map[string]any `json:"services"`
	}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg.Services
}

// T0: the harness compose equals the stack's consumerauth compose except the
// documented token settings, and every image is built from STACK_REV.
func t0(t *testing.T, h *harness) {
	stack := composeConfig(t, env(t, "STACK_COMPOSE"))
	mine := composeConfig(t, env(t, "HARNESS_COMPOSE"))
	for _, svc := range stackServices {
		se, _ := stack[svc]["environment"].(map[string]any)
		me, _ := mine[svc]["environment"].(map[string]any)
		trimmed := map[string]any{}
		for k, v := range me {
			trimmed[k] = v
		}
		for _, k := range allowedExtraEnv[svc] {
			if _, ok := me[k]; !ok {
				t.Errorf("%s: harness is missing token setting %s", svc, k)
			}
			delete(trimmed, k)
		}
		if !reflect.DeepEqual(se, trimmed) {
			t.Errorf("%s environment differs from the stack beyond the token settings:\n stack   %v\n harness %v", svc, se, trimmed)
		}
		if !reflect.DeepEqual(stack[svc]["ports"], mine[svc]["ports"]) {
			t.Errorf("%s ports differ: stack %v, harness %v", svc, stack[svc]["ports"], mine[svc]["ports"])
		}
	}
	rev, tag := env(t, "STACK_REV"), os.Getenv("TAG")
	if tag == "" {
		tag = "v0.1.0"
	}
	for _, svc := range stackServices {
		img := "ghcr.io/ulfbod/" + svc + ":" + tag
		out, err := exec.Command("docker", "image", "inspect", "--format",
			`{{ index .Config.Labels "org.opencontainers.image.revision" }}`, img).Output()
		if err != nil || string(out) != rev+"\n" {
			t.Errorf("%s: revision label %q, want %q (%v)", img, out, rev, err)
		}
	}
}

// T13 (C13): CA certificate.
func t13(t *testing.T, h *harness) {
	resp, err := plain(t, "PCA_HTTP_URL").Do(ctx(t), transport.Request{Method: http.MethodGet, Path: ca.InfoPath})
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("GET /ca/info: %v", err)
	}
	exactKeys(t, "C13", wire(t, resp.Body), "commonName", "certificate")

	info, err := ca.New(plain(t, "PCA_HTTP_URL")).Info(ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	c := parseCert(t, info.Certificate)
	if !c.IsCA || c.Subject.CommonName != "Arrowhead Local Cloud CA" || info.CommonName != "Arrowhead Local Cloud CA" {
		t.Errorf("CA certificate: IsCA=%v CN=%q", c.IsCA, c.Subject.CommonName)
	}
	h.caPEM = info.Certificate
}

// T14 (C14): onboarding certificate, then examples/ca runs the whole chain.
func t14(t *testing.T, h *harness) {
	pc := plain(t, "PCA_HTTP_URL")
	resp, err := pc.Do(ctx(t), transport.Request{Method: http.MethodPost, Path: ca.OnboardingPath,
		Body: models.CertificateRequest{SystemName: "TestHarnessWire"}})
	if err != nil || resp.StatusCode != 201 {
		t.Fatalf("onboarding: %v", err)
	}
	m := wire(t, resp.Body)
	exactKeys(t, "C14", m, "systemName", "certificate", "privateKey", "profile", "issuedAt")
	if m["profile"] != "on" {
		t.Errorf("profile %v", m["profile"])
	}

	on, err := ca.New(pc).RequestCertificate(ctx(t), models.ProfileOnboarding, "TestHarness")
	if err != nil {
		t.Fatal(err)
	}
	c := parseCert(t, on.Certificate)
	if c.Subject.CommonName != "TestHarness" || c.Subject.OrganizationalUnit[0] != "on" {
		t.Errorf("onboarding cert subject %v", c.Subject)
	}
	if err := c.CheckSignatureFrom(parseCert(t, h.caPEM)); err != nil {
		t.Errorf("onboarding cert does not chain to the CA: %v", err)
	}
	if _, err := ca.Credentials(on, h.caPEM); err != nil {
		t.Errorf("private key does not match certificate: %v", err)
	}

	_, err = pc.Do(ctx(t), transport.Request{Method: http.MethodPost, Path: ca.OnboardingPath, Body: models.CertificateRequest{}})
	e := apiErr(t, "empty systemName", err, 400)
	exactKeys(t, "C14 error body", wire(t, e.Body), "error")

	h.creds, err = ca.Credentials(on, h.caPEM) // "on" for T15
	if err != nil {
		t.Fatal(err)
	}
	h.runExample(t, "ca", "-name", "TestHarnessExample", "-out", h.certDir,
		"-plain", env(t, "PCA_HTTP_URL"), "-tls", env(t, "PCA_TLS_URL"))
}

func pcaTLS(t *testing.T, h *harness, creds transport.Credentials) *ca.Client {
	t.Helper()
	c, err := transport.NewMTLS(env(t, "PCA_TLS_URL"), "localhost", creds)
	if err != nil {
		t.Fatal(err)
	}
	return ca.New(c)
}

// T15 (C15): device certificate presenting "on".
func t15(t *testing.T, h *harness) {
	de, err := pcaTLS(t, h, h.creds).RequestCertificate(ctx(t), models.ProfileDevice, "TestHarness")
	if err != nil {
		t.Fatal(err)
	}
	if ou := parseCert(t, de.Certificate).Subject.OrganizationalUnit[0]; ou != "de" || de.Profile != "de" {
		t.Errorf("device: profile %q OU %q", de.Profile, ou)
	}
	h.handshakeRefused(t, env(t, "PCA_TLS_URL"), "localhost", ca.DevicePath)

	deCreds, err := ca.Credentials(de, h.caPEM)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pcaTLS(t, h, deCreds).RequestCertificate(ctx(t), models.ProfileDevice, "TestHarness")
	e := apiErr(t, "device-cert presenting de", err, 403)
	exactKeys(t, "C15 error body", wire(t, e.Body), "error")
	h.creds = deCreds // "de" for T16
}

// T16 (C16): system certificate presenting "de"; it opens the foundation.
func t16(t *testing.T, h *harness) {
	sy, err := pcaTLS(t, h, h.creds).RequestCertificate(ctx(t), models.ProfileSystem, "TestHarness")
	if err != nil {
		t.Fatal(err)
	}
	if ou := parseCert(t, sy.Certificate).Subject.OrganizationalUnit[0]; ou != "sy" || sy.Profile != "sy" {
		t.Errorf("system: profile %q OU %q", sy.Profile, ou)
	}
	h.creds, err = ca.Credentials(sy, h.caPEM)
	if err != nil {
		t.Fatal(err)
	}

	// Presenting an "on" certificate to system-cert is refused.
	pc := plain(t, "PCA_HTTP_URL")
	on, err := ca.New(pc).RequestCertificate(ctx(t), models.ProfileOnboarding, "TestHarnessOn")
	if err != nil {
		t.Fatal(err)
	}
	onCreds, _ := ca.Credentials(on, h.caPEM)
	_, err = pcaTLS(t, h, onCreds).RequestCertificate(ctx(t), models.ProfileSystem, "X")
	apiErr(t, "system-cert presenting on", err, 403)

	// The "sy" certificate opens an mTLS connection to the ServiceRegistry;
	// no certificate is refused in the handshake.
	sr := h.mtls(t, "SR_URL", "serviceregistry")
	if _, err := sr.Do(ctx(t), transport.Request{Method: http.MethodPost, Path: "/serviceregistry/service-discovery/lookup",
		Body: models.ServiceLookup{ServiceDefinitionNames: []string{"nothing"}}}); err != nil {
		t.Errorf("SR with the sy certificate: %v", err)
	}
	h.handshakeRefused(t, env(t, "SR_URL"), "serviceregistry", "/serviceregistry/service-discovery/lookup")
}

// createIdentity is the operator fixture POST /authentication/mgmt/identities
// (not wrapped by the SDK). It returns the error, if any, for negative checks.
func (h *harness) createIdentity(t *testing.T, token, name, password string) error {
	t.Helper()
	body := map[string]any{
		"authenticationMethod": "PASSWORD",
		"identities": []any{map[string]any{
			"systemName": name, "credentials": map[string]any{"password": password}, "sysop": false, "createdBy": "contract-test",
		}},
	}
	_, err := h.mtls(t, "AUTH_URL", "authentication").Do(ctx(t), transport.Request{
		Method: http.MethodPost, Path: "/authentication/mgmt/identities", Token: token, Body: body})
	return err
}

// T1 (C1): login; fixture identities for later scenarios.
func t1(t *testing.T, h *harness) {
	auth := h.mtls(t, "AUTH_URL", "authentication")
	id := identity.New(auth)

	sysop, err := id.Login(ctx(t), "Sysop", env(t, "SYSOP_PASSWORD"))
	if err != nil || !sysop.Sysop || sysop.Token == "" {
		t.Fatalf("Sysop login: %+v %v", sysop, err)
	}
	h.sysopToken = sysop.Token

	// Fixture guard: no token 401, non-sysop 403, non-PascalCase 400.
	cons := "TCons" + h.sfx
	e := apiErr(t, "identities without token", h.createIdentity(t, "", cons, "pw"), 401)
	envelope(t, "identities without token", e, models.ExceptionAuth)
	for _, name := range []string{cons, "TProv" + h.sfx, "TOther" + h.sfx, "TPub" + h.sfx} {
		pw := "pw-" + name
		if err := h.createIdentity(t, h.sysopToken, name, pw); err != nil {
			t.Fatalf("create identity %s: %v", name, err)
		}
		h.passwords[name] = pw
	}
	apiErr(t, "identity t-cons", h.createIdentity(t, h.sysopToken, "t-cons"+h.sfx, "pw"), 400)

	resp, err := auth.Do(ctx(t), transport.Request{Method: http.MethodPost, Path: identity.LoginPath,
		Body: models.LoginRequest{SystemName: cons, Credentials: models.Credentials{Password: h.passwords[cons]}}})
	if err != nil || resp.StatusCode != 201 {
		t.Fatalf("raw login: %v", err)
	}
	exactKeys(t, "C1", wire(t, resp.Body), "token", "systemName", "expirationTime", "sysop")

	login, err := id.Login(ctx(t), cons, h.passwords[cons])
	if err != nil || login.Sysop || login.SystemName != cons {
		t.Fatalf("login %s: %+v %v", cons, login, err)
	}
	h.tokens[cons] = login.Token
	apiErr(t, "identities with non-sysop token", h.createIdentity(t, login.Token, "TNever"+h.sfx, "pw"), 403)

	_, err = id.Login(ctx(t), cons, "wrong")
	envelope(t, "wrong password", apiErr(t, "wrong password", err, 401), models.ExceptionAuth)
	_, err = auth.Do(ctx(t), transport.Request{Method: http.MethodPost, Path: identity.LoginPath,
		Body: map[string]any{"systemName": cons, "credentials": "plain-string"}})
	apiErr(t, "credentials as string", err, 400)

	for _, name := range []string{"TProv" + h.sfx, "TOther" + h.sfx, "TPub" + h.sfx} {
		l, err := id.Login(ctx(t), name, h.passwords[name])
		if err != nil {
			t.Fatalf("login %s: %v", name, err)
		}
		h.tokens[name] = l.Token
	}

	h.runExample(t, "identity", "-url", env(t, "AUTH_URL"),
		"-cert", filepath.Join(h.certDir, "TestHarnessExample.crt"), "-key", filepath.Join(h.certDir, "TestHarnessExample.key"),
		"-ca", filepath.Join(h.certDir, "ca.crt"), "-system", "Sysop", "-password", env(t, "SYSOP_PASSWORD"))
}
