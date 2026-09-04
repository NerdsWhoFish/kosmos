# 11. Store account events as an append only account timeline

Date: 2026-09-04

## Status

Accepted.

## Context and Problem Statement

Kosmos needs one chronological account history for changes, communications, documents, reminders, opportunities, domains, and transactions.
The timeline must support cheap recent reads, filtering, and cursor pagination while Cloud Run remains scale to zero.

## Considered Options

1. Persist events in a Firestore subcollection beneath each account
2. Reuse the organization-wide administrative audit log
3. Derive events at read time from every source collection
4. Persist all account events in one organization-wide collection

## Decision Outcome

Persist immutable account events under `organizations/{scope}/accounts/{accountID}/events`. Each event records a category, action, human-readable title and summary, actor, source entity, occurrence time, and creation time. Writers use deterministic identifiers when an external source already provides one, making synchronization retries idempotent. Account mutations write best-effort events after the primary operation and emit structured telemetry if the timeline write fails. The API serves recent and filtered history with cursor pagination.

## Consequences

### Good

- Recent account history reads only the owning account subcollection and does not scan unrelated organizations or accounts.
- External synchronization retries can avoid duplicate email and transaction events.
- Events remain readable after a related contact, opportunity, document, or reminder is deleted.
- The timeline API can add categories and source actions without coupling its schema to every source model.

### Bad

- Existing account history is not automatically backfilled and the timeline begins with newly observed events.
- Best-effort dual writes can miss an event when the primary mutation succeeds but the timeline write fails.
- A future cross-account event feed will require a collection-group query and supporting index.
- Deleting an account must explicitly remove its event subcollection because Firestore does not cascade deletes.

### Rejected because

- The administrative audit log has different retention, access, and payload needs, and it does not carry a durable account association for all source entities.
- Deriving history at read time requires expensive joins across unrelated schemas, produces unstable pagination, and loses history when source records are deleted.
- One organization-wide event collection makes every account read depend on an account identifier composite index and weakens natural data ownership without providing a required current feature.
