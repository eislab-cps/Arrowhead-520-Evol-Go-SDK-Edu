# CONTRACT.md — Arrowhead-520-Evol-Go-SDK-Edu

The stack this SDK is written and tested against, and the exact surface it depends on.
This file is the source of truth for the SDK; code follows it.

## Pinned stack

| Item | Value |
|---|---|
| Stack repo | `github.com/ulfbod/Arrowhead-520-Go-Evol` |
| Tag | `v0.1.3` |
| SPECs cited | `foundation/SPEC.md` (F), `core/SPEC.md` (C), `services/profile-ca/SPEC.md` (P), at the tag |
| Compose used by the contract test | `deploy/docker-compose.consumerauth.yml` at the tag (six services), with the token settings below |
| Token settings assumed | `REGISTER_AUTH_URL` on ServiceRegistry; `MGMT_AUTH_URL` on ServiceRegistry, Authentication and ConsumerAuthorization; `SYSOP_PASSWORD` on Authentication; `MGMT_AUTH_URL` on dynamicorch-xacml. `LOOKUP_AUTH_URL` and `SERVICE_DISCOVERY_POLICY` unset. |
| Images | `ghcr.io/ulfbod/<name>:v0.1.3` for `serviceregistry`, `authentication`, `consumerauth`, `dynamicorch-xacml`, `profile-ca`, `cert-provisioner` |
| Orchestrator mode assumed | `AUTH_BACKEND=consumerauth` (XACML/PDP mode is out of scope for the SDK) |

Citations name the SPEC section heading. Where a SPEC leaves something out that the SDK
depends on, the row cites the code file at the tag as well; the SDK follows the running
code.

## Conventions that apply to every row

**Transports.** Three kinds, and the SDK caller always chooses which one a client uses:

| Target | Scheme | Host port (stack compose) | Client certificate |
|---|---|---|---|
| ServiceRegistry, Authentication, ConsumerAuthorization | `https` | 8490, 8491, 8492 | required: the server uses `RequireAndVerifyClientCert` against the profile-ca root (`foundation/internal/tlsutil/tlsutil.go`). Any profile-ca certificate is accepted; the OU is not checked. |
| profile-ca mTLS port | `https` | 8788 | required: `RequireAndVerifyClientCert` (`services/profile-ca/main.go`); the handler then checks the OU |
| profile-ca plain port | `http` | 8787 | none |
| dynamicorch-xacml | `http` | 8083 | none. It has no TLS listener at this tag. The requester is **asserted in the request body, not authenticated**: any caller can claim any `requesterSystem.systemName`. |

**Server names.** The foundation server certificates are issued by cert-provisioner with
the service name as their only DNS name (`serviceregistry`, `authentication`,
`consumerauth`). A client that connects to `localhost:8490` must set the TLS server name
to the service name explicitly. profile-ca serves its CA root as the TLS server
certificate, with DNS names `profile-ca` and `localhost` (`services/profile-ca/ca.go`
`NewProfileCA`).

**Trust anchor.** The profile-ca root is fetched with C13 (`GET /ca/info`) over plain HTTP.
The SDK never fetches it implicitly.

**Tokens.** The SDK is specified against a deployment with the token settings in
"Pinned stack" (the course kit sets them; the stack's own consumerauth compose leaves
them unset, and then every row below is open). A token is the `token` from C1, sent as
`Authorization: Bearer <token>`. The SDK takes a token as an explicit argument and never
attaches, caches or refreshes one on its own. What each setting guards, from the code:

