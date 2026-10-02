// Command registry registers one HTTP service instance for a provider, looks
// it up, and revokes it again unless -keep is given. The provider logs in first
// (identity.Login) because registration requires its own token.
//
//	go run ./examples/registry -cert certs/TestHarness.crt -key certs/TestHarness.key -ca certs/ca.crt \
//	    -system TemperatureProvider -password secret -service temperatureService \
//	    -address 10.0.0.5 -port 9000 -base-path /temperature
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/identity"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/models"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/registry"
	"github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu/transport"
)

type config struct {
	srURL, authURL, certFile, keyFile, caFile string
	system, password, service, address        string
	port                                      int
	basePath                                  string
	keep                                      bool
}

func main() {
	var c config
	flag.StringVar(&c.srURL, "sr-url", "https://localhost:8490", "ServiceRegistry base URL (TLS, server name serviceregistry)")
	flag.StringVar(&c.authURL, "auth-url", "https://localhost:8491", "Authentication base URL (TLS, server name authentication)")
	flag.StringVar(&c.certFile, "cert", "", "client certificate PEM (required)")
	flag.StringVar(&c.keyFile, "key", "", "client private key PEM (required)")
	flag.StringVar(&c.caFile, "ca", "", "CA certificate PEM (required)")
	flag.StringVar(&c.system, "system", "", "provider system name, PascalCase (required)")
	flag.StringVar(&c.password, "password", "", "provider password")
	flag.StringVar(&c.service, "service", "", "service definition name, camelCase (required)")
	flag.StringVar(&c.address, "address", "127.0.0.1", "address consumers use to reach the provider")
	flag.IntVar(&c.port, "port", 9000, "port consumers use to reach the provider")
	flag.StringVar(&c.basePath, "base-path", "/", "path of the service at the provider")
	flag.BoolVar(&c.keep, "keep", false, "leave the instance registered")
	flag.Parse()
	if err := run(c); err != nil {
		fmt.Fprintln(os.Stderr, "registry example:", err)
		os.Exit(1)
	}
}

func run(c config) error {
	if err := models.ValidateSystemName(c.system); err != nil {
		return err
	}
	if err := models.ValidateServiceDefinitionName(c.service); err != nil {
		return err
	}
	creds, err := transport.LoadCredentials(c.certFile, c.keyFile, c.caFile)
	if err != nil {
		return err
	}
	authT, err := transport.NewMTLS(c.authURL, "authentication", creds)
	if err != nil {
		return err
	}
	srT, err := transport.NewMTLS(c.srURL, "serviceregistry", creds)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	login, err := identity.New(authT).Login(ctx, c.system, c.password)
	if err != nil {
		return fmt.Errorf("C1 login: %w", err)
	}
	fmt.Printf("C1 logged in as %s\n", login.SystemName)

	sr := registry.New(srT)
	inst, created, err := sr.Register(ctx, login.Token, models.ServiceRegistration{
		SystemName:            c.system,
		ServiceDefinitionName: c.service,
		Version:               "1.0.0",
		Interfaces:            []models.Interface{registry.HTTPInterface("generic_http", c.address, c.port, c.basePath)},
	})
	if err != nil {
		return fmt.Errorf("C2 register: %w", err)
	}
	fmt.Printf("C2 registered instance %s (created=%v)\n", inst.InstanceID, created)

	found, err := sr.Lookup(ctx, models.ServiceLookup{ServiceDefinitionNames: []string{c.service}})
	if err != nil {
		return fmt.Errorf("C3 lookup: %w", err)
	}
	fmt.Printf("C3 lookup %s: %d instance(s)\n", c.service, found.Count)

	if c.keep {
		return nil
	}
	removed, err := sr.Revoke(ctx, inst.InstanceID)
	if err != nil {
		return fmt.Errorf("C4 revoke: %w", err)
	}
	fmt.Printf("C4 revoked %s (removed=%v)\n", inst.InstanceID, removed)
	return nil
}
