# 12. Authenticate workflows with revocable scoped API credentials

Date: 2026-09-04

## Status

Accepted.

## Context and Problem Statement

Kosmos users need durable credentials for external workflows that call its REST API without an interactive Google session.
A shared static secret would make revocation, attribution, and least privilege impossible, while browser sessions require CSRF protection and expire.

## Considered Options

1. Generate per-organization bearer credentials with one-time secret display, hashed storage, explicit read or write access, and administrator-managed revocation
2. Reuse one deployment-wide integration secret
3. Require external workflows to use Google OAuth sessions
4. Issue self-contained signed JWTs without a credential lookup

## Decision Outcome

Owners and administrators can create named API credentials from Settings. Kosmos generates each credential with cryptographically secure randomness, returns the full bearer token only in the creation response, and stores only its SHA-256 hash plus non-secret metadata. Credentials have either read-only or read-and-write access to ordinary workspace APIs. They cannot manage members, credentials, or integrations, even when configured for writes. Every request validates the stored record so revocation takes effect immediately. Creation and revocation are audit events. Authentication spans record only stable outcome and access attributes, never tokens, headers, names, or credential identifiers.

## Consequences

### Good

- Each workflow can be named, attributed, independently revoked, and granted only the mutation access it needs
- A Firestore leak does not expose usable bearer tokens because plaintext credentials are never persisted
- The existing REST API remains the single integration surface and works with ordinary Authorization headers
- Revocation is immediate because every request checks current credential state

### Bad

- Every API credential request adds a Firestore credential lookup
- Lost plaintext credentials cannot be recovered and must be replaced
- The initial read or write access model is intentionally coarser than per-resource scopes
- Long-lived credentials still require operators to rotate and revoke them deliberately

### Rejected because

- The deployment-wide integration secret cannot be revoked per workflow, cannot provide useful attribution, and would give every caller identical authority.
- Google OAuth sessions are designed for humans, expire, require browser interaction, and would force workflows to impersonate employees.
- Self-contained JWTs avoid the datastore lookup but make immediate revocation and authoritative status checks harder without reintroducing server-side state.
