package models

// Certificate profiles issued by profile-ca, in chain order (C14–C16).
const (
	ProfileOnboarding = "on" // C14, no client certificate needed
	ProfileDevice     = "de" // C15, requires an "on" client certificate
	ProfileSystem     = "sy" // C16, requires a "de" client certificate
)

// CAInfo is the 200 body of C13, GET /ca/info.
type CAInfo struct {
	CommonName  string `json:"commonName"`
	Certificate string `json:"certificate"`
}

// CertificateRequest is the body of C14, C15 and C16.
type CertificateRequest struct {
	SystemName string `json:"systemName"`
}

// IssuedCertificate is the 201 body of C14, C15 and C16. profile-ca generates
// the key pair; PrivateKey is a PEM "EC PRIVATE KEY" block.
type IssuedCertificate struct {
	SystemName  string `json:"systemName"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"privateKey"`
	Profile     string `json:"profile"`
	IssuedAt    string `json:"issuedAt"`
}
