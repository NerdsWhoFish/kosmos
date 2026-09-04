# 10. Authorize one shared Google Contacts account for Google Voice

Date: 2026-09-04

## Status

Accepted.

## Context and Problem Statement

Nerds Who Fish shares a free Google Voice account that is separate from every employee's approved Workspace identity.
Google Voice reads that account's Google Contacts, while Kosmos must keep login domain restrictions and personal Gmail grants intact.

## Considered Options

1. Let an owner or administrator authorize one organization-wide Google Contacts account through a delegated OAuth grant
2. Require every employee to authorize and maintain a separate Google Contacts copy
3. Share the Google account password with Kosmos or automate the Google Voice web interface

## Decision Outcome

Chosen: **option 1**.

Add a distinct `voice-contacts` OAuth purpose that may connect a verified Google identity different from the signed-in administrator. Recheck the initiating user's active owner or administrator role both before redirect and at callback, encrypt the refresh token at organization scope, and grant only the Google Contacts scope. Kosmos contact mutations enqueue retryable synchronization jobs against this shared connection. Store Google resource names and etags separately from workspace contact records so disconnecting the integration does not alter customer data. Kosmos remains the source of truth.

## Consequences

### Good

- Every employee sees the same names in the shared Google Voice account
- Kosmos never stores or distributes the shared Google account password
- Login and employee Gmail permissions remain isolated from the organization-wide contacts grant
- Cloud Tasks retries provider failures while both Cloud Run services continue scaling to zero

### Bad

- The shared account becomes an organization-wide dependency and must be reconnected if Google revokes its refresh token
- Contact changes are eventually consistent with Google Voice
- Deleting a Google contact requires an explicit destructive choice and can affect every employee using the shared Voice account
- The additional Google Contacts scope may require OAuth consent-screen verification

### Rejected because

- Per-user copies drift, create duplicate contacts, and require each employee to connect the same data independently.
- Storing a shared password is unacceptable, and Voice UI automation is brittle because Google offers no supported Voice contact API.
