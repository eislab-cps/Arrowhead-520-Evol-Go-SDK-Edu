// Package ca requests certificates from profile-ca, the Arrowhead local cloud
// certificate authority (CONTRACT.md C13–C16).
//
// Certificates are requested by system name and profile. profile-ca generates
// the key pair and returns the private key in the response. The chain has
// three steps, each an explicit call with the profile visible:
//
//	on  POST /bootstrap/onboarding-cert  plain HTTP, no client certificate
//	de  POST /ca/device-cert             mTLS, presenting the "on" certificate
//	sy  POST /ca/system-cert             mTLS, presenting the "de" certificate
//
// Each step needs its own transport client, because each presents a different
// certificate. Nothing here stores a certificate or builds a client for you;
// use Credentials to turn a result into transport.Credentials, and Save to
// write it to disk.
package ca

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/models"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/transport"
)

// Endpoints of profile-ca used by this package.
const (
	InfoPath       = "/ca/info"
	OnboardingPath = "/bootstrap/onboarding-cert"
	DevicePath     = "/ca/device-cert"
	SystemPath     = "/ca/system-cert"
)

// Client talks to profile-ca through one transport client.
type Client struct {
	t *transport.Client
}

// New returns a ca client that sends its requests through t. Use a
// transport.NewPlain client (port 8787) for Info and the "on" profile, and a
// transport.NewMTLS client (port 8788, server name "profile-ca" or
// "localhost") for the "de" and "sy" profiles.
func New(t *transport.Client) *Client {
	return &Client{t: t}
}

// Info returns the CA's own certificate (C13). Its PEM is the trust anchor
// for every mTLS client.
func (c *Client) Info(ctx context.Context) (models.CAInfo, error) {
	resp, err := c.t.Do(ctx, transport.Request{Method: http.MethodGet, Path: InfoPath})
	if err != nil {
		return models.CAInfo{}, err
	}
	var out models.CAInfo
	if err := resp.DecodeJSON(&out); err != nil {
		return models.CAInfo{}, err
	}
	return out, nil
}

// RequestCertificate asks profile-ca for a certificate with the given profile
// (models.ProfileOnboarding, ProfileDevice or ProfileSystem) for systemName
// (C14, C15, C16).
//
// The client must match the profile: plain HTTP for "on", mutual TLS for "de"
// (presenting an "on" certificate) and "sy" (presenting a "de" certificate).
// profile-ca answers 403 when the presented certificate has the wrong profile.
// A result whose profile differs from the one requested is returned together
// with an error.
func (c *Client) RequestCertificate(ctx context.Context, profile, systemName string) (models.IssuedCertificate, error) {
	path, wantMode, err := route(profile)
	if err != nil {
		return models.IssuedCertificate{}, err
	}
	if c.t.Mode() != wantMode {
		return models.IssuedCertificate{}, fmt.Errorf("ca: profile %q needs a %s client, this one is %s", profile, wantMode, c.t.Mode())
	}
	if systemName == "" {
		return models.IssuedCertificate{}, errors.New("ca: system name is empty")
	}
	resp, err := c.t.Do(ctx, transport.Request{
		Method: http.MethodPost,
		Path:   path,
		Body:   models.CertificateRequest{SystemName: systemName},
	})
	if err != nil {
		return models.IssuedCertificate{}, err
	}
	var out models.IssuedCertificate
	if err := resp.DecodeJSON(&out); err != nil {
		return models.IssuedCertificate{}, err
	}
	if out.Profile != profile {
		return out, fmt.Errorf("ca: requested profile %q, profile-ca issued %q", profile, out.Profile)
	}
	return out, nil
}

func route(profile string) (path string, mode transport.Mode, err error) {
	switch profile {
	case models.ProfileOnboarding:
		return OnboardingPath, transport.PlainHTTP, nil
	case models.ProfileDevice:
		return DevicePath, transport.MutualTLS, nil
	case models.ProfileSystem:
		return SystemPath, transport.MutualTLS, nil
	}
	return "", 0, fmt.Errorf("ca: unknown profile %q (want %q, %q or %q)",
		profile, models.ProfileOnboarding, models.ProfileDevice, models.ProfileSystem)
}

// Credentials turns an issued certificate and the CA certificate PEM (from
// Info) into transport.Credentials for transport.NewMTLS.
func Credentials(issued models.IssuedCertificate, caPEM string) (transport.Credentials, error) {
	return transport.CredentialsFromPEM([]byte(issued.Certificate), []byte(issued.PrivateKey), []byte(caPEM))
}

// Save writes the certificate and private key PEM to the given files. The key
// file is created with mode 0600. Existing files are overwritten.
func Save(issued models.IssuedCertificate, certFile, keyFile string) error {
	if err := os.WriteFile(certFile, []byte(issued.Certificate), 0o644); err != nil {
		return fmt.Errorf("ca: write certificate: %w", err)
	}
	if err := os.WriteFile(keyFile, []byte(issued.PrivateKey), 0o600); err != nil {
		return fmt.Errorf("ca: write private key: %w", err)
	}
	return nil
}
