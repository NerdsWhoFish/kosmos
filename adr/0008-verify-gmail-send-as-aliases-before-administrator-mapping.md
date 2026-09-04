# 8. Verify Gmail send as aliases before administrator mapping

Date: 2026-09-04

## Status

Accepted.

## Context and Problem Statement

Administrators need users to sign in with one Workspace address and send from another configured alias. Accepting arbitrary From addresses would create failures and misleading identity.

## Considered Options

1. Allow any administrator-entered From address
2. Send only from the login address
3. List Gmail send-as identities and allow only the primary or accepted aliases

## Decision Outcome

Chosen: **option 3**.

Google authorization includes the minimal Gmail settings scope required to list send-as identities. Kosmos permits owners and admins to map a member only to the connected primary address or an alias whose verification status is accepted, then passes that address as the Gmail message From header.

## Consequences

### Good

- Users can send through approved Workspace aliases
- Invalid or unverified aliases fail during configuration instead of send time
- The mapping remains server-enforced

### Bad

- Existing Google connections need incremental reauthorization for the settings scope
- Alias availability depends on Google Workspace configuration
- Administrators can choose among a user's accepted identities

### Rejected because

- Arbitrary addresses can be rejected or rewritten by Gmail and enable identity mistakes.
- Primary-only sending does not satisfy businesses that separate login and customer-facing identities.
