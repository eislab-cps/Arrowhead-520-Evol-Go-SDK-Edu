// Command ca walks profile-ca's certificate chain for one system name:
// CA certificate, then onboarding (on), device (de) and system (sy)
// certificates, each requested explicitly. It writes ca.crt, <name>.crt and
// <name>.key (the "sy" certificate) to -out.
//
//	go run ./examples/ca -name TemperatureProvider -out ./certs
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/ca"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/models"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/transport"
)

func main() {
	plainURL := flag.String("plain", "http://localhost:8787", "profile-ca plain HTTP base URL (info, onboarding)")
	tlsURL := flag.String("tls", "https://localhost:8788", "profile-ca mTLS base URL (device, system)")
	serverName := flag.String("server-name", "localhost", "DNS name in profile-ca's server certificate")
	name := flag.String("name", "", "system name, PascalCase (required)")
	out := flag.String("out", ".", "directory for ca.crt, <name>.crt and <name>.key")
	flag.Parse()
	if err := run(*plainURL, *tlsURL, *serverName, *name, *out); err != nil {
		fmt.Fprintln(os.Stderr, "ca example:", err)
		os.Exit(1)
	}
}

func run(plainURL, tlsURL, serverName, name, out string) error {
	if err := models.ValidateSystemName(name); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	plain, err := transport.NewPlain(plainURL)
	if err != nil {
		return err
	}
	pc := ca.New(plain)

	info, err := pc.Info(ctx)
	if err != nil {
		return fmt.Errorf("C13 CA info: %w", err)
	}
	fmt.Printf("C13 CA certificate: %s\n", info.CommonName)

	// Step 1: onboarding certificate over plain HTTP, no client certificate.
	on, err := pc.RequestCertificate(ctx, models.ProfileOnboarding, name)
	if err != nil {
		return fmt.Errorf("C14 onboarding: %w", err)
	}
	fmt.Printf("C14 issued profile=%s for %s\n", on.Profile, on.SystemName)

	// Step 2: device certificate over mTLS, presenting the onboarding certificate.
	de, err := step(ctx, tlsURL, serverName, on, info.Certificate, models.ProfileDevice, name)
	if err != nil {
		return fmt.Errorf("C15 device: %w", err)
	}
	fmt.Printf("C15 issued profile=%s for %s\n", de.Profile, de.SystemName)

	// Step 3: system certificate over mTLS, presenting the device certificate.
	sy, err := step(ctx, tlsURL, serverName, de, info.Certificate, models.ProfileSystem, name)
	if err != nil {
		return fmt.Errorf("C16 system: %w", err)
	}
	fmt.Printf("C16 issued profile=%s for %s\n", sy.Profile, sy.SystemName)

	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "ca.crt"), []byte(info.Certificate), 0o644); err != nil {
		return err
	}
	certFile, keyFile := filepath.Join(out, name+".crt"), filepath.Join(out, name+".key")
	if err := ca.Save(sy, certFile, keyFile); err != nil {
		return err
	}
	fmt.Printf("wrote %s, %s, %s\n", filepath.Join(out, "ca.crt"), certFile, keyFile)
	return nil
}

// step builds an mTLS client that presents presented and requests profile.
func step(ctx context.Context, tlsURL, serverName string, presented models.IssuedCertificate, caPEM, profile, name string) (models.IssuedCertificate, error) {
	creds, err := ca.Credentials(presented, caPEM)
	if err != nil {
		return models.IssuedCertificate{}, err
	}
	client, err := transport.NewMTLS(tlsURL, serverName, creds)
	if err != nil {
		return models.IssuedCertificate{}, err
	}
	return ca.New(client).RequestCertificate(ctx, profile, name)
}
