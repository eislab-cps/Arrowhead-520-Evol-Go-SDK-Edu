// Command auth grants one consumer access to a provider's service in
// ConsumerAuthorization, looks the policy up, and revokes it again unless
// -keep is given.
//
//	go run ./examples/auth -cert certs/TestHarness.crt -key certs/TestHarness.key -ca certs/ca.crt \
//	    -provider TemperatureProvider -service temperatureService -consumer ConsumerApp
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/auth"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/models"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/transport"
)

func main() {
	url := flag.String("url", "https://localhost:8492", "ConsumerAuthorization base URL (TLS, server name consumerauth)")
	certFile := flag.String("cert", "", "client certificate PEM (required)")
	keyFile := flag.String("key", "", "client private key PEM (required)")
	caFile := flag.String("ca", "", "CA certificate PEM (required)")
	provider := flag.String("provider", "", "provider system name (required)")
	service := flag.String("service", "", "service definition name (required)")
	consumer := flag.String("consumer", "", "consumer system name to allow (required)")
	keep := flag.Bool("keep", false, "leave the policy in place")
	flag.Parse()
	if err := run(*url, *certFile, *keyFile, *caFile, *provider, *service, *consumer, *keep); err != nil {
		fmt.Fprintln(os.Stderr, "auth example:", err)
		os.Exit(1)
	}
}

func run(url, certFile, keyFile, caFile, provider, service, consumer string, keep bool) error {
	creds, err := transport.LoadCredentials(certFile, keyFile, caFile)
	if err != nil {
		return err
	}
	t, err := transport.NewMTLS(url, "consumerauth", creds)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	a := auth.New(t)

	policy, err := a.Grant(ctx, auth.AllowConsumers(provider, service, consumer))
	if err != nil {
		return fmt.Errorf("C10 grant: %w", err)
	}
	fmt.Printf("C10 granted %s: %s %v\n", policy.InstanceID, policy.DefaultPolicy.PolicyType, policy.DefaultPolicy.PolicyList)

	found, err := a.Lookup(ctx, models.PolicyLookup{InstanceIDs: []string{policy.InstanceID}})
	if err != nil {
		return fmt.Errorf("C12 lookup: %w", err)
	}
	fmt.Printf("C12 lookup: %d policy(ies)\n", found.Count)

	if keep {
		return nil
	}
	if err := a.Revoke(ctx, policy.InstanceID); err != nil {
		return fmt.Errorf("C11 revoke: %w", err)
	}
	fmt.Printf("C11 revoked %s\n", policy.InstanceID)
	return nil
}
