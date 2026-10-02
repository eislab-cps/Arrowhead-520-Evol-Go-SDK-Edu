// Command identity logs a system in to the Authentication system over mTLS
// and prints what the token is for. The token itself is not printed in full.
//
//	go run ./examples/identity -cert certs/TestHarness.crt -key certs/TestHarness.key \
//	    -ca certs/ca.crt -system Sysop -password "$SYSOP_PASSWORD"
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/identity"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/transport"
)

func main() {
	url := flag.String("url", "https://localhost:8491", "Authentication base URL (TLS)")
	serverName := flag.String("server-name", "authentication", "DNS name in Authentication's server certificate")
	certFile := flag.String("cert", "", "client certificate PEM (required)")
	keyFile := flag.String("key", "", "client private key PEM (required)")
	caFile := flag.String("ca", "", "CA certificate PEM (required)")
	system := flag.String("system", "", "system name to log in as (required)")
	password := flag.String("password", "", "password of that system")
	flag.Parse()
	if err := run(*url, *serverName, *certFile, *keyFile, *caFile, *system, *password); err != nil {
		fmt.Fprintln(os.Stderr, "identity example:", err)
		os.Exit(1)
	}
}

func run(url, serverName, certFile, keyFile, caFile, system, password string) error {
	creds, err := transport.LoadCredentials(certFile, keyFile, caFile)
	if err != nil {
		return err
	}
	client, err := transport.NewMTLS(url, serverName, creds)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	login, err := identity.New(client).Login(ctx, system, password)
	if err != nil {
		return fmt.Errorf("C1 login: %w", err)
	}
	shown := login.Token
	if len(shown) > 8 {
		shown = shown[:8] + "…"
	}
	fmt.Printf("C1 logged in as %s (sysop=%v), token %s expires %s\n",
		login.SystemName, login.Sysop, shown, login.ExpirationTime)
	return nil
}
