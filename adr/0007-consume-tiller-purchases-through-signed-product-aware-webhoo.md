# 7. Consume Tiller purchases through signed product aware webhooks

Date: 2026-09-04

## Status

Accepted.

## Context and Problem Statement

Kosmos needs optional near-real-time purchase transactions without depending on Tiller spreadsheet imports. A product mapping must remain stable when product names change and delivery must tolerate retries.

## Considered Options

1. Continue importing purchases only from spreadsheets
2. Poll Tiller from Kosmos
3. Receive signed order events and map immutable product IDs to Kosmos accounts

## Decision Outcome

Chosen: **option 3**.

Tiller sends timestamped HMAC-signed order.paid events containing immutable product identifiers and line amounts. A Kosmos owner or admin configures the signing secret and maps each Tiller product ID to one Kosmos account. Kosmos creates deterministic per-line transactions so retries are idempotent.

## Consequences

### Good

- Purchases appear without spreadsheet polling
- Product mappings survive display-name changes
- Retries are safe and consumers need no privileged callback into Tiller

### Bad

- Both services share and rotate a signing secret
- The event contract must evolve compatibly
- One purchase with many lines creates multiple Kosmos transactions

### Rejected because

- Spreadsheet-only imports are delayed and require manual setup.
- Polling couples Kosmos to privileged Tiller APIs and wastes requests when nothing changes.
