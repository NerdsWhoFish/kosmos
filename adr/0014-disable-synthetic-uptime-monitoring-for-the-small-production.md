# 14. Disable synthetic uptime monitoring for the small production deployment

Date: 2026-09-04

## Status

Accepted.

## Context and Problem Statement

Kosmos production serves three known users and Cloud Run is configured to scale to zero.
The five-minute public health probe creates traffic while providing an availability signal the operators do not currently value enough to keep.

## Considered Options

1. Keep the five-minute public uptime probe and availability alert enabled
2. Reduce the probe frequency or monitored regions
3. Omit the public uptime probe and its availability alert from production

## Decision Outcome

Chosen: **option 3**.

Add an enabled input to the reusable environment module that defaults to true. The Kosmos production configuration sets it to false, which omits the uptime check and its matching alert policy because Google uptime checks do not expose a disabled state. Their module definitions remain available for one-line re-enablement. Other application, job, scheduler, and queue monitoring remains enabled.

## Consequences

### Good

- Cloud Run receives no synthetic uptime traffic and can remain idle between real requests
- The safer reusable-module default remains enabled
- Re-enabling uptime monitoring is a one-line configuration change

### Bad

- Kosmos will not proactively alert when the public endpoint is unavailable
- An outage may remain unnoticed until a user visits the service or another alert fires
- Re-enabling the option recreates the uptime check and its alert policy

### Rejected because

- Keeping the probe was rejected because its recurring traffic and notifications are not useful enough for a three-user internal deployment.
- Reducing frequency or regions was rejected because it still creates synthetic traffic while weakening an already low-value signal.
