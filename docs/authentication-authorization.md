# Phase 8 authentication and authorization

Phase 8 establishes the control-plane trust boundary:

```text
Authorization bearer credential
  -> transport extraction
  -> provider-neutral TokenVerifier
  -> normalized AuthenticatedPrincipal
  -> persisted identity resolution and ACTIVE lifecycle check
  -> typed capability authorization
  -> TrustedRequest in context.Context
  -> requestauth.StorageScopeFromContext
  -> tenant-scoped application/storage boundary
```

The JWT adapter is deliberately narrow and deterministic: it uses deployment-supplied RSA public-key material, requires RS256, validates issuer, audience, expiration, issued-at, not-before, and required subject/tenant/actor claims, and returns safe authentication errors. It does not perform OIDC discovery, remote JWKS fetching, token refresh, or identity provisioning.

A principal is not an application identity. Authentication proves an external provider subject. Resolution then loads the existing Phase 7 identity repository, requires tenant/actor/provider consistency, and calls `Identity.EnsureAuthorizable()`. UNKNOWN, SUSPENDED, and REVOKED identities are rejected.

Permissions are a closed typed set for the current foundation capabilities: intent create/read, approval request/read, policy evaluation, and execution preparation. Permission authorization is not financial approval and does not bypass wallet binding, policy evaluation, approval, or execution lifecycle checks.

The canonical storage mapping uses only trusted principal tenant/actor values and server-generated request metadata. MCP input cannot construct or replace a principal, permission set, identity, or storage.Scope. Raw bearer tokens never enter domain values, logs, audit metadata, or persistent records.

No Phase 9+ execution runtime or provider functionality is implemented.
