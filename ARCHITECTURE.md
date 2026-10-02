# ARCHITECTURE.md — Arrowhead-520-Evol-Go-SDK-Edu

## System overview

A thin, typed Go client library over the Arrowhead 5.2 core systems in
Arrowhead-520-Go-Evol. One package per Arrowhead concept. No runtime, no node
abstraction, no automatic anything. Every operation returns a value and an error.

## Directory tree

```
Arrowhead-520-Evol-Go-SDK-Edu/
├── identity/       login, token handling (Authentication system)
├── registry/       AH5 service-discovery register / lookup / revoke
├── orchestration/  pull, subscribe, push trigger, history
├── auth/           ConsumerAuthorization grant / revoke / lookup
├── ca/             profile-ca certificate requests (profile is a visible field)
├── events/         MQTT client only; MQTT-INSECURE-JSON interface
├── transport/      HTTP client: explicit mTLS or plain HTTP, explicit server name, timeouts, no retries
├── models/         typed request/response structs (own definitions; Go-Evol types are internal)
├── examples/       one runnable example per package, used by the contract test
└── CONTRACT.md     pinned stack tag and endpoints
```

## Technology choices

| Concern | Choice | Reason |
|---|---|---|
| Language | Go | Matches the stack and the course gateway |
| Module path | `github.com/eislab-cps/Arrowhead-520-Evol-Go-SDK-Edu` | Same in private and public copies (REPO_SETUP rule 4) |
| Types | Own `models/` | Go-Evol module paths are not fetchable and all packages are `internal/` |

## Design principles

- Explicit over implicit; no magic; strong typing.
- Events = MQTT only. Push orchestration stays in `orchestration/`. There is no
  Event Handler system in Go-Evol.
- Identity is a first-class module: registration and management calls need tokens.
- TLS is explicit but bootstrapped: the foundation systems require client
  certificates on their exposed ports, so `transport/` loads a certificate in one
  call (`LoadCredentials` or `CredentialsFromPEM`) without hiding that it happens.
- Two kinds of client, chosen by the caller: `transport.NewMTLS` (foundation systems,
  profile-ca mTLS port; needs an explicit server name because the foundation server
  certificates carry only the service name) and `transport.NewPlain` (orchestrator and
  profile-ca plain port; nothing is authenticated).
- Each request is sent once: no retries, no redirects, no proxy from the environment.

## Design decisions

The key design decisions are summarized below.

| ID | Decision | Summary |
|---|---|---|
| D1 | Own models, pinned tag | SDK cannot import Go-Evol; drift controlled by CONTRACT.md + tier 3 |
| D2 | consumerauth mode only | Course-scope; XACML mode not abstracted |
| D3 | Events = MQTT only | Avoids hiding the transport (SDK principle "no magic") |
