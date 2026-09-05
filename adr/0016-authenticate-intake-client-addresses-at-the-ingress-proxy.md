# 16. Authenticate intake client addresses at the ingress proxy

Date: 2026-09-04

## Status

Accepted.

## Context and Problem Statement

The public intake endpoint runs on multiple Cloud Run instances behind a Cloudflare Worker. The run.app origin is publicly reachable, and caller-supplied forwarding headers cannot identify a trusted client.

## Considered Options

1. Sign the client address at the existing Worker and enforce shared Firestore quotas
2. Trust CF-Connecting-IP or a fixed forwarded-header position
3. Add a new external load balancer and edge rate-limit service

## Decision Outcome

The ingress Worker overwrites client-address headers and signs the canonical address and timestamp with a dedicated secret. The web process rejects unsigned or stale intake requests in production and applies a shared Firestore rolling-window quota. Expired quota documents use Firestore TTL. Secrets remain server-side and separate from session and integration keys.

## Consequences

### Good

- Direct-origin requests and forged forwarding headers cannot evade quotas.
- Reuses the existing ingress and database while retaining scale-to-zero.

### Bad

- Ingress and application secret provisioning and rotation must be coordinated.
- Firestore availability becomes a prerequisite for intake; requests fail closed when the limiter cannot be read.

### Rejected because

- Trusted-looking forwarding headers are caller-controlled at the public origin.
- A new load balancer adds infrastructure and recurring costs when the existing Worker can establish the required trust boundary.
