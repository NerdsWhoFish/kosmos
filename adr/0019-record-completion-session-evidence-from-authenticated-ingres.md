# 19. Record completion session evidence from authenticated ingress metadata

Date: 2026-09-05

## Status

Accepted.

## Context and Problem Statement

Document senders need a signing audit that records the network address, browser description, and approximate location associated with completion.
The public signing endpoint must not trust caller-supplied forwarding headers or turn this business evidence into behavioral analytics.

## Considered Options

1. Capture a bounded completion snapshot from a signed ingress envelope and save it with the winning completion.
2. Trust browser-submitted or ordinary forwarded IP and location fields.
3. Request device geolocation or add browser fingerprinting.
4. Collect all document views and signing activity through the analytics pipeline.

## Decision Outcome

Chosen: **option 1**.

The ingress signs a domain-separated HMAC envelope binding method, signing completion path, payload, and a timestamp valid within 60 seconds. Reuse the existing dedicated intake secret while keeping the signing-session signature domain distinct. In production, accept the IP address, browser User-Agent, and optional city, region, and country only from the verified Cloudflare envelope. Cloudflare location is approximate edge-provided IP geolocation. Direct local operation records the socket address and supplied User-Agent without claiming edge location. Save the snapshot once in the same compare-and-swap completion that selects the completed PDF, include it in completion evidence, and return the saved winner on retries. Preserve absence on older completed requests. Disclose the collection before consent; keep the values in the signing business record and artifact, outside logs and analytics.

## Consequences

### Good

- Provides useful completion evidence without trusting forgeable forwarding headers or requesting location permissions.
- Concurrent completion and retry behavior retain one immutable snapshot tied to the winning PDF.
- Reuses existing ingress and secret provisioning without another geolocation vendor or persistent service.
- Bounds browser and location fields, and retains exact UTF-8 metadata while safely escaping unsupported PDF characters.

### Bad

- An IP address, self-reported browser description, and approximate city or region are not proof of identity or precise physical location.
- Session evidence is personal data retained with the document. Anyone holding a still-valid completed signing link can read it in the completed request and PDF.
- Ingress rollout must precede the application change; missing, invalid, or stale envelopes prevent new production completions.
- The PDF's existing font coverage requires escaping some Unicode evidence text, which is less readable than the exact UTF-8 metadata.

### Rejected because

- Browser-submitted or unsigned forwarded metadata is spoofable and cannot establish the origin of the recorded evidence.
- GPS and fingerprinting collect more sensitive information than this completion audit needs and add permission and reliability costs.
- View tracking and analytics broaden collection beyond the requested signing record and conflict with telemetry privacy boundaries.
