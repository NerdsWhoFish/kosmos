# 15. Persist contact synchronization intents with workspace mutations

Date: 2026-09-04

## Status

Accepted.

## Context and Problem Statement

Cloud Tasks and Firestore cannot participate in one transaction. Previously a committed contact deletion could lose its Google deletion task permanently when dispatch failed.

## Considered Options

1. Persist an outbox entry in the same Firestore transaction or batch as each contact mutation
2. Enqueue only after committing the contact mutation
3. Call Google synchronously before committing

## Decision Outcome

Persist the synchronization intent atomically with the contact mutation. Dispatch immediately through the existing queue and replay pending intents during scheduled and manual synchronization. Acknowledge the matching intent version only after provider success. Preserve deletion intents and provider mappings until completion.

## Consequences

### Good

- A queue outage no longer discards deletions or updates.
- Reuses Firestore, Cloud Tasks, and the existing scheduler without an always-on process.

### Bad

- Pending intents add storage and require replay-safe provider operations.
- Automatic recovery after exhausted retries waits for the next business-hours scheduler pass unless an administrator requests sync.

### Rejected because

- Enqueue-after-commit cannot recover missing deleted records.
- Synchronous provider calls couple workspace availability and latency to Google and still cannot atomically commit both systems.
