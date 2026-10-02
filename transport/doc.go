// Package transport is the HTTP layer every SDK client is built on.
//
// There are two kinds of client and the caller always picks one explicitly:
//
//   - NewMTLS for the foundation systems (ServiceRegistry, Authentication,
//     ConsumerAuthorization) and the profile-ca mTLS port. It presents a
//     client certificate, verifies the server against the given roots, and
//     checks the server certificate against an explicit server name. The
//     foundation server certificates carry only the service name
//     ("serviceregistry", "authentication", "consumerauth") as DNS name, so a
//     client that dials localhost must still name the service.
//   - NewPlain for the orchestrator (dynamicorch-xacml has no TLS listener)
//     and the profile-ca plain port. Nothing is authenticated: the orchestrator
//     believes whatever requester name the body asserts.
//
// Certificates are loaded in one explicit call, LoadCredentials (files) or
// CredentialsFromPEM (memory, e.g. straight from the ca package). Nothing is
// read from environment variables or default locations.
//
// The client never retries, never follows redirects, and never attaches a
// token on its own: a Bearer token is sent only when the Request carries one.
package transport
