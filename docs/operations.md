# Kosmos operations

## Identity and access

Kosmos owns no passwords. Google authenticates users, and production admits only verified identities from the configured domains. The first approved user atomically becomes the organization owner. Later approved users start as members. Owners and administrators can assign owner, admin, member, or read-only viewer roles and can disable access.

Google Workspace access is a separate incremental grant for Gmail compose, Gmail metadata, and read-only Google Sheets. Refresh tokens are encrypted at rest with a key derived from `KOSMOS_SESSION_SECRET`. Rotating that secret intentionally invalidates saved provider tokens, so users reconnect Google afterward. Register one OAuth redirect URI at `https://<host>/auth/callback`.

## Public contact form

Submit JSON to `POST /api/v1/intake/contact` with `name`, `email`, and optional `company`, `phone`, `message`, and `source`. Include an empty `website` field as the honeypot. Kosmos validates size and email shape, deduplicates by email, records source attribution, and limits each client IP to five attempts per hour per Cloud Run instance. Cloudflare remains the global edge rate-limit boundary.

## Gmail and Tiller

Outbound email always requires an explicit user action and an `Idempotency-Key` header. Retries with the same key return the saved delivery instead of sending twice. Inbox sync stores sender, subject, snippet, time, and thread identifiers only for known contacts. It does not store message bodies or replace Gmail.

Tiller import expects a header row with `Date`, `Description`, and `Amount`. `Merchant` and `Transaction ID` are optional. The default range is `Transactions!A:Z`. Stable transaction IDs make imports replay-safe. One deterministic contact match is accepted; zero or multiple matches enter the review queue.

## Files, retention, and recovery

Attachments are private objects. Uploads are limited to 10 MB and PDF, text, JPEG, PNG, or WebP. Receipt records must link to a cost. Downloads require both an authenticated session and an application-signed URL that expires after 15 minutes.

Production buckets prevent public access, use uniform access control, retain object versions, and apply lifecycle cleanup. Firestore production enables point-in-time recovery. CSV exports for contacts and costs provide a portable copy before deletion or migration. Restore by selecting the desired Firestore recovery point and object generation, then verify organization ownership before serving traffic.

## Deployment and observability

Publish `infra/modules/environment` to the Spacelift private module registry, then consume its pinned version from the configurations repository. Quill is the only release path. It versions, signs, and publishes the application image before Spacelift deploys it.

Backend tracing uses standard `OTEL_EXPORTER_OTLP_*` variables. Browser telemetry uses `KOSMOS_FARO_URL`. Telemetry must never include tokens, message bodies, receipt contents, or raw customer data. Production verification checks health, anonymous access denial, responsive login behavior, Cloud Run logs, and a Grafana trace before accepting the release.
