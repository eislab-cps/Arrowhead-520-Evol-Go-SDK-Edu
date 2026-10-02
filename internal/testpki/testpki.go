// Package testpki is an in-memory certificate authority for unit tests. It
// issues EC certificates shaped like profile-ca's: CN and the only DNS name are
// the system name, OU is the profile ("on", "de", "sy").
package testpki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"sync"
	"testing"
	"time"
)

// CA is a test certificate authority.
type CA struct {
	Cert *x509.Certificate
	PEM  []byte

	key    *ecdsa.PrivateKey
	mu     sync.Mutex
	serial int64
}

// New creates a CA. extraDNS are added to the CA certificate itself, which
// lets a test serve with the CA certificate as profile-ca does.
func New(t testing.TB, extraDNS ...string) *CA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test Local Cloud CA", OrganizationalUnit: []string{"lo"}},
		DNSNames:              extraDNS,
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return &CA{Cert: cert, PEM: Block("CERTIFICATE", der), key: key, serial: 1}
}

// Issue returns a PEM certificate and PEM "EC PRIVATE KEY" for name, with OU
// set to ou. The certificate is valid for both client and server use.
func (c *CA) Issue(t testing.TB, name, ou string) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	c.serial++
	serial := c.serial
	c.mu.Unlock()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: name, OrganizationalUnit: []string{ou}},
		DNSNames:     []string{name},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.Cert, &key.PublicKey, c.key)
	if err != nil {
		t.Fatal(err)
	}
	kder, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return Block("CERTIFICATE", der), Block("EC PRIVATE KEY", kder)
}

// ServerTLS returns a server config for name that requires and verifies a
// client certificate from this CA, as the foundation systems and profile-ca do.
func (c *CA) ServerTLS(t testing.TB, name string) *tls.Config {
	t.Helper()
	certPEM, keyPEM := c.Issue(t, name, "sy")
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(c.Cert)
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}
}

// Block PEM-encodes der.
func Block(typ string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der})
}
