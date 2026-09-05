# 21. Collect parallel signatures with recipient scoped capabilities

Date: 2026-09-05

## Status

Accepted. Supersedes [ADR-0017](0017-embed-single-recipient-electronic-signing-in-kosmos.md).

## Context and Problem Statement

A document may need signatures from both staff and a prospect, with everyone allowed to sign immediately.
Each recipient must only fill their assigned fields, retain their own signing evidence, and receive a short download window after they sign.
Concurrent submissions must preserve every accepted signature and produce one coherent final document.

## Considered Options

1. Collect up to ten parallel recipients under one request, with assigned fields and server-rendered aggregate snapshots.
2. Require sequential signing through a fixed recipient order.
3. Create independent single-signer requests and merge the resulting documents later.

## Decision Outcome

Chosen: **option 1**.

Drafts contain one to ten recipients with stable IDs, names, and email addresses. Every field belongs to a recipient, and each recipient must have a required signature field before issuing links. Issuing links freezes the recipients and fields. Each recipient receives an independent, domain-separated HMAC capability bound to the organization, request, and recipient ID, verified before datastore reads. Legacy single-recipient requests remain supported.

All unsigned recipients may sign before the shared signing deadline. The server accepts only the current recipient's field values and captures their entered name, consent, UTC signing time, and authenticated session evidence once. Compare-and-swap retries merge that immutable candidate with other accepted recipients and re-render the original prepared PDF with all accepted field values. Each committed recipient retains the key of their winning snapshot. A request remains pending until everybody signs; partial PDF evidence pages explicitly say PARTIALLY SIGNED and identify outstanding recipients. Final completion records the all-signed state once.

Each recipient's original link expires 15 minutes after their own accepted signature. Within that window, they can download the current partial or final artifact. Staff can create a fresh final-document download link after all recipients finish. Public JSON exposes the current recipient's writable fields and personal session, plus limited progress for the others. Shared signed PDF evidence includes the participants' signing records, as disclosed before consent. Atomic deletion includes every committed snapshot in the cleanup outbox. Failed render attempts require an ownership check before retained-file cleanup.

## Consequences

### Good

- Staff and prospects can sign concurrently without coordinating a signing order.
- Per-recipient authorization prevents one link from filling another person's fields.
- Immutable captured candidates preserve signing time and session evidence through concurrent transaction retries.
- Every delivered snapshot is traceable and included in document cleanup.

### Bad

- Partial copies can exist until the remaining recipients sign; the UI and PDF record must make that state unmistakable.
- An early recipient's link may expire before final completion, requiring staff to provide a fresh final download link.
- Re-rendering after concurrent writes costs extra work and retains intermediate snapshots subject to storage policy.
- Possession links can be forwarded, and typed names and supplied email addresses do not independently verify identity.

### Rejected because

- Sequential signing conflicts with the user's explicit choice that everyone can sign immediately.
- Independent requests produce separate evidence histories and cannot safely combine field state or completion into one authoritative document.
