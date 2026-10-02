package transport

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

// Credentials is a client certificate with its private key, plus the CA roots
// used to verify servers.
type Credentials struct {
	Certificate tls.Certificate
	Roots       *x509.CertPool
	// Leaf is the parsed client certificate, for inspection (CN, OU, expiry).
	Leaf *x509.Certificate
}

// LoadCredentials reads a PEM client certificate, its PEM private key and a
// PEM CA bundle from files. It is the one call that loads a certificate.
func LoadCredentials(certFile, keyFile, caFile string) (Credentials, error) {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return Credentials{}, fmt.Errorf("transport: read certificate: %w", err)
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return Credentials{}, fmt.Errorf("transport: read private key: %w", err)
	}
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return Credentials{}, fmt.Errorf("transport: read CA bundle: %w", err)
	}
	return CredentialsFromPEM(certPEM, keyPEM, caPEM)
}

// CredentialsFromPEM builds Credentials from PEM data already in memory, such
// as the certificate, private key and CA certificate returned by profile-ca.
func CredentialsFromPEM(certPEM, keyPEM, caPEM []byte) (Credentials, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return Credentials{}, fmt.Errorf("transport: certificate and key: %w", err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return Credentials{}, fmt.Errorf("transport: parse certificate: %w", err)
	}
	cert.Leaf = leaf
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return Credentials{}, errors.New("transport: CA bundle contains no PEM certificate")
	}
	return Credentials{Certificate: cert, Roots: roots, Leaf: leaf}, nil
}
