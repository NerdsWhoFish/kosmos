# 6. Import Cloudflare domain metadata without granting mutation

Date: 2026-09-04

## Status

Accepted.

## Context and Problem Statement

Account owners need renewal reminders for domains managed through Cloudflare. Kosmos does not need authority to edit DNS, registrations, or billing to provide those reminders.

## Considered Options

1. Grant Kosmos a broad Cloudflare API token
2. Use a read-only scoped token and import registrar expiry dates
3. Require every renewal date to be entered manually

## Decision Outcome

Chosen: **option 2**.

Kosmos accepts a read-only Cloudflare token, links a selected zone to an account website, and creates idempotent reminders 30, 14, and 7 days before the known renewal date. Registrar-managed zones use imported dates, while externally registered zones require a manual date.

## Consequences

### Good

- A compromised Kosmos token cannot mutate Cloudflare resources
- Renewal reminders stay attached to the account
- Repeated synchronization does not duplicate reminders

### Bad

- Cloudflare API and registrar response changes can break imports
- Externally registered domains still require manual renewal dates
- The encrypted integration credential becomes another secret Kosmos must protect

### Rejected because

- A broad token gives Kosmos dangerous authority it does not need.
- Manual-only dates discard reliable registrar metadata and create avoidable data entry.
