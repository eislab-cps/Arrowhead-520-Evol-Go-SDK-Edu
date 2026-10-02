// Command orchestration pulls providers for a service as a consumer and prints
// them. An empty result means no provider is registered or ConsumerAuthorization
// allows none for this consumer. With -sysop-password it also logs in as Sysop
// and prints the orchestration history.
//
//	go run ./examples/orchestration -consumer ConsumerApp -service temperatureService
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/identity"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/orchestration"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/transport"
)

func main() {
	orchURL := flag.String("url", "http://localhost:8083", "orchestrator base URL (plain HTTP)")
	consumer := flag.String("consumer", "", "requester system name (asserted, not authenticated)")
	service := flag.String("service", "", "service definition name")
	authURL := flag.String("auth-url", "https://localhost:8491", "Authentication base URL, for -sysop-password")
	certFile := flag.String("cert", "", "client certificate PEM, for -sysop-password")
	keyFile := flag.String("key", "", "client private key PEM, for -sysop-password")
	caFile := flag.String("ca", "", "CA certificate PEM, for -sysop-password")
	sysopPassword := flag.String("sysop-password", "", "if set, log in as Sysop and print the history")
	flag.Parse()
	if err := run(*orchURL, *consumer, *service, *authURL, *certFile, *keyFile, *caFile, *sysopPassword); err != nil {
		fmt.Fprintln(os.Stderr, "orchestration example:", err)
		os.Exit(1)
	}
}

func run(orchURL, consumer, service, authURL, certFile, keyFile, caFile, sysopPassword string) error {
	t, err := transport.NewPlain(orchURL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	orch := orchestration.New(t)

	res, err := orch.Pull(ctx, orchestration.PullRequest(consumer, service))
	if err != nil {
		return fmt.Errorf("C5 pull: %w", err)
	}
	fmt.Printf("C5 pull %s as %s: %d provider(s)\n", service, consumer, len(res.Response))
	for _, r := range res.Response {
		fmt.Printf("    %s at %s:%d%s (version %d)\n", r.Provider.SystemName, r.Provider.Address, r.Provider.Port, r.Service.ServiceURI, r.Service.Version)
	}

	if sysopPassword == "" {
		return nil
	}
	creds, err := transport.LoadCredentials(certFile, keyFile, caFile)
	if err != nil {
		return err
	}
	authT, err := transport.NewMTLS(authURL, "authentication", creds)
	if err != nil {
		return err
	}
	sysop, err := identity.New(authT).Login(ctx, "Sysop", sysopPassword)
	if err != nil {
		return fmt.Errorf("C1 login as Sysop: %w", err)
	}
	hist, err := orch.History(ctx, sysop.Token)
	if err != nil {
		return fmt.Errorf("C9 history: %w", err)
	}
	fmt.Printf("C9 history: %d entries\n", hist.Count)
	return nil
}
