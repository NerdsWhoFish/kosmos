# 20. Separate signing and download access and persist document cleanup

Date: 2026-09-05

## Status

Accepted. Supersedes [ADR-0017](0017-embed-single-recipient-electronic-signing-in-kosmos.md).

## Context and Problem Statement

A completed document should remain downloadable briefly through its original signing link, while staff can issue fresh download links later.
Staff need confirmed deletion of drafts and completed documents without leaving active links or losing track of files held by storage retention.
Firestore metadata and private object storage cannot participate in one transaction.

## Considered Options

1. Keep short signing access, rotate independent download capabilities, and delete metadata atomically with a durable cleanup outbox.
2. Extend or reuse the original signing link for future downloads and delete objects synchronously.
3. Store many independent download-link records and use soft deletion without scheduled file cleanup.

## Decision Outcome

Chosen: **option 1**.

Original signing tokens remain valid for pending requests until the configured signing deadline. On completion, access switches to a 15-minute window derived from the immutable completion time, including for existing completed records. Authenticated staff issue a fresh download-only token for 1 minute to 7 days, defaulting to 60 minutes. A random nonce and a distinct HMAC domain preserve rejection of forged tokens before datastore reads; the stored token hash and expiry authorize only the latest generated link. Download capabilities cannot complete a request or fetch the prepared or uploaded source. Link creation uses revision compare-and-swap and leaves the signed artifact and completion evidence unchanged.

Confirmed deletion of draft, completed, or revoked requests atomically removes the live record and writes minimal object keys to a private cleanup outbox. Pending requests must be revoked first. The existing scheduler and job queue retry due cleanup records in bounded batches, recording per-object progress transactionally and deferring retained or temporarily unavailable objects. Link access stops as soon as the live record is gone. No retention policy is weakened.

## Consequences

### Good

- Limits accidental long-lived access while keeping later customer downloads possible.
- Preserves immutable signed artifacts and completion evidence across link rotation.
- Deletion cannot lose its cleanup obligation during storage or queue failures.
- Reuses existing managed storage, transactions, scheduled jobs, and private telemetry.

### Bad

- Replacing a generated download link invalidates the previous generated link, including one another staff member may have shared.
- Existing completed signing links older than the grace period stop working after rollout; staff must issue a download link.
- Files can remain privately retained after user-visible deletion until retention ends and a scheduled cleanup succeeds.
- The scheduler's existing business-hour cadence can delay physical cleanup, and download links still prove possession rather than identity.

### Rejected because

- Reusing signing tokens couples signing permission to later downloads; synchronous object deletion fails while retention is active and cannot atomically remove metadata.
- Multiple active link records add management and revocation complexity the requested workflow does not need; soft deletion alone retains unwanted private files indefinitely.
