# 17. Embed single recipient electronic signing in Kosmos

Date: 2026-09-05

## Status

Accepted.

## Context and Problem Statement

Kosmos users need to upload a PDF, position signature and date fields, and share a signing link with a customer or prospect.
The application must retain its stateless, scale-to-zero deployment and avoid sending private documents to another provider.

## Considered Options

1. Embed single-recipient electronic signing in the existing Go service and private storage.
2. Integrate a third-party e-signature SaaS.
3. Deploy a complete self-hosted e-signature suite.

## Decision Outcome

Chosen: **option 1**.

Embed signing in Kosmos using immutable original PDFs and normalized field coordinates. Derive a possession token with domain-separated HMAC-SHA256 over the organization scope and unique immutable request ID using the existing integration key, encoded as 43 base64url characters. Return the token once in a URL fragment and accept it only through a dedicated header. Verify the HMAC before reading Firestore to reject unauthenticated lookup traffic without database reads; the stored token SHA-256 hash and request state remain authoritative. Links expire after 1 to 90 days. Draft fields lock when the link is created. Server-side rendering adds typed signature, date, name, and text values plus an evidence page, while metadata records original and completed PDF hashes. Firestore compare-and-swap transactions enforce revision and state checks so revocation and concurrent completion have one winning terminal result.

## Consequences

### Good

- Reuses Kosmos identity, private storage, Firestore, deployment, and observability without another persistent service.
- Original bytes remain available and verifiable; rendering checks their hash before producing a completed artifact.
- Explicit consent, UTC completion time, document hashes, and lifecycle events accompany the signed document.
- Fragment and header transport keep the bearer secret out of request URLs; pre-storage HMAC checks reject forged tokens without billable document reads.

### Bad

- A link proves possession only and can be forwarded. The signer name is self-reported and the recipient email is supplied by the sender without independent verification.
- Rotating the integration key invalidates every signing link, including completed-document download links. Authenticated workspace access to the PDFs remains available.
- This is an electronic signature record, with no PKI certificate and no claim of DocuSign feature or compliance equivalence.
- Version one supports one signer, typed signatures, flattened PDFs, and Windows-1252 Western European text only.
- PDF validation and rendering add an in-process parser and conservative resource limits; unsupported forms, annotations, actions, rotation, and cropping require flattening before upload.
- Firestore metadata and object storage are not one atomic transaction; ambiguous failures can retain unreferenced artifacts requiring cautious recovery.

### Rejected because

- Third-party SaaS adds billing and transfers sensitive documents to another provider.
- A complete self-hosted e-signature suite adds a persistent service and operating burden against the near-free stateless deployment boundary.
