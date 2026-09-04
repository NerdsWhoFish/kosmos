# 3. Authorize Google Workspace separately from sign-in

Date: 2026-09-04

## Status

Accepted.

## Context and Problem Statement

Kosmos uses Google for identity, Gmail for deliberate outbound messages and relevant inbound metadata, and Google Sheets for Tiller imports. Login must remain low-friction and cannot silently grant business-data access.

## Considered Options

1. Request Gmail and Sheets scopes during every login
2. Use a separate incremental Google authorization flow and encrypt each refresh token at rest
3. Avoid provider APIs and expose only Gmail, Voice, and Sheets links

## Decision Outcome

Chosen: **option 2**.

Use a separate Google authorization action after login. The callback verifies that the granted Google subject matches the active Kosmos session. Refresh tokens are encrypted with an application-derived key before organization-scoped persistence, and provider scopes are limited to Gmail compose, Gmail metadata, and read-only Sheets.

## Consequences

### Good

- Authentication remains independent from optional business integrations
- Users explicitly see and approve the provider capabilities they enable
- Refresh tokens are not stored as plaintext and can refresh short-lived access tokens

### Bad

- Rotating the session secret requires users to reconnect Google Workspace
- The OAuth client must keep the shared callback URI configured
- Google consent-screen verification may be required before broad external use

### Rejected because

- Requesting all scopes at login couples account access to optional integrations and creates an unnecessarily alarming consent prompt
- Link-only integrations cannot send mail, detect relevant replies, or import Tiller rows end to end
