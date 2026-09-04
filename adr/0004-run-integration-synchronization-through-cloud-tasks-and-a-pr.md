# 4. Run integration synchronization through Cloud Tasks and a private worker

Date: 2026-09-04

## Status

Accepted.

## Context and Problem Statement

Kosmos must synchronize Gmail and Tiller without keeping an instance alive or blocking an interactive HTTP request.
Scheduled synchronization runs hourly from 9 AM through 5 PM on weekdays in America/New_York, and every machine-to-machine endpoint must remain private.

## Considered Options

1. Use Cloud Scheduler and user actions to enqueue Cloud Tasks for a private Cloud Run worker
2. Have Cloud Scheduler invoke the public web service and perform provider calls inline
3. Run timers inside the scale-to-zero web process

## Decision Outcome

Chosen: **option 1**.

Use one Cloud Tasks queue and a second Cloud Run service built from the same immutable image. The worker exposes only scheduling and execution routes, requires a dedicated OIDC invoker identity, and keeps minimum instances at zero. Cloud Scheduler invokes the private scheduling route, which creates one task per connected account and provider. Interactive synchronization actions enqueue the same task type and return HTTP 202.

## Consequences

### Good

- Provider latency no longer blocks browser requests.
- Cloud Tasks provides bounded retries, backoff, and conservative concurrency.
- The web and worker services both scale to zero and use the same tested artifact.
- Private worker routes are protected by Cloud Run IAM and a dedicated OIDC identity.
- Scheduled and interactive synchronization share one execution path.

### Bad

- The deployment gains a second Cloud Run service, a service account, and additional IAM wiring.
- Cold starts can delay scheduled work.
- Failures occur asynchronously and require notifications, dashboards, and queue inspection.
- One scheduled pass can create several tasks when many users connect Google Workspace.

### Rejected because

- Calling the public service inline exposes a machine endpoint at the internet boundary, couples scheduler deadlines to provider latency, and gives user-triggered requests no durable retry.
- In-process timers do not run while Cloud Run is scaled to zero and can execute more than once when multiple instances exist.