| Setting | Guards | Failure codes | Source |
|---|---|---|---|
| `REGISTER_AUTH_URL` (SR) | C2 register: the token's `systemName` must equal the body's `systemName` | `401` no token, an invalid or expired token (verify returns `verified: false`), or Authentication unreachable; `403` a verified token for another system | F §1 configuration (`REGISTER_AUTH_URL`) |
| `MGMT_AUTH_URL` (SR, Authentication, ConsumerAuthorization) | only paths under `/mgmt/`; no SDK row is one. It guards the operator fixture `POST /authentication/mgmt/identities` (see C1). | `401` no token, an invalid or expired token (verify returns `verified: false`), or Authentication unreachable; `403` a verified token that is not sysop | F §1, §2, §3 configuration (`MGMT_AUTH_URL`) |
| `MGMT_AUTH_URL` (dynamicorch-xacml) | C8, C9 (paths under `/mgmt/`); not C5, C6, C7 | `401` no token, an invalid or expired token (verify returns `verified: false`), or Authentication unreachable; `403` a verified token that is not sysop. The body is the JSON envelope with `origin` `dynamicorch-xacml` (the orchestrator's other errors are plain text) | C "Management access" |

Not guarded by any setting at the tag, so any client holding a profile-ca certificate may
call them: C3 lookup (open while `LOOKUP_AUTH_URL` is unset), C4 revoke of any instance
(`foundation/internal/api/ah5_handler.go` `handleServiceRevoke` has no token check), and
C10–C12 grant, revoke and lookup of policies (`foundation/internal/consumerauth/api/handler.go`
`handleGrant`, `handleRevoke`, `handleLookup` have no token check; they are not under
`/mgmt/`, so `MGMT_AUTH_URL` does not reach them).

**AH5 names** (enforced by the ServiceRegistry discovery endpoints,
`foundation/internal/api/validate.go`; not listed in F; violations give `400`):

| Name | Rule | Valid | Rejected |
|---|---|---|---|
| System name | PascalCase `^[A-Z][A-Za-z0-9]{0,62}$` | `TemperatureProvider` | `temp-provider`, `tempProvider` |
| Service definition | camelCase `^[a-z][A-Za-z0-9]{0,62}$` | `temperatureService` | `temperature-service`, `Telemetry` |

Interface `properties` values are strings (`"accessPort": "9000"`, not `9000`). System
`addresses` are objects `{"type": "...", "address": "..."}`, never bare strings.
ConsumerAuthorization and the orchestrator do not validate names, but the names used
there must equal the ones registered, so the SDK documents the same rules for them.

**Error bodies** differ per system:

| System | Error body | Source |
|---|---|---|
| Foundation (SR, Authentication, ConsumerAuthorization) | `{"errorMessage": "...", "errorCode": 400, "exceptionType": "INVALID_PARAMETER", "origin": "..."}`; `exceptionType` follows the status (`400` INVALID_PARAMETER, `401` AUTH_EXCEPTION, `403` FORBIDDEN, `404` DATA_NOT_FOUND, `423` LOCKED, `501` NOT_IMPLEMENTED, other ARROWHEAD_EXCEPTION). `origin` names the system, but is `""` for invalid-JSON `400` and wrong-method `405` errors from the shared helpers. Not an envelope: an unknown path gives plain-text `404 page not found`; a wrong method on `general/mgmt` endpoints (not used by the SDK) gives plain-text `405`. | F "Error Format" |
| profile-ca | `{"error": "..."}` | P; `services/profile-ca/handlers.go` `writeError` |
| dynamicorch-xacml | plain text (`http.Error`), except the `401`/`403` of C8 and C9, which use the JSON envelope above | `core/internal/orchestration/handler.go` `http.Error` calls and `requireMgmtAuth` |

The SDK returns every non-2xx as a Go error that carries the status code and the raw body.
It parses the envelope when the body is one, tolerates a plain-text body, and never
relies on `origin` being non-empty.

## Endpoints depended on

| ID | Module | System | Method and path | Transport | Auth | Success | Errors | Citation |
|---|---|---|---|---|---|---|---|---|
| C1 | identity | Authentication | `POST /authentication/identity/login` | mTLS | none (credentials in body) | 201 | 400, 401 | F §2 "POST /authentication/identity/login" |
| C2 | registry | ServiceRegistry | `POST /serviceregistry/service-discovery/register` | mTLS | Bearer of the registering system (`REGISTER_AUTH_URL`) | 201 new, 200 update | 400; 401 no token, invalid or expired token, or Authentication unreachable; 403 verified token for another system (see Tokens) | F §1a "Service Discovery"; F §1 configuration |
| C3 | registry | ServiceRegistry | `POST /serviceregistry/service-discovery/lookup` | mTLS | none (`LOOKUP_AUTH_URL` unset) | 200 | 400 | F §1a "Service Discovery" |
| C4 | registry | ServiceRegistry | `DELETE /serviceregistry/service-discovery/revoke/{instanceId}` | mTLS | none; no token check in code | 200 removed, 204 not found | 400 | F §1a "Service Discovery" |
| C5 | orchestration | dynamicorch-xacml | `POST /serviceorchestration/orchestration/pull` | http | none (requester asserted) | 200, also when empty | 400, 500 | C "POST /serviceorchestration/orchestration/pull"; C "Authorization backends" |
| C6 | orchestration | dynamicorch-xacml | `POST /serviceorchestration/orchestration/subscribe` | http | none | 201 new, 200 overwrite | 400 | C "POST /serviceorchestration/orchestration/subscribe" |
| C7 | orchestration | dynamicorch-xacml | `DELETE /serviceorchestration/orchestration/unsubscribe/{id}` | http | none | 200 removed, 204 not found | — | C "DELETE /serviceorchestration/orchestration/unsubscribe/{id}" |
| C8 | orchestration | dynamicorch-xacml | `POST /serviceorchestration/orchestration/mgmt/push/trigger` | http | sysop Bearer (`MGMT_AUTH_URL`) | 200 | 400, 404; 401 no token, invalid or expired token, or Authentication unreachable; 403 verified token not sysop (see Tokens) | C "mgmt/push/trigger"; C "Management access" |
| C9 | orchestration | dynamicorch-xacml | `POST /serviceorchestration/orchestration/mgmt/history/query` | http | sysop Bearer (`MGMT_AUTH_URL`) | 200 | 401 no token, invalid or expired token, or Authentication unreachable; 403 verified token not sysop (see Tokens) | C "mgmt/history/query"; C "Management access" |
| C10 | auth | ConsumerAuthorization | `POST /consumerauthorization/authorization/grant` | mTLS | none; not under `MGMT_AUTH_URL` (no token check in code) | 201 | 400, 403 (blacklisted, only with `BLACKLIST_URL`), 409 | F §3 "POST .../authorization/grant", "AuthPolicy" |
| C11 | auth | ConsumerAuthorization | `DELETE /consumerauthorization/authorization/revoke/{instanceId}` | mTLS | none; not under `MGMT_AUTH_URL` (no token check in code) | 200 | 400, 404 | F §3 "DELETE .../authorization/revoke/{instanceId}" |
| C12 | auth | ConsumerAuthorization | `POST /consumerauthorization/authorization/lookup` | mTLS | none; not under `MGMT_AUTH_URL` (no token check in code) | 200 | 400 | F §3 "POST .../authorization/lookup" |
| C13 | ca | profile-ca | `GET /ca/info` | http (8787) | none | 200 | — | P "Plain HTTP Endpoints" `GET /ca/info` |
| C14 | ca | profile-ca | `POST /bootstrap/onboarding-cert` | http (8787) | none | 201 | 400, 500 | P "Plain HTTP Endpoints" `POST /bootstrap/onboarding-cert` |
| C15 | ca | profile-ca | `POST /ca/device-cert` | mTLS (8788), client cert OU=on | onboarding certificate | 201 | 400, 403, 500; TLS handshake failure (no HTTP status) without a certificate from this CA | P "mTLS Endpoints" `POST /ca/device-cert` |
| C16 | ca | profile-ca | `POST /ca/system-cert` | mTLS (8788), client cert OU=de | device certificate | 201 | 400, 403, 500; TLS handshake failure (no HTTP status) without a certificate from this CA | P "mTLS Endpoints" `POST /ca/system-cert` |
| C17 | events | MQTT broker (not a stack system) | publish / subscribe, interface `MQTT-INSECURE-JSON` | `tcp` MQTT, no TLS, no broker auth | none | — | — | F §11 "MQTT Communication Profiles" (interface name); C2 for how an MQTT provider is registered |

## Shapes per row

JSON below is the wire format. `…` marks a value the caller chooses.

### C1 — login

Request:
```json
{ "systemName": "…", "credentials": { "password": "…" } }
```
Response `201`:
```json
{ "token": "…", "systemName": "…", "expirationTime": "2006-01-02T15:04:05Z", "sysop": false }
```
`400`: `credentials` not an object with `password`. `401`: unknown system or wrong password.
Identities are created by an operator through `POST /authentication/mgmt/identities`
(F §2), which the SDK does not wrap. With `MGMT_AUTH_URL` set that call needs a sysop
token: log in (C1) as the built-in `Sysop` identity, which Authentication creates when its
identity store is empty, with the password from `SYSOP_PASSWORD` (default `arrowhead`;
F §2 "Bootstrap"; `foundation/internal/authentication/service/auth.go` `NewAuthServiceFull`).

### C2 — register a service instance

Request (upsert key: `systemName` + `serviceDefinitionName` + `version`):
```json
{
  "systemName": "TemperatureProvider",
  "serviceDefinitionName": "temperatureService",
  "version": "1.0.0",
  "expiresAt": "2027-01-01T00:00:00Z",
  "metadata": { "zone": "B" },
  "interfaces": [
    {
      "templateName": "generic_http",
      "protocol": "http",
      "policy": "NONE",
      "properties": { "accessAddresses": "10.0.0.5", "accessPort": "9000", "basePath": "/temperature" }
    }
  ]
}
```
`policy` is one of `NONE`, `CERT_AUTH`, `TIME_LIMITED_TOKEN_AUTH`, `USAGE_LIMITED_TOKEN_AUTH`,
`BASE64_SELF_CONTAINED_TOKEN_AUTH`, `RSA_SHA256_JSON_WEB_TOKEN_AUTH`,
`RSA_SHA512_JSON_WEB_TOKEN_AUTH`; anything else is `400`. The SDK always sends the
structured interface form, never the flat-string form.

Response `201` (new) or `200` (updated): the stored `AH5ServiceInstance` (F §1a "AH5 Shared
Types"; the SPEC gives the status codes only, the body is from
`foundation/internal/api/ah5_handler.go` `handleServiceRegister`):
```json
{
  "instanceId": "…",
  "provider": { "name": "TemperatureProvider", "createdAt": "…", "updatedAt": "…" },
  "serviceDefinitionName": "temperatureService",
  "version": "1.0.0",
  "expiresAt": "…",
  "metadata": { "zone": "B" },
  "interfaces": [ { "templateName": "generic_http", "protocol": "http", "policy": "NONE", "properties": { "accessPort": "9000" } } ],
  "createdAt": "…",
  "updatedAt": "…"
}
```
`provider.metadata`, `provider.version`, `provider.addresses` and `provider.device` are
omitted when empty (`foundation/internal/model/ah5_types.go` `AH5System`).

Header: `Authorization: Bearer <token>` where the token was issued by C1 to the system
named in `systemName`. The ServiceRegistry resolves the token through Authentication and
compares names before it validates the body.

`401`: no token, an invalid or expired token (verify returns `verified: false`), or
Authentication unreachable.
`403`: a verified token for another system than `systemName`.
`400`: invalid name (see conventions), empty `serviceDefinitionName`, unknown `policy`,
invalid JSON.

The orchestrator (C5) turns properties into its result: `accessAddresses` (first value
if comma-separated) → `provider.address`, `accessPort` → `provider.port`, `basePath` →
`service.serviceUri`, leading integer of `version` → `service.version` (C "ServiceRegistry
lookup"). A provider meant to be orchestrated must set them.

### C3 — lookup

Request (at least one of `instanceIds`, `providerNames`, `serviceDefinitionNames`
non-empty, else `400`):
```json
{
  "instanceIds": [],
  "providerNames": [],
  "serviceDefinitionNames": ["temperatureService"],
  "versions": [],
  "interfaceTemplateNames": []
}
```
Response `200`:
```json
{ "entries": [ /* AH5ServiceInstance, as in C2 */ ], "count": 1, "totalCount": 1 }
```
`totalCount` is present in the code (`ah5_handler.go` `handleServiceLookup`) but not in F.

### C4 — revoke a service instance

`DELETE /serviceregistry/service-discovery/revoke/{instanceId}`, no body.
`200` removed; `204` no such instance (not an error); `400` empty `instanceId`.

### C5 — pull orchestration

Request:
```json
{
  "requesterSystem": { "systemName": "ConsumerApp", "address": "", "port": 0 },
  "requestedService": { "serviceDefinition": "temperatureService", "interfaces": ["generic_http"], "metadata": { "zone": "B" } }
}
```
`requestedService.interfaces` and `metadata` are optional filters.

Response `200`:
```json
{
  "response": [
    {
      "provider": { "systemName": "TemperatureProvider", "address": "10.0.0.5", "port": 9000 },
      "service": {
        "serviceDefinition": "temperatureService",
        "serviceUri": "/temperature",
        "interfaces": ["generic_http"],
        "version": 1,
        "metadata": { "zone": "B" }
      },
      "cloudIdentifier": "LOCAL"
    }
  ]
}
```
No registered provider, or no provider the requester is authorized for, is also `200`
with `{"response": []}`. There is no 404. `service.metadata` is omitted when empty. This
nested shape differs from the AH5 `OrchestrationResult`; the SDK models the nested shape.

Authorization in consumerauth mode: for each candidate provider the orchestrator calls
`POST /consumerauthorization/authorization/verify` with
`{consumer: requesterSystem.systemName, provider, target: serviceDefinition, targetType: "SERVICE_DEF"}`
and keeps the provider only on `true`; any error excludes it (C "Authorization backends").
Candidates come from `POST /serviceregistry/service-discovery/lookup` only, so only C2
registrations are orchestrated (C "ServiceRegistry lookup").

`400`: invalid JSON, missing `requesterSystem.systemName` or
`requestedService.serviceDefinition`. `500`: ServiceRegistry unreachable or erroring.

### C6 — subscribe for push

Request:
```json
{
  "ownerSystemName": "ConsumerApp",
  "targetSystemName": "TemperatureProvider",
  "orchestrationRequest": {
    "requesterSystem": { "systemName": "ConsumerApp" },
    "requestedService": { "serviceDefinition": "temperatureService" }
  },
  "notifyInterface": { "notifyUri": "http://consumer-host:9100/notify" },
  "expiredAt": "2027-01-01T00:00:00Z"
}
```
`notifyInterface` is an object; the delivery URL is its `notifyUri`, else `uri`, else
`http://<address>[:<port>]<path>` from its `address`, `port`, `path` keys.
`expiredAt` is optional, stored and returned, and not enforced.

Response `201` (new) or `200` (same `ownerSystemName` + `targetSystemName` already
subscribed; the stored one is overwritten and keeps its `id`): the subscription.
```json
{
  "id": "…uuid…",
  "ownerSystemName": "ConsumerApp",
  "targetSystemName": "TemperatureProvider",
  "orchestrationRequest": { "…": "…" },
  "notifyInterface": { "notifyUri": "http://consumer-host:9100/notify" },
  "expiredAt": "2027-01-01T00:00:00Z",
  "createdAt": "…"
}
```
`400`: invalid JSON. No field is validated and no authorization check runs at subscribe
or at delivery.

### C7 — unsubscribe

`DELETE /serviceorchestration/orchestration/unsubscribe/{id}`, `id` from C6. `200`
removed; `204` no such subscription.

### C8 — trigger push delivery (management)

Header: `Authorization: Bearer <sysop token>` (C1 as `Sysop`).

Request:
```json
{ "subscriptionId": "…uuid from C6…" }
```
Response `200`:
```json
{ "status": "triggered" }
```
`400`: invalid JSON. `404`: unknown `subscriptionId` (plain-text body). `401`: no
token, an invalid or expired token (verify returns `verified: false`), or Authentication
unreachable. `403`: a verified token that is not sysop. The token is checked before the
body (C "Management access").

Delivery is asynchronous after the `200`: the orchestrator POSTs over plain HTTP to the
subscription's notify URL
```json
{ "subscriptionId": "…", "ownerSystemName": "ConsumerApp", "targetSystemName": "TemperatureProvider" }
```
within `PUSH_DELIVERY_TIMEOUT_SECONDS` (default 5). The notification carries **no provider
list**; the consumer runs C5 after receiving it. A 2xx reply marks the history entry
`DELIVERED`, anything else `FAILED` (C9). The notify URL must be reachable from the
orchestrator container.

### C9 — orchestration history (management)

Header: `Authorization: Bearer <sysop token>`. `401` and `403` as in C8.

Request: the body is ignored; every entry is returned, with no filter and no pagination.
The SDK sends `{}`.

Response `200`:
```json
{
  "entries": [
    {
      "id": "…",
      "status": "DONE",
      "type": "PULL",
      "requesterSystem": "ConsumerApp",
      "serviceDefinition": "temperatureService",
      "createdAt": "…",
      "finishedAt": "…"
    }
  ],
  "count": 1
}
```
`status` is `DONE` or `ERROR` for `PULL`; `PENDING`, `DELIVERED` or `FAILED` for `PUSH`.
`requesterSystem`, `serviceDefinition`, `message` and `finishedAt` are omitted when empty.
An empty pull result is recorded as `DONE`.

### C10 — grant

Request:
```json
{
  "provider": "TemperatureProvider",
  "targetType": "SERVICE_DEF",
  "target": "temperatureService",
  "defaultPolicy": { "policyType": "WHITELIST", "policyList": ["ConsumerApp"] },
  "scopedPolicies": {},
  "description": "optional",
  "createdBy": "optional"
}
```
`policyType`: `ALL`, `WHITELIST`, `BLACKLIST`. `targetType`: `SERVICE_DEF`, `EVENT_TYPE`.
Response `201`: the stored `AuthPolicy`:
```json
{
  "instanceId": "PR|LOCAL|TemperatureProvider|SERVICE_DEF|temperatureService",
  "authorizationLevel": "PR",
  "cloud": "LOCAL",
  "provider": "TemperatureProvider",
  "targetType": "SERVICE_DEF",
  "target": "temperatureService",
  "description": "optional",
  "defaultPolicy": { "policyType": "WHITELIST", "policyList": ["ConsumerApp"] },
  "scopedPolicies": {},
  "createdBy": "…",
  "createdAt": "2006-01-02T15:04:05Z"
}
```
`409`: a policy with that `instanceId` exists (one policy per provider and target).

### C11 — revoke a policy

`DELETE /consumerauthorization/authorization/revoke/{instanceId}` with `|` percent-encoded
as `%7C`. `200` removed; `404` not found; `400` empty or undecodable `instanceId`.

### C12 — lookup policies

Request (at least one of `instanceIds`, `cloudIdentifiers`, `targetNames`, else `400`):
```json
{
  "instanceIds": ["PR|LOCAL|TemperatureProvider|SERVICE_DEF|temperatureService"],
  "cloudIdentifiers": ["LOCAL"],
  "targetNames": ["temperatureService"],
  "targetType": "SERVICE_DEF"
}
```
Response `200`:
```json
{ "policies": [ /* AuthPolicy, as in C10 */ ], "count": 1, "totalCount": 1 }
```

### C13 — CA root certificate

`GET /ca/info`. Response `200`:
```json
{ "commonName": "Arrowhead Local Cloud CA", "certificate": "-----BEGIN CERTIFICATE-----\n…" }
```

### C14, C15, C16 — certificate chain by name and profile

Round one requests certificates by name and profile; profile-ca generates the key pair
and returns the private key. The SDK exposes each step as its own call with the profile
visible. A CSR endpoint is planned as an additive change in a later stack version.

| Step | Row | Port | Client certificate presented | Issued profile (OU) |
|---|---|---|---|---|
| 1 | C14 `POST /bootstrap/onboarding-cert` | 8787 plain | none | `on` |
| 2 | C15 `POST /ca/device-cert` | 8788 mTLS | the `on` certificate from step 1 | `de` |
| 3 | C16 `POST /ca/system-cert` | 8788 mTLS | the `de` certificate from step 2 | `sy` |

Request (all three):
```json
{ "systemName": "TemperatureProvider" }
```
Response `201` (all three; `profile` is `on`, `de` or `sy`):
```json
{
  "systemName": "TemperatureProvider",
  "certificate": "-----BEGIN CERTIFICATE-----\n…",
  "privateKey": "-----BEGIN EC PRIVATE KEY-----\n…",
  "profile": "on",
  "issuedAt": "2026-06-25T00:00:00Z"
}
```
C14–C16 `500`: profile-ca could not persist the new certificate record; no certificate is
returned (P, persistent CA state, since stack `v0.1.2`). C14 `400`: invalid JSON or empty `systemName` (`services/profile-ca/handlers.go` onboarding handler). C15 and C16: `400` invalid JSON; `403` client certificate
has the wrong OU, or `systemName` is empty (the empty-name case is from
`services/profile-ca/handlers.go` `handleDeviceCert`, `handleSystemCert`; P lists only
the OU case). Without a client certificate from this CA the TLS handshake fails and there
is no HTTP status (P "mTLS Endpoints").

The requested `systemName` is not checked against the presenting certificate's CN
(`services/profile-ca/ca.go` `IssueDeviceCert`, `IssueSystemCert`). The certificate's CN
and DNS name are the `systemName`. `POST /ca/certificate/issue` (plain HTTP, no chain,
used by cert-provisioner) is not wrapped by the SDK.

### C17 — MQTT events

There is no stack endpoint. No stack service connects to a broker in the pinned compose.
`events/` connects directly to an MQTT broker (Mosquitto in the course kit) over plain
`tcp`, with no TLS and no broker authentication, which is what `INSECURE` in the
interface name means. The stack contract is limited to:

- the interface name `MQTT-INSECURE-JSON` (F §11), used as `templateName` with
  `protocol: "mqtt"` when a publisher registers its topic as a service instance through C2;
- interface `properties` values are strings (C2 conventions);
- payloads are JSON and their schema is defined by the application, not by the stack.

SDK and course-kit convention, not stack behaviour: a publisher puts its MQTT topic in
the `basePath` property and the broker's address and port in `accessAddresses` and
`accessPort` (`events.Interface`). The stack stores `basePath` as an opaque string and
the orchestrator returns it unchanged as `service.serviceUri` (C "ServiceRegistry
lookup", mapping table), so a consumer that pulls (C5) gets the topic there.

F §11's `ah5/<system>/request` topic scheme belongs to the core systems' own MQTT adapter,
which is not wired in the pinned compose; the SDK does not use it.

## Contract test

Tier 3: start the six services of the pinned compose, run the SDK examples against
them, and assert every row above: status code, and field names and values in the body.
Publishing to the public mirror requires this tier to be green. Until the images exist
on GHCR, the test builds them locally from the public stack tree at the tag commit,
under the image names above.
