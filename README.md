# Arrowhead-520-Evol-Go-SDK-Edu

Educational Go client library for the Arrowhead 5.2 core systems as implemented in
[Arrowhead-520-Go-Evol](https://github.com/ulfbod/Arrowhead-520-Go-Evol).

It removes HTTP, JSON and TLS boilerplate while keeping every Arrowhead concept
explicit: registration, discovery, orchestration, authorization, identity,
certificates and MQTT events. It is not a production SDK, not a runtime framework,
and it never auto-registers, auto-discovers or auto-authorizes anything.

**Status:** `v0.1.1`, pinned to Arrowhead-520-Go-Evol `v0.1.3` (see `CONTRACT.md`). Every
module is covered by a contract test against that stack version, run on the published
images. Compatibility: v0.1.1 changes only tests, CI and documentation; the package API
is identical to v0.1.0, and v0.1.0 passes the same contract test against stack v0.1.3. Module path:
`github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu`.

## Quick start

```bash
go get github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu@v0.1.1
```

Every step is a call you make yourself. A provider gets a certificate, logs in,
and registers; nothing happens in the background:

```go
// Trust anchor and a system certificate from profile-ca (see examples/ca).
creds, _ := transport.LoadCredentials("certs/TemperatureProvider.crt",
    "certs/TemperatureProvider.key", "certs/ca.crt")

// mTLS clients name the service in the server certificate explicitly.
authT, _ := transport.NewMTLS("https://localhost:8491", "authentication", creds)
srT, _ := transport.NewMTLS("https://localhost:8490", "serviceregistry", creds)

login, _ := identity.New(authT).Login(ctx, "TemperatureProvider", password)
inst, created, err := registry.New(srT).Register(ctx, login.Token, models.ServiceRegistration{
    SystemName:            "TemperatureProvider",
    ServiceDefinitionName: "temperatureService",
    Version:               "1.0.0",
    Interfaces: []models.Interface{
        registry.HTTPInterface("generic_http", "10.0.0.5", 9000, "/temperature"),
    },
})
```

A consumer pulls from the orchestrator, which speaks plain HTTP; the result is
empty until ConsumerAuthorization allows that consumer (`auth.Grant`):

```go
orchT, _ := transport.NewPlain("http://localhost:8083")
res, err := orchestration.New(orchT).Pull(ctx, orchestration.PullRequest("ConsumerApp", "temperatureService"))
```

One runnable program per module is in `examples/` (`ca`, `identity`, `registry`,
`auth`, `orchestration`, `events`).

## Development

```bash
go vet ./...      # tier 1
go test -race ./...   # tier 2
# tier 3: contract test against the pinned stack (needs Docker and the stack source tree)
GOEVOL_DIR=/path/to/Arrowhead-520-Go-Evol STACK_REV=$(git -C /path/to/Arrowhead-520-Go-Evol rev-parse v0.1.3^{commit}) \
    bash test/contract/run.sh
```

## Documentation

| File | What it covers |
|---|---|
| `ARCHITECTURE.md` | Module layout and design principles |
| `CONTRACT.md` | Pinned Arrowhead-520-Go-Evol tag and the endpoints depended on |
| `examples/` | One runnable program per module |
| `test/contract/` | Tier-3 contract test: builds the stack images, runs every `CONTRACT.md` row |

## License

MIT, see `LICENSE`.
