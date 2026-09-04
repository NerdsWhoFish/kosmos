# 5. Make accounts own customer lifecycle and business identity

Date: 2026-09-04

## Status

Accepted.

## Context and Problem Statement

Kosmos previously duplicated business identity and prospect or customer state on contacts. Opportunities also treated a contact as their primary owner, which makes account-level reporting and workflows inconsistent.

## Considered Options

1. Keep lifecycle and opportunity ownership on contacts
2. Move business identity, lifecycle, and opportunity ownership to accounts while contacts remain optional context
3. Duplicate ownership on accounts and contacts

## Decision Outcome

Chosen: **option 2**.

Accounts are the canonical business identity and own lifecycle state and opportunities. Contacts belong to an account and may be referenced by an opportunity only as optional context.

## Consequences

### Good

- One source of truth for each business relationship
- Account-level reporting and documents become straightforward
- Contacts can change without changing opportunity ownership

### Bad

- Existing contact-centric API consumers must adopt account IDs
- Account deletion and migration logic must preserve relationship integrity

### Rejected because

- Keeping contact ownership preserves the duplicate state that caused the inconsistency.
- Duplicating ownership creates synchronization rules and ambiguous reporting.
